package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/interest"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

// ChatDeps holds the dependencies for the chat handlers.
type ChatDeps struct {
	Sessions     *session.Manager
	Provider     llm.Provider
	Router       mcpclient.ToolRouter
	Logger       *slog.Logger
	Model        string
	Capabilities []capability.Capability
	Catalog      *interest.Catalog
	InterestsDir string
	ExtraTools   map[string]agent.InternalTool
	PromptConfig agent.SystemPromptConfig
}

// PendingApproval represents a tool call waiting for user approval.
type PendingApproval struct {
	ID         string          `json:"approval_id"`
	Capability string          `json:"capability"`
	ToolName   string          `json:"tool"`
	Params     json.RawMessage `json:"params"`
	Desc       string          `json:"description"`
	resultCh   chan bool
}

// ChatMessageRequest is the JSON body for POST /chat/message.
type ChatMessageRequest struct {
	Message    string                   `json:"message"`
	Mode       string                   `json:"mode,omitempty"`
	SessionID  string                   `json:"session_id,omitempty"`
	CanvasTabs []agent.CanvasTabSummary `json:"canvas_tabs,omitempty"`
}

// ChatEmitter is called for each SSE event.
type ChatEmitter func(event string, data any)

var (
	pendingApprovals sync.Map
	activeContexts   sync.Map
)

// Server is the chat service HTTP server.
type Server struct {
	deps   *ChatDeps
	mux    *http.ServeMux
	logger *slog.Logger
}

// New creates a new chat service server.
func New(deps *ChatDeps) *Server {
	s := &Server{
		deps:   deps,
		mux:    http.NewServeMux(),
		logger: deps.Logger,
	}

	s.mux.HandleFunc("POST /chat/message", s.PostChatMessage)
	s.mux.HandleFunc("DELETE /chat/session", s.DeleteChatSession)
	s.mux.HandleFunc("GET /chat/capabilities", s.GetChatCapabilities)
	s.mux.HandleFunc("POST /chat/capabilities", s.PostChatCapabilities)
	s.mux.HandleFunc("POST /chat/approve", s.PostChatApprove)
	s.mux.HandleFunc("POST /chat/deny", s.PostChatDeny)
	s.mux.HandleFunc("POST /chat/stop", s.PostChatStop)
	s.mux.HandleFunc("GET /health", s.GetHealth)

	return s
}

// ServeUI registers a handler that serves the embedded UI shell at /.
// The provided fsys should be the ui.Dist embed.FS.
func (s *Server) ServeUI(fsys fs.FS) {
	sub, err := fs.Sub(fsys, "dist")
	if err != nil {
		s.logger.Warn("ui dist not available", "error", err)
		return
	}

	fileServer := http.FileServer(http.FS(sub))

	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file; fall back to index.html for SPA routing.
		f, err := sub.Open(r.URL.Path[1:]) // strip leading /
		if err != nil {
			// Serve index.html for any path that doesn't match a static file.
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	})

	s.logger.Info("serving built-in chat UI at /")
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// PostChatMessage streams agent responses as SSE events.
func (s *Server) PostChatMessage(w http.ResponseWriter, r *http.Request) {
	var req ChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "message is required"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "streaming not supported"})
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var emitMu sync.Mutex
	emit := func(event string, data any) {
		jsonData, err := json.Marshal(data)
		if err != nil {
			s.logger.Error("failed to marshal SSE data", "error", err)
			return
		}
		emitMu.Lock()
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
		flusher.Flush()
		emitMu.Unlock()
	}

	sess := s.deps.Sessions.GetOrCreate(req.SessionID)
	activeContexts.Store(sess.ID, cancel)
	defer activeContexts.Delete(sess.ID)

	approvalFunc := func(capID, toolName string, tc llm.ToolCall) bool {
		approvalID := randomID()
		pa := &PendingApproval{
			ID:         approvalID,
			Capability: capID,
			ToolName:   toolName,
			Params:     tc.Input,
			Desc:       fmt.Sprintf("%s → %s", capID, toolName),
			resultCh:   make(chan bool, 1),
		}
		pendingApprovals.Store(approvalID, pa)

		emit("tool_approval_required", map[string]any{
			"type":        "tool_approval_required",
			"approval_id": approvalID,
			"capability":  capID,
			"tool":        toolName,
			"params":      tc.Input,
			"description": pa.Desc,
		})

		select {
		case approved := <-pa.resultCh:
			return approved
		case <-ctx.Done():
			pendingApprovals.Delete(approvalID)
			return false
		case <-time.After(5 * time.Minute):
			pendingApprovals.Delete(approvalID)
			return false
		}
	}

	RunChat(ctx, s.deps, req, emit, approvalFunc)
}

// DeleteChatSession clears a session's conversation history.
func (s *Server) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "session_id is required"})
		return
	}
	s.deps.Sessions.Delete(body.SessionID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "session cleared"})
}

// GetChatCapabilities returns the current capability states.
func (s *Server) GetChatCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := s.deps.Capabilities
	router := s.deps.Router

	totalTools := 0
	connectedServers := router.ConnectedServers()
	serverConnected := make(map[string]bool, len(connectedServers))
	for _, name := range connectedServers {
		serverConnected[name] = true
	}

	toolMap := router.ToolMap()
	for i := range caps {
		caps[i].Available = serverConnected[caps[i].ServerName]
		count := 0
		for _, serverName := range toolMap {
			if serverName == caps[i].ServerName {
				count++
			}
		}
		caps[i].ToolsCount = count
		totalTools += count
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"capabilities": caps,
		"total_tools":  totalTools,
	})
}

// PostChatCapabilities updates capability states.
func (s *Server) PostChatCapabilities(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Capabilities map[string]string `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	for i := range s.deps.Capabilities {
		cap := &s.deps.Capabilities[i]
		if newState, ok := body.Capabilities[cap.ID]; ok {
			st := capability.State(newState)
			if st.Valid() {
				cap.State = st
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "capabilities updated"})
}

// PostChatApprove approves a pending tool call.
func (s *Server) PostChatApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ApprovalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "approval_id is required"})
		return
	}

	v, ok := pendingApprovals.LoadAndDelete(body.ApprovalID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "approval not found or expired"})
		return
	}
	v.(*PendingApproval).resultCh <- true
	writeJSON(w, http.StatusOK, map[string]string{"message": "approved"})
}

// PostChatDeny denies a pending tool call.
func (s *Server) PostChatDeny(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ApprovalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "approval_id is required"})
		return
	}

	v, ok := pendingApprovals.LoadAndDelete(body.ApprovalID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "approval not found or expired"})
		return
	}
	v.(*PendingApproval).resultCh <- false
	writeJSON(w, http.StatusOK, map[string]string{"message": "denied"})
}

// PostChatStop cancels an in-progress chat.
func (s *Server) PostChatStop(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.SessionID != "" {
		if cancel, ok := activeContexts.LoadAndDelete(body.SessionID); ok {
			cancel.(context.CancelFunc)()
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "stopped"})
}

// GetHealth returns service health status.
func (s *Server) GetHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RunChat runs the agent loop for a single user message, emitting SSE events.
func RunChat(ctx context.Context, deps *ChatDeps, req ChatMessageRequest, emit ChatEmitter, approvalFunc func(capID, toolName string, tc llm.ToolCall) bool) {
	deps.Logger.Info("user prompt", "message", req.Message, "mode", req.Mode, "session", req.SessionID)

	// Default mode is read-only.
	mode := req.Mode
	if mode == "" {
		mode = "read-only"
	}

	// Get or create session.
	sess := deps.Sessions.GetOrCreate(req.SessionID)

	// Append user message.
	sess.AddMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: req.Message,
	})

	// Determine which capabilities are active and build tool filter.
	capStates := capability.ToMap(deps.Capabilities)

	// Pre-match: if the user message matches an interest trigger, narrow
	// tools to only the capabilities the interest requires.
	if deps.Catalog != nil {
		if matched := deps.Catalog.Match(req.Message); matched != nil {
			required := make(map[string]bool, len(matched.Meta.Requires))
			for _, r := range matched.Meta.Requires {
				required[r] = true
			}
			for _, cap := range deps.Capabilities {
				if !required[cap.ID] {
					capStates[cap.ID] = capability.StateOff
				}
			}
			deps.Logger.Info("pre-matched interest, scoping tools",
				"interest", matched.Meta.ID,
				"requires", matched.Meta.Requires)
		}
	}

	// Build tool-name → capability-ID mapping for ask-mode routing.
	serverToCap := make(map[string]string)
	for _, cap := range deps.Capabilities {
		serverToCap[cap.ServerName] = cap.ID
	}
	toolServerMap := make(map[string]string)
	for toolName, serverName := range deps.Router.ToolMap() {
		if capID, ok := serverToCap[serverName]; ok {
			toolServerMap[toolName] = capID
		}
	}

	// Build interest index and internal tools from the catalog.
	var interestIndex string
	var internalTools map[string]agent.InternalTool
	if deps.Catalog != nil {
		interestIndex = deps.Catalog.BuildIndex()
		deps.Logger.Debug("interest index built", "index", interestIndex, "interests", len(deps.Catalog.All()))
		if interestIndex != "" {
			// Build valid capability ID set for save validation.
			validCaps := make(map[string]bool)
			for _, cap := range deps.Capabilities {
				validCaps[cap.ID] = true
			}

			internalTools = map[string]agent.InternalTool{
				"get_interest": {
					Def:     interest.ToolDef(),
					Handler: interest.NewHandler(deps.Catalog),
				},
				"save_interest": {
					Def:           interest.SaveToolDef(),
					Handler:       interest.NewSaveHandler(deps.Catalog, deps.InterestsDir, validCaps),
					ReadWriteOnly: true,
				},
				"delete_interest": {
					Def:           interest.DeleteToolDef(),
					Handler:       interest.NewDeleteHandler(deps.Catalog, deps.InterestsDir),
					ReadWriteOnly: true,
				},
			}
		}
	}

	// Register product-specific internal tools.
	if internalTools == nil {
		internalTools = make(map[string]agent.InternalTool)
	}
	for name, tool := range deps.ExtraTools {
		internalTools[name] = tool
	}

	// Build the agent with capability + mode filtering.
	ag := agent.New(
		deps.Provider,
		deps.Router,
		agent.WithSystemPrompt(agent.BuildSystemPrompt(deps.PromptConfig, deps.Router, interestIndex, req.CanvasTabs...)),
		agent.WithModel(deps.Model),
		agent.WithLogger(deps.Logger),
		agent.WithCapabilityFilter(capStates, mode),
		agent.WithToolServerMap(toolServerMap),
		agent.WithInternalTools(internalTools),
	)

	if approvalFunc != nil {
		ag.ApprovalFunc = approvalFunc
	}

	// Collect assistant response text for session history.
	var assistantText string

	// Run agent loop, converting agent events to emitted events.
	ag.Run(ctx, sess.Messages, func(evt agent.Event) {
		switch evt.Type {
		case agent.EventText:
			assistantText += evt.Text
			emit("message", map[string]string{
				"type":    "text",
				"content": evt.Text,
			})

		case agent.EventToolStart:
			params := map[string]any{
				"type":   "tool_call",
				"tool":   evt.ToolName,
				"status": "executing",
			}
			if evt.ToolCall != nil {
				params["params"] = evt.ToolCall.Input
				params["capability"] = evt.Capability
			}
			emit("tool_call", params)

		case agent.EventToolApprovalRequired:
			emit("tool_approval_required", map[string]any{
				"type":        "tool_approval_required",
				"approval_id": evt.ApprovalID,
				"capability":  evt.Capability,
				"tool":        evt.ToolName,
				"params":      evt.ToolCall.Input,
				"description": fmt.Sprintf("%s → %s", evt.Capability, evt.ToolName),
			})

		case agent.EventToolResult:
			emit("tool_result", map[string]any{
				"type":   "tool_result",
				"tool":   evt.ToolName,
				"result": evt.ToolResult,
			})

		case agent.EventToolError:
			slog.Warn("tool call failed", "tool", evt.ToolName, "error", evt.Error)
			emit("tool_result", map[string]any{
				"type":  "tool_error",
				"tool":  evt.ToolName,
				"error": evt.Error,
			})

		case agent.EventTextClear:
			assistantText = ""
			emit("text_clear", map[string]string{
				"type": "text_clear",
			})

		case agent.EventCanvasOpen:
			if evt.Canvas != nil {
				emit("canvas_open", evt.Canvas)
			}

		case agent.EventError:
			emit("error", map[string]string{
				"type":    "error",
				"message": evt.Text,
			})

		case agent.EventDone:
			// Append assistant response to session history.
			if assistantText != "" {
				sess.AddMessage(llm.Message{
					Role:    llm.RoleAssistant,
					Content: assistantText,
				})
			}
			emit("done", map[string]string{
				"type":       "done",
				"session_id": sess.ID,
			})
		}
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
