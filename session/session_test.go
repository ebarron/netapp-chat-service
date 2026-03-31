package session

import (
	"sync"
	"testing"

	"github.com/ebarron/netapp-chat-service/llm"
)

func TestNewManager(t *testing.T) {
	m := NewManager(0)
	if m.maxMsg != DefaultMaxMessages {
		t.Errorf("default maxMsg = %d, want %d", m.maxMsg, DefaultMaxMessages)
	}

	m2 := NewManager(10)
	if m2.maxMsg != 10 {
		t.Errorf("custom maxMsg = %d, want 10", m2.maxMsg)
	}
}

func TestGetOrCreateNewSession(t *testing.T) {
	m := NewManager(20)

	s := m.GetOrCreate("")
	if s == nil {
		t.Fatal("expected non-nil session")
	}
	if s.ID == "" {
		t.Error("expected generated session ID")
	}
	if len(s.Messages) != 0 {
		t.Errorf("new session should have 0 messages, got %d", len(s.Messages))
	}
	if m.Count() != 1 {
		t.Errorf("count = %d, want 1", m.Count())
	}
}

func TestGetOrCreateExistingSession(t *testing.T) {
	m := NewManager(20)

	s1 := m.GetOrCreate("sess-1")
	s1.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hello"})

	s2 := m.GetOrCreate("sess-1")
	if s2.ID != "sess-1" {
		t.Errorf("ID = %q, want %q", s2.ID, "sess-1")
	}
	if len(s2.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(s2.Messages))
	}
	if m.Count() != 1 {
		t.Errorf("count = %d, want 1 (should not duplicate)", m.Count())
	}
}

func TestGetNonExistent(t *testing.T) {
	m := NewManager(20)
	s := m.Get("nonexistent")
	if s != nil {
		t.Error("expected nil for nonexistent session")
	}
}

func TestDeleteSession(t *testing.T) {
	m := NewManager(20)
	m.GetOrCreate("sess-del")
	if m.Count() != 1 {
		t.Fatalf("count = %d, want 1", m.Count())
	}

	m.Delete("sess-del")
	if m.Count() != 0 {
		t.Errorf("count = %d after delete, want 0", m.Count())
	}
	if m.Get("sess-del") != nil {
		t.Error("session should be nil after delete")
	}
}

func TestSlidingWindow(t *testing.T) {
	m := NewManager(5)
	s := m.GetOrCreate("win-test")

	// Add 7 messages — should keep only 5
	for i := range 7 {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		s.AddMessage(llm.Message{Role: role, Content: "msg"})
	}

	if len(s.Messages) != 5 {
		t.Errorf("messages = %d, want 5", len(s.Messages))
	}
}

func TestSlidingWindowPreservesSystemMessages(t *testing.T) {
	m := NewManager(4)
	s := m.GetOrCreate("sys-test")

	// Add 1 system message, then 5 regular messages
	s.AddMessage(llm.Message{Role: llm.RoleSystem, Content: "system prompt"})
	for i := 1; i <= 5; i++ {
		s.AddMessage(llm.Message{Role: llm.RoleUser, Content: "msg"})
	}

	// Should have: 1 system + 3 recent (window=4)
	if len(s.Messages) != 4 {
		t.Errorf("messages = %d, want 4", len(s.Messages))
	}
	if s.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", s.Messages[0].Role, "system")
	}
	if s.Messages[0].Content != "system prompt" {
		t.Error("system message content should be preserved")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager(100)
	var wg sync.WaitGroup

	// Concurrent creates
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := m.GetOrCreate("")
			s.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hello"})
			_ = i
		}(i)
	}

	// Concurrent gets
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Count()
		}()
	}

	wg.Wait()

	if m.Count() != 50 {
		t.Errorf("count = %d, want 50", m.Count())
	}
}

func TestGenerateIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for range 100 {
		id := generateID()
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}
