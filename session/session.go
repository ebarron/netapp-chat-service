// Package session provides in-memory conversation session management for the
// chatbot. Sessions are keyed by ID and hold a sliding window of messages.
//
// Design ref: docs/chatbot-design-spec.md §5.3
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/ebarron/netapp-chat-service/llm"
)

// DefaultMaxMessages is the maximum number of messages kept per session.
const DefaultMaxMessages = 40

// Session holds the conversation state for a single chat session.
type Session struct {
	ID        string
	Messages  []llm.Message
	CreatedAt time.Time
	UpdatedAt time.Time

	maxMessages int
	turnMu      sync.Mutex
	turnActive  bool
	turnWait    chan struct{}
	turnClosed  bool
}

// AcquireTurn waits until no other turn is running for this session. It
// returns false without acquiring the session when ctx is cancelled.
func (s *Session) AcquireTurn(ctx context.Context) bool {
	for {
		s.turnMu.Lock()
		if s.turnClosed {
			s.turnMu.Unlock()
			return false
		}
		if !s.turnActive {
			s.turnActive = true
			s.turnMu.Unlock()
			return true
		}
		if s.turnWait == nil {
			s.turnWait = make(chan struct{})
		}
		wait := s.turnWait
		s.turnMu.Unlock()

		select {
		case <-ctx.Done():
			return false
		case <-wait:
		}
	}
}

// ReleaseTurn allows the next waiting turn for this session to proceed.
func (s *Session) ReleaseTurn() {
	s.turnMu.Lock()
	if !s.turnActive {
		s.turnMu.Unlock()
		return
	}
	s.turnActive = false
	if s.turnWait != nil {
		close(s.turnWait)
		s.turnWait = nil
	}
	s.turnMu.Unlock()
}

// Close prevents new turns from acquiring this session and waits for the
// active turn, if any, to release it. Waiting turns are awakened by the active
// turn's release and fail their acquisition because the session is closed.
func (s *Session) Close(ctx context.Context) bool {
	s.turnMu.Lock()
	s.turnClosed = true
	if !s.turnActive {
		s.turnMu.Unlock()
		return true
	}
	if s.turnWait == nil {
		s.turnWait = make(chan struct{})
	}
	wait := s.turnWait
	s.turnMu.Unlock()

	select {
	case <-ctx.Done():
		return false
	case <-wait:
		return true
	}
}

// AddMessage appends a message and trims the window if needed.
// Tool call/result pairs are kept together: we always trim from the front,
// skipping the system messages.
func (s *Session) AddMessage(msg llm.Message) {
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
	s.trimWindow()
}

// trimWindow removes the oldest non-system messages when the window exceeds
// maxMessages.
func (s *Session) trimWindow() {
	max := s.maxMessages
	if max <= 0 {
		max = DefaultMaxMessages
	}
	if len(s.Messages) <= max {
		return
	}

	// Count leading system messages (always preserved).
	sysCount := 0
	for _, m := range s.Messages {
		if m.Role == llm.RoleSystem {
			sysCount++
		} else {
			break
		}
	}

	// Remove oldest non-system messages from the front.
	excess := len(s.Messages) - max
	if excess <= 0 {
		return
	}
	// Preserve system messages, trim from after them.
	preserved := s.Messages[:sysCount]
	rest := s.Messages[sysCount:]
	if excess >= len(rest) {
		// Shouldn't happen, but be safe.
		s.Messages = preserved
		return
	}
	s.Messages = append(preserved, rest[excess:]...)
}

// Manager is a concurrent-safe session store. Sessions are held in memory
// and are lost on process restart.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	maxMsg   int
}

// NewManager creates a new session manager.
func NewManager(maxMessages int) *Manager {
	if maxMessages <= 0 {
		maxMessages = DefaultMaxMessages
	}
	return &Manager{
		sessions: make(map[string]*Session),
		maxMsg:   maxMessages,
	}
}

// Get returns the session for the given ID, or nil if it doesn't exist.
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetOrCreate returns the session for the given ID, creating it if needed.
// If id is empty, a new session with a generated ID is created.
func (m *Manager) GetOrCreate(id string) *Session {
	if id == "" {
		return m.create()
	}

	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if ok {
		return s
	}

	// Create new session with the requested ID.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if s, ok = m.sessions[id]; ok {
		return s
	}

	now := time.Now()
	s = &Session{
		ID:          id,
		Messages:    make([]llm.Message, 0, 16),
		CreatedAt:   now,
		UpdatedAt:   now,
		maxMessages: m.maxMsg,
	}
	m.sessions[id] = s
	return s
}

// Delete removes a session after its active turn has completed.
func (m *Manager) Delete(id string) {
	_ = m.DeleteContext(context.Background(), id)
}

// DeleteContext closes a session, waits for its active turn to complete, and
// removes that exact session from the manager. Closing before removal prevents
// waiters on the old object from overlapping a newly created same-ID session.
func (m *Manager) DeleteContext(ctx context.Context, id string) bool {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil {
		return true
	}
	if !s.Close(ctx) {
		// Closing is irreversible: keep the old object unavailable and finish
		// removing it after its active turn eventually releases. This prevents
		// a cancelled delete request from stranding a permanently closed entry
		// or recreating the same ID while the old turn is still running.
		go func() {
			if s.Close(context.Background()) {
				m.deleteSessionIfSame(id, s)
			}
		}()
		return false
	}
	m.deleteSessionIfSame(id, s)
	return true
}

func (m *Manager) deleteSessionIfSame(id string, s *Session) {
	m.mu.Lock()
	if m.sessions[id] == s {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

// Count returns the number of active sessions.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// create generates a new session with a random ID.
func (m *Manager) create() *Session {
	id := generateID()
	now := time.Now()
	s := &Session{
		ID:          id,
		Messages:    make([]llm.Message, 0, 16),
		CreatedAt:   now,
		UpdatedAt:   now,
		maxMessages: m.maxMsg,
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	return s
}

// generateID creates a random hex session ID.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
