package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
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

	// Tool routing (S7a). When ToolRoutingMode is agent.ToolRoutingInBand the
	// in-band supervisor is enabled: the model self-selects capability groups
	// via load_tools. Empty/agent.ToolRoutingOff reproduces today's behavior.
	ToolRoutingMode      string
	ToolRoutingMaxTools  int
	ToolRoutingAlwaysOn  []string
	ToolRoutingForceLoad bool
	// ToolRoutingExpandThreshold enables S8 intra-group tool-level selection:
	// groups larger than this are listed tool-by-tool in the routing menu so
	// the model can load a subset. 0 disables expansion (pure group-level S7a).
	ToolRoutingExpandThreshold int
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

// Request body limits. The canvas_tabs array is taken verbatim from the
// network client and is rendered into the system prompt before the first LLM
// call, so without these caps the client alone decides how much work the
// server performs per request (and how much of the process's memory it holds).
// The limits are far above what any legitimate host sends: the chat component
// keeps at most 5 canvas tabs open and the canvas design doc budgets ~10.
const (
	// maxChatMessageBytes bounds the POST /chat/message body before decoding.
	maxChatMessageBytes = 1 << 20 // 1 MiB

	// maxControlBodyBytes bounds the small control endpoints (session delete,
	// capability updates, approve/deny, stop), whose bodies are a handful of
	// identifiers.
	maxControlBodyBytes = 64 << 10 // 64 KiB

	// maxCanvasTabs caps the number of canvas tab summaries per request.
	maxCanvasTabs = 64

	// maxCanvasFieldLen caps each single-line canvas field rendered into the
	// prompt (tab_id, kind, name, qualifier, status, option labels/choices).
	maxCanvasFieldLen = 1024

	// maxCanvasDigestLen caps a tab's free-text digest (C5).
	maxCanvasDigestLen = 8 << 10 // 8 KiB

	// maxCanvasKeyProperties caps the key_properties map on a single tab.
	// KeyProperties is not rendered into the prompt, so this bounds the
	// decoded request only.
	maxCanvasKeyProperties = 64

	// maxCanvasOptionsPerTab / maxCanvasChoicesPerOption cap the structured
	// per-control option sets (C1 grounding) on a single tab.
	maxCanvasOptionsPerTab    = 32
	maxCanvasChoicesPerOption = 128

	// maxCanvasTotalBytes caps the summed length of all canvas strings in one
	// request. The per-field caps alone would allow 64 tabs × 8 KiB of digest
	// (plus option text) to land in the system prompt; this keeps the whole
	// client-supplied section within a sane prompt budget while leaving ample
	// room for a real canvas (~10 tabs with a paragraph each).
	maxCanvasTotalBytes = 128 << 10 // 128 KiB

	// bodyReadTimeout bounds how long a client may take to upload a request
	// body. http.MaxBytesReader caps bytes, not time, so without a deadline a
	// client can hold a request goroutine open indefinitely by trickling.
	bodyReadTimeout = 30 * time.Second
)

// validateCanvasTabs rejects a canvas_tabs payload that is larger than any
// real canvas. Prompt construction is linear in its size (see
// agent.renderCanvasContext), so this is not load-bearing for complexity, but
// it keeps the prompt — and the per-request work done before the LLM is
// contacted — proportional to what a host can legitimately have on screen.
// Returns a message safe to hand back to the client verbatim.
func validateCanvasTabs(tabs []agent.CanvasTabSummary) error {
	if len(tabs) > maxCanvasTabs {
		return fmt.Errorf("canvas_tabs has %d entries (max %d)", len(tabs), maxCanvasTabs)
	}
	total := 0
	for i, tab := range tabs {
		for _, f := range []struct {
			name  string
			value string
		}{
			{"tab_id", tab.TabID},
			{"kind", tab.Kind},
			{"name", tab.Name},
			{"qualifier", tab.Qualifier},
			{"status", tab.Status},
		} {
			if len(f.value) > maxCanvasFieldLen {
				return fmt.Errorf("canvas_tabs[%d].%s is %d bytes (max %d)", i, f.name, len(f.value), maxCanvasFieldLen)
			}
			total += len(f.value)
		}
		if len(tab.Digest) > maxCanvasDigestLen {
			return fmt.Errorf("canvas_tabs[%d].digest is %d bytes (max %d)", i, len(tab.Digest), maxCanvasDigestLen)
		}
		total += len(tab.Digest)
		if len(tab.KeyProperties) > maxCanvasKeyProperties {
			return fmt.Errorf("canvas_tabs[%d].key_properties has %d entries (max %d)", i, len(tab.KeyProperties), maxCanvasKeyProperties)
		}
		for k, v := range tab.KeyProperties {
			total += len(k) + len(v)
		}
		if len(tab.Options) > maxCanvasOptionsPerTab {
			return fmt.Errorf("canvas_tabs[%d].options has %d entries (max %d)", i, len(tab.Options), maxCanvasOptionsPerTab)
		}
		for j, o := range tab.Options {
			if len(o.Label) > maxCanvasFieldLen {
				return fmt.Errorf("canvas_tabs[%d].options[%d].label is %d bytes (max %d)", i, j, len(o.Label), maxCanvasFieldLen)
			}
			total += len(o.Label)
			if len(o.Choices) > maxCanvasChoicesPerOption {
				return fmt.Errorf("canvas_tabs[%d].options[%d].choices has %d entries (max %d)", i, j, len(o.Choices), maxCanvasChoicesPerOption)
			}
			for k, c := range o.Choices {
				if len(c) > maxCanvasFieldLen {
					return fmt.Errorf("canvas_tabs[%d].options[%d].choices[%d] is %d bytes (max %d)", i, j, k, len(c), maxCanvasFieldLen)
				}
				total += len(c)
			}
		}
		if total > maxCanvasTotalBytes {
			return fmt.Errorf("canvas_tabs carries more than %d bytes of text", maxCanvasTotalBytes)
		}
	}
	return nil
}

// decodeCanvasTabs turns the raw canvas_tabs value into tab summaries,
// refusing an oversized array before it is materialized.
//
// Decoding straight into []agent.CanvasTabSummary allocates ~128 bytes per
// element, so a 1 MiB body of `{}` entries (~350k of them) costs hundreds of
// MB of allocation churn before a length check can look at the slice — and a
// duplicate canvas_tabs key hides that work from such a check entirely, since
// encoding/json keeps the last occurrence of a repeated key. Counting the
// top-level elements with a streaming scan that stops at the cap keeps the
// work proportional to the bytes the client actually sent.
func decodeCanvasTabs(raw json.RawMessage) ([]agent.CanvasTabSummary, error) {
	trimmed := bytes.TrimSpace(raw)
	// Absent or explicitly null: no canvas, as before.
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New("canvas_tabs must be an array")
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, errors.New("canvas_tabs must be an array")
	}
	for n := 0; dec.More(); {
		n++
		if n > maxCanvasTabs {
			return nil, fmt.Errorf("canvas_tabs has more than %d entries", maxCanvasTabs)
		}
		var elem json.RawMessage
		if err := dec.Decode(&elem); err != nil {
			return nil, errors.New("canvas_tabs is not a valid array of tab summaries")
		}
	}

	var tabs []agent.CanvasTabSummary
	if err := json.Unmarshal(trimmed, &tabs); err != nil {
		return nil, errors.New("canvas_tabs is not a valid array of tab summaries")
	}
	return tabs, nil
}

// limitBody caps how many bytes and how long a handler will spend reading the
// request body. Exceeding the byte cap surfaces as an *http.MaxBytesError from
// the JSON decoder, which callers map to 413.
//
// A handler that goes on to stream a response must call clearReadDeadline once
// the body is decoded — the deadline also covers the server's background read
// for disconnect detection, so leaving it set would cancel a long-lived SSE
// stream when it expires.
func limitBody(w http.ResponseWriter, r *http.Request, max int64) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	// Not every ResponseWriter supports deadlines (httptest recorders do not);
	// the byte cap still applies when it does not.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(bodyReadTimeout))
}

// clearReadDeadline removes the body-read deadline set by limitBody.
func clearReadDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
}

// bodyTooLarge reports whether a decode error was caused by the body cap.
func bodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// writeTooLarge answers a request whose body exceeded the given cap.
func writeTooLarge(w http.ResponseWriter, max int64) {
	writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
		"message": fmt.Sprintf("request body exceeds %d bytes", max),
	})
}

// requireJSON rejects state-changing requests whose Content-Type is not
// application/json, writing 415 and returning false.
//
// This is the service's cross-site request forgery defence. The JSON decoder
// ignores Content-Type, so without this check a hostile page could reach these
// handlers with a CORS "simple request" (text/plain fetch or an HTML form),
// which the browser sends with the victim's ambient credentials (cookies,
// cached HTTP auth) and without a preflight. A cross-origin request carrying
// application/json always triggers a CORS preflight, which this service does
// not answer, so requiring it confines these endpoints to same-origin callers
// (or origins a fronting proxy explicitly allows via CORS). It must run before
// the body is read or any state is touched.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{
			"message": "Content-Type must be application/json",
		})
		return false
	}
	return true
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
	if !requireJSON(w, r) {
		return
	}
	// Bound the body before decoding, and the canvas payload before anything
	// is done with it: both feed system-prompt construction, which runs
	// synchronously on this goroutine before the first LLM call. Rejecting
	// here happens before the session is created or the user message is
	// appended, so an oversized request mutates no state.
	limitBody(w, r, maxChatMessageBytes)

	// canvas_tabs is held as raw JSON through the decode so the element cap can
	// be applied before the tab structs are allocated (see decodeCanvasTabs).
	var body struct {
		Message    string          `json:"message"`
		Mode       string          `json:"mode,omitempty"`
		SessionID  string          `json:"session_id,omitempty"`
		CanvasTabs json.RawMessage `json:"canvas_tabs,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if bodyTooLarge(err) {
			writeTooLarge(w, maxChatMessageBytes)
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}
	if body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "message is required"})
		return
	}
	tabs, err := decodeCanvasTabs(body.CanvasTabs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	req := ChatMessageRequest{
		Message:    body.Message,
		Mode:       body.Mode,
		SessionID:  body.SessionID,
		CanvasTabs: tabs,
	}
	if err := validateCanvasTabs(req.CanvasTabs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	// The body is in hand; drop the upload deadline so it cannot fire mid-stream.
	clearReadDeadline(w)

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

	// Relay any configured per-request headers (e.g. a host-minted identity
	// token) from this chat request onto outbound MCP calls made while serving
	// it. Header names are an operator-configured allowlist; values are opaque.
	if fwd := s.deps.Router.CollectForwardableHeaders(r.Header); len(fwd) > 0 {
		ctx = mcpclient.WithForwardedHeaders(ctx, fwd)
	}

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
	if !requireJSON(w, r) {
		return
	}
	limitBody(w, r, maxControlBodyBytes)

	var body struct {
		SessionID string `json:"session_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&body)
	if bodyTooLarge(err) {
		writeTooLarge(w, maxControlBodyBytes)
		return
	}
	if err != nil || body.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "session_id is required"})
		return
	}
	s.deps.Sessions.Delete(body.SessionID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "session cleared"})
}

// GetChatCapabilities returns the current capability states.
//
// Optional query param: ?mode=read-only|read-write (default read-only).
// The response includes per-capability tools_count + read_only_tools_count
// plus a tool_budget summary so the UI can show usage and prevent enables
// that would exceed the LLM's hard 128-tool limit.
func (s *Server) GetChatCapabilities(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode != "read-write" {
		mode = "read-only"
	}

	caps := s.deps.Capabilities
	router := s.deps.Router

	connectedServers := router.ConnectedServers()
	serverConnected := make(map[string]bool, len(connectedServers))
	for _, name := range connectedServers {
		serverConnected[name] = true
	}

	// Group all router tools by server name so we can compute totals and
	// read-only counts per capability without scanning N×M times.
	allTools := router.Tools()
	toolMap := router.ToolMap()
	type counts struct{ total, readOnly int }
	perServer := make(map[string]*counts)
	for _, t := range allTools {
		srv, ok := toolMap[t.Name]
		if !ok {
			continue
		}
		c := perServer[srv]
		if c == nil {
			c = &counts{}
			perServer[srv] = c
		}
		c.total++
		if t.ReadOnlyHint {
			c.readOnly++
		}
	}

	for i := range caps {
		caps[i].Available = serverConnected[caps[i].ServerName]
		c := perServer[caps[i].ServerName]
		if c != nil {
			caps[i].ToolsCount = c.total
			caps[i].ReadOnlyToolsCount = c.readOnly
		} else {
			caps[i].ToolsCount = 0
			caps[i].ReadOnlyToolsCount = 0
		}
	}

	// Budget reporting. When in-band tool routing is enabled the per-request
	// tool list is the subset the model loads via load_tools, not the sum of
	// every enabled capability. The binding limit is therefore the largest
	// single capability (the smallest set the model can load) rather than the
	// total — otherwise the UI's read-write toggle / enable guards would block
	// legitimate multi-server deployments that only ever send a routed subset
	// per turn. See docs/high-tool-count-scaling.md.
	routingOn := s.deps.ToolRoutingMode == agent.ToolRoutingInBand
	roTotal, roPerCap := computeToolBudget(caps, "read-only", router)
	rwTotal, rwPerCap := computeToolBudget(caps, "read-write", router)
	usedReadOnly := effectiveToolBudget(roTotal, roPerCap, routingOn)
	usedReadWrite := effectiveToolBudget(rwTotal, rwPerCap, routingOn)

	used := usedReadOnly
	if mode == "read-write" {
		used = usedReadWrite
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"capabilities": caps,
		"total_tools":  used,
		"tool_budget": map[string]any{
			"used": used,
			"max":  agent.MaxToolsPerRequest,
			"mode": mode,
		},
		// Both-mode budgets so the UI can preview the impact of a mode
		// switch without an extra round-trip.
		"tool_budgets": map[string]any{
			"read_only":  map[string]int{"used": usedReadOnly, "max": agent.MaxToolsPerRequest},
			"read_write": map[string]int{"used": usedReadWrite, "max": agent.MaxToolsPerRequest},
		},
	})
}

// PostChatCapabilities updates capability states.
//
// Validates that the resulting tool count does not exceed
// agent.MaxToolsPerRequest in the requested mode (defaults to read-only).
// Returns 409 with a helpful message when the change would blow the budget,
// without mutating server state.
func (s *Server) PostChatCapabilities(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	limitBody(w, r, maxControlBodyBytes)

	var body struct {
		Capabilities map[string]string `json:"capabilities"`
		Mode         string            `json:"mode,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if bodyTooLarge(err) {
			writeTooLarge(w, maxControlBodyBytes)
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	mode := body.Mode
	if mode != "read-write" {
		mode = "read-only"
	}

	// Build the proposed capability set without mutating shared state.
	proposed := make([]capability.Capability, len(s.deps.Capabilities))
	copy(proposed, s.deps.Capabilities)
	for i := range proposed {
		if newState, ok := body.Capabilities[proposed[i].ID]; ok {
			st := capability.State(newState)
			if st.Valid() {
				proposed[i].State = st
			}
		}
	}

	// Compute the resulting tool count. With in-band routing on, the binding
	// limit is the largest single capability (what the model loads per turn),
	// not the sum across enabled capabilities.
	total, perCap := computeToolBudget(proposed, mode, s.deps.Router)
	routingOn := s.deps.ToolRoutingMode == agent.ToolRoutingInBand
	used := effectiveToolBudget(total, perCap, routingOn)
	if used > agent.MaxToolsPerRequest {
		msg := fmt.Sprintf(
			"Enabling these capabilities would use %d tools (max %d) in %s mode. Disable a capability or switch mode to fit within the budget.",
			used, agent.MaxToolsPerRequest, mode,
		)
		if routingOn {
			// Routing splits tools by capability, so the only unroutable case
			// is one server whose own tools exceed the cap.
			msg = fmt.Sprintf(
				"A single capability exposes %d tools, over the %d-tool per-request limit (mode=%s). Tool routing cannot split one server — reduce that server's tool set.",
				used, agent.MaxToolsPerRequest, mode,
			)
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"message": msg,
			"tool_budget": map[string]any{
				"used":           used,
				"max":            agent.MaxToolsPerRequest,
				"mode":           mode,
				"per_capability": perCap,
			},
		})
		return
	}

	// Commit the change.
	for i := range s.deps.Capabilities {
		s.deps.Capabilities[i].State = proposed[i].State
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "capabilities updated",
		"tool_budget": map[string]any{
			"used": used,
			"max":  agent.MaxToolsPerRequest,
			"mode": mode,
		},
	})
}

// computeToolBudget returns the total number of tools that would be sent to
// the LLM given the supplied capability states and mode, plus a per-capability
// breakdown for diagnostics. Internal tools are not counted here — they
// account for a small fixed overhead (typically 3-7).
func computeToolBudget(caps []capability.Capability, mode string, router mcpclient.ToolRouter) (int, map[string]int) {
	srvToCap := make(map[string]string, len(caps))
	off := make(map[string]bool, len(caps))
	askOnWrite := make(map[string]bool, len(caps))
	for _, c := range caps {
		srvToCap[c.ServerName] = c.ID
		switch c.State {
		case capability.StateOff:
			off[c.ID] = true
		case capability.StateAskOnWrite:
			askOnWrite[c.ID] = true
		}
	}

	perCap := make(map[string]int)
	total := 0
	toolMap := router.ToolMap()
	for _, t := range router.Tools() {
		srv := toolMap[t.Name]
		capID, ok := srvToCap[srv]
		if !ok || off[capID] {
			continue
		}
		// Write tools count toward the budget when the global mode is
		// read-write OR this capability is ask-on-write (writes are sent
		// to the LLM in both cases).
		allowWrites := mode == "read-write" || askOnWrite[capID]
		if !allowWrites && !t.ReadOnlyHint {
			continue
		}
		perCap[capID]++
		total++
	}
	return total, perCap
}

// buildRoutingGroups derives the in-band routing group menu (S7a) from the
// connected, enabled capabilities. The menu is built from only the tools that
// are callable in the given mode (S6b): in read-only mode a server's write
// tools are dropped exactly as filteredTools() drops them, so a read-only-
// annotated server (ReadOnlyHint) presents a smaller group and the model is
// never offered tools it cannot call this turn. ask-on-write capabilities
// surface their writes regardless of mode, matching filteredTools().
func buildRoutingGroups(caps []capability.Capability, capStates map[string]capability.State, mode string, connected map[string]bool, toolServerMap map[string]string, router mcpclient.ToolRouter, expandThreshold int) []capability.Group {
	groupEnabled := make(map[string]bool, len(caps))
	askOnWrite := make(map[string]bool, len(caps))
	for _, c := range caps {
		if connected[c.ServerName] && capStates[c.ID] != capability.StateOff {
			groupEnabled[c.ID] = true
		}
		if capStates[c.ID] == capability.StateAskOnWrite {
			askOnWrite[c.ID] = true
		}
	}
	toolsByCap := make(map[string][]capability.ToolInfo)
	for _, t := range router.Tools() {
		capID, ok := toolServerMap[t.Name]
		if !ok {
			continue
		}
		allowWrites := mode == "read-write" || askOnWrite[capID]
		if !allowWrites && !t.ReadOnlyHint {
			continue
		}
		toolsByCap[capID] = append(toolsByCap[capID], capability.ToolInfo{Name: t.Name, Description: t.Description})
	}
	return capability.BuildGroupsExpanding(caps, groupEnabled, toolsByCap, expandThreshold)
}

// effectiveToolBudget returns the tool count that actually constrains a
// request. Without routing it is the total across all enabled capabilities.
// With in-band routing the model loads capabilities on demand via load_tools
// and the agent caps each turn at MaxToolsPerRequest, so the real floor is the
// largest single capability — the smallest set the model can load. Reporting
// the max (not the sum) is what lets a deployment enable more than 128 tools
// total while keeping the UI's budget guards meaningful (they then only block
// a single server whose own tools exceed the cap, which routing cannot split).
func effectiveToolBudget(total int, perCap map[string]int, routingOn bool) int {
	if !routingOn {
		return total
	}
	max := 0
	for _, n := range perCap {
		if n > max {
			max = n
		}
	}
	return max
}

// PostChatApprove approves a pending tool call.
func (s *Server) PostChatApprove(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	limitBody(w, r, maxControlBodyBytes)

	var body struct {
		ApprovalID string `json:"approval_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&body)
	if bodyTooLarge(err) {
		writeTooLarge(w, maxControlBodyBytes)
		return
	}
	if err != nil || body.ApprovalID == "" {
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
	if !requireJSON(w, r) {
		return
	}
	limitBody(w, r, maxControlBodyBytes)

	var body struct {
		ApprovalID string `json:"approval_id"`
	}
	err := json.NewDecoder(r.Body).Decode(&body)
	if bodyTooLarge(err) {
		writeTooLarge(w, maxControlBodyBytes)
		return
	}
	if err != nil || body.ApprovalID == "" {
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
	if !requireJSON(w, r) {
		return
	}
	limitBody(w, r, maxControlBodyBytes)

	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); bodyTooLarge(err) {
		writeTooLarge(w, maxControlBodyBytes)
		return
	}
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

	// The canvas caps apply wherever the request came from, not just from
	// PostChatMessage: RunChat is exported, so an embedder that decodes its own
	// transport reaches the prompt builder through here. Checked before the
	// session is touched, so a rejected request appends no message.
	if err := validateCanvasTabs(req.CanvasTabs); err != nil {
		deps.Logger.Warn("rejecting chat request", "error", err)
		emit("error", map[string]string{"type": "error", "message": err.Error()})
		return
	}

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

	// Compute the currently-enabled capability set (MCP server connected
	// AND not turned off by the user). This is recomputed per request so
	// interest matching tracks runtime state — newly-connected MCP servers
	// become available immediately without a catalog reload.
	enabledCaps := make(map[string]bool, len(deps.Capabilities))
	connectedServers := make(map[string]bool)
	for _, name := range deps.Router.ConnectedServers() {
		connectedServers[name] = true
	}
	for _, cap := range deps.Capabilities {
		if connectedServers[cap.ServerName] && capStates[cap.ID] != capability.StateOff {
			enabledCaps[cap.ID] = true
		}
	}

	// Pre-match: if the user message matches an interest trigger, narrow
	// tools to only the capabilities the interest requires.
	if deps.Catalog != nil {
		if matched := deps.Catalog.Match(req.Message, enabledCaps); matched != nil {
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
		interestIndex = deps.Catalog.BuildIndex(enabledCaps)
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

	// In-band tool routing (S7a): derive the capability group menu from the
	// currently-enabled capabilities (connected AND not turned off — this
	// reflects any interest pre-match narrowing above, so the two compose) and
	// render its index for the system prompt. When routing is off, groupIndex
	// stays empty and BuildSystemPromptWithRouting is byte-identical to today.
	var groupIndex string
	var groups []capability.Group
	if deps.ToolRoutingMode == agent.ToolRoutingInBand {
		groups = buildRoutingGroups(deps.Capabilities, capStates, mode, connectedServers, toolServerMap, deps.Router, deps.ToolRoutingExpandThreshold)
		groupIndex = capability.RenderGroupIndex(groups)
	}

	// Build the agent with capability + mode filtering.
	opts := []agent.Option{
		agent.WithSystemPrompt(agent.BuildSystemPromptWithRouting(deps.PromptConfig, deps.Router, interestIndex, groupIndex, req.CanvasTabs...)),
		agent.WithModel(deps.Model),
		agent.WithLogger(deps.Logger),
		agent.WithCapabilityFilter(capStates, mode),
		agent.WithToolServerMap(toolServerMap),
		agent.WithInternalTools(internalTools),
	}
	if deps.ToolRoutingMode == agent.ToolRoutingInBand {
		opts = append(opts, agent.WithToolRouting(
			deps.ToolRoutingMode, groups, deps.ToolRoutingAlwaysOn,
			deps.ToolRoutingMaxTools, deps.ToolRoutingForceLoad))
	}
	ag := agent.New(deps.Provider, deps.Router, opts...)

	if approvalFunc != nil {
		ag.ApprovalFunc = approvalFunc
	}

	// Collect assistant response text for session history. A builder, not
	// `text += evt.Text`: one EventText arrives per streamed delta, and
	// concatenation would copy the whole response per delta (quadratic in the
	// number of deltas) for a response the model, not this code, sizes.
	var assistantText strings.Builder

	// Run agent loop, converting agent events to emitted events.
	ag.Run(ctx, sess.Messages, func(evt agent.Event) {
		switch evt.Type {
		case agent.EventText:
			assistantText.WriteString(evt.Text)
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
			assistantText.Reset()
			emit("text_clear", map[string]string{
				"type": "text_clear",
			})

		case agent.EventCanvasOpen:
			if evt.Canvas != nil {
				emit("canvas_open", evt.Canvas)
			}

		case agent.EventOpenNav:
			if evt.OpenNav != nil {
				emit("open_nav", evt.OpenNav)
			}

		case agent.EventError:
			emit("error", map[string]string{
				"type":    "error",
				"message": evt.Error,
			})

		case agent.EventDone:
			// Append assistant response to session history.
			if text := assistantText.String(); text != "" {
				sess.AddMessage(llm.Message{
					Role:    llm.RoleAssistant,
					Content: text,
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
