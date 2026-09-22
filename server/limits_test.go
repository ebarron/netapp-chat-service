package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

// limitsServer builds a Server with a mock provider so a request that passes
// validation runs to completion.
func limitsServer() *Server {
	return New(&ChatDeps{
		Sessions: session.NewManager(10),
		Provider: &llm.MockProvider{
			ProviderName: "mock",
			Responses:    [][]llm.StreamEvent{llm.MockTextResponse("ok")},
		},
		Router: mcpclient.NewMockRouter(nil),
		Logger: slogDiscard(),
	})
}

func postChatMessage(srv *Server, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/chat/message", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatMessage(w, req)
	return w
}

// TestPostChatMessageRejectsTooManyCanvasTabs covers the request-side half of
// the canvas prompt-amplification fix: canvas_tabs is taken verbatim from the
// network client and rendered into the system prompt before the first LLM
// call, so an oversized array must be rejected up front — and before the
// session is created or the user message appended, so a rejected request
// leaves no state behind.
func TestPostChatMessageRejectsTooManyCanvasTabs(t *testing.T) {
	srv := limitsServer()

	body, err := json.Marshal(ChatMessageRequest{
		Message:    "hello",
		CanvasTabs: make([]agent.CanvasTabSummary, maxCanvasTabs+1),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := postChatMessage(srv, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "canvas_tabs") {
		t.Errorf("error message should name canvas_tabs, got %s", w.Body.String())
	}
	if n := srv.deps.Sessions.Count(); n != 0 {
		t.Errorf("rejected request created %d session(s); validation must precede session mutation", n)
	}
}

// TestPostChatMessageRejectsOversizedBody verifies the body cap fires before
// the JSON is decoded, so the client cannot decide how much work the server
// does per request.
func TestPostChatMessageRejectsOversizedBody(t *testing.T) {
	srv := limitsServer()

	// A canvas_tabs array of two-byte objects — the cheapest possible
	// amplification input — padded past the body cap.
	var b bytes.Buffer
	b.WriteString(`{"message":"hi","canvas_tabs":[`)
	for b.Len() < maxChatMessageBytes+1024 {
		if b.Len() > len(`{"message":"hi","canvas_tabs":[`) {
			b.WriteByte(',')
		}
		b.WriteString("{}")
	}
	b.WriteString("]}")

	w := postChatMessage(srv, b.Bytes())
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}
	if n := srv.deps.Sessions.Count(); n != 0 {
		t.Errorf("rejected request created %d session(s)", n)
	}
}

// TestPostChatMessageAcceptsCanvasTabsAtLimit guards against the caps being so
// tight that a legitimate canvas is refused: the chat component keeps at most
// 5 tabs open, and a request right at the cap must still be served.
func TestPostChatMessageAcceptsCanvasTabsAtLimit(t *testing.T) {
	srv := limitsServer()

	tabs := make([]agent.CanvasTabSummary, maxCanvasTabs)
	for i := range tabs {
		tabs[i] = agent.CanvasTabSummary{
			TabID:     fmt.Sprintf("volume::vol%d::", i),
			Kind:      "volume",
			Name:      fmt.Sprintf("vol%d", i),
			Qualifier: "on SVM svm1",
			Status:    "ok",
			Digest:    strings.Repeat("x", maxCanvasDigestLen),
			Options: []agent.CanvasControlOptions{
				{Label: "Provider", Choices: []string{"OpenAI", "Anthropic"}},
			},
		}
	}
	body, err := json.Marshal(ChatMessageRequest{Message: "hello", CanvasTabs: tabs})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := postChatMessage(srv, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if n := srv.deps.Sessions.Count(); n == 0 {
		t.Error("accepted request did not reach the session/agent path")
	}
}

// TestValidateCanvasTabsFieldLimits checks each per-field cap independently, so
// a single tab cannot carry an arbitrarily large payload into the prompt.
func TestValidateCanvasTabsFieldLimits(t *testing.T) {
	long := strings.Repeat("a", maxCanvasFieldLen+1)

	choices := make([]string, maxCanvasChoicesPerOption+1)
	for i := range choices {
		choices[i] = "c"
	}
	keyProps := make(map[string]string, maxCanvasKeyProperties+1)
	for i := 0; i <= maxCanvasKeyProperties; i++ {
		keyProps[fmt.Sprintf("k%d", i)] = "v"
	}

	cases := []struct {
		name string
		tab  agent.CanvasTabSummary
	}{
		{"tab_id", agent.CanvasTabSummary{TabID: long}},
		{"kind", agent.CanvasTabSummary{Kind: long}},
		{"name", agent.CanvasTabSummary{Name: long}},
		{"qualifier", agent.CanvasTabSummary{Qualifier: long}},
		{"status", agent.CanvasTabSummary{Status: long}},
		{"digest", agent.CanvasTabSummary{Digest: strings.Repeat("d", maxCanvasDigestLen+1)}},
		{"key_properties", agent.CanvasTabSummary{KeyProperties: keyProps}},
		{"options", agent.CanvasTabSummary{Options: make([]agent.CanvasControlOptions, maxCanvasOptionsPerTab+1)}},
		{"option label", agent.CanvasTabSummary{Options: []agent.CanvasControlOptions{{Label: long}}}},
		{"option choices", agent.CanvasTabSummary{Options: []agent.CanvasControlOptions{{Label: "L", Choices: choices}}}},
		{"choice value", agent.CanvasTabSummary{Options: []agent.CanvasControlOptions{{Label: "L", Choices: []string{long}}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateCanvasTabs([]agent.CanvasTabSummary{tc.tab}); err == nil {
				t.Errorf("oversized %s accepted", tc.name)
			}
		})
	}

	// A realistic canvas must pass untouched.
	ok := []agent.CanvasTabSummary{
		{TabID: "volume::vol1::on SVM svm1", Kind: "volume", Name: "vol1", Qualifier: "on SVM svm1", Status: "warning"},
		{TabID: "nav", Kind: "nav-view", Name: "AI Settings", Qualifier: "/settings/ai",
			Digest:  "3 rules enabled, 1 disabled.",
			Options: []agent.CanvasControlOptions{{Label: "Provider", Choices: []string{"OpenAI", "Anthropic"}}}},
	}
	if err := validateCanvasTabs(ok); err != nil {
		t.Errorf("legitimate canvas rejected: %v", err)
	}
	if err := validateCanvasTabs(nil); err != nil {
		t.Errorf("absent canvas_tabs rejected: %v", err)
	}
}

// TestControlEndpointsBoundBodies verifies the small control endpoints also
// stop reading at their cap instead of accepting an unbounded body.
func TestControlEndpointsBoundBodies(t *testing.T) {
	oversized := append(append([]byte(`{"session_id":"`),
		bytes.Repeat([]byte("a"), maxControlBodyBytes+1)...), []byte(`"}`)...)

	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"/chat/session":      limitsServer().DeleteChatSession,
		"/chat/capabilities": limitsServer().PostChatCapabilities,
		"/chat/approve":      limitsServer().PostChatApprove,
		"/chat/deny":         limitsServer().PostChatDeny,
	}
	for path, h := range handlers {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(oversized))
			w := httptest.NewRecorder()
			h(w, req)
			if w.Code == http.StatusOK {
				t.Errorf("%s accepted an oversized body (status 200)", path)
			}
		})
	}
}
