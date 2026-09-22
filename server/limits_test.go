package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

// limitsServer builds a Server with a mock provider so a request that passes
// validation runs to completion, and returns the provider for prompt checks.
func limitsServer() (*Server, *llm.MockProvider) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses:    [][]llm.StreamEvent{llm.MockTextResponse("ok")},
	}
	return New(&ChatDeps{
		Sessions: session.NewManager(10),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slogDiscard(),
	}), provider
}

func postChatMessage(srv *Server, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/chat/message", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatMessage(w, req)
	return w
}

// canvasTabsBody builds a /chat/message body whose canvas_tabs array holds n
// two-byte objects — the cheapest amplification input a client can send.
func canvasTabsBody(n int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"message":"hi","canvas_tabs":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("{}")
	}
	b.WriteString("]}")
	return b.Bytes()
}

// TestPostChatMessageRejectsTooManyCanvasTabs covers the request-side half of
// the canvas prompt-amplification fix: canvas_tabs is taken verbatim from the
// network client and rendered into the system prompt before the first LLM
// call, so an oversized array must be rejected up front — and before the
// session is created or the user message appended, so a rejected request
// leaves no state behind.
func TestPostChatMessageRejectsTooManyCanvasTabs(t *testing.T) {
	srv, provider := limitsServer()

	w := postChatMessage(srv, canvasTabsBody(maxCanvasTabs+1))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "canvas_tabs") {
		t.Errorf("error message should name canvas_tabs, got %s", w.Body.String())
	}
	if n := srv.deps.Sessions.Count(); n != 0 {
		t.Errorf("rejected request created %d session(s); validation must precede session mutation", n)
	}
	if len(provider.Calls) != 0 {
		t.Errorf("rejected request reached the provider")
	}
}

// TestPostChatMessageRejectsOversizedBody verifies the body cap fires before
// the JSON is decoded, so the client cannot decide how much work the server
// does per request.
func TestPostChatMessageRejectsOversizedBody(t *testing.T) {
	srv, _ := limitsServer()

	// ~350k empty tabs: what fits in a body just over the cap.
	w := postChatMessage(srv, canvasTabsBody(maxChatMessageBytes/3))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", w.Code, w.Body.String())
	}
	if n := srv.deps.Sessions.Count(); n != 0 {
		t.Errorf("rejected request created %d session(s)", n)
	}
}

// TestDecodeCanvasTabsRejectsWithoutMaterializing is the reason canvas_tabs is
// held as json.RawMessage through the decode: a post-decode length check would
// already have paid ~128 bytes per element (tens of MB for a 1 MiB body), and
// a duplicate canvas_tabs key would hide that work from it entirely, since
// encoding/json keeps the last occurrence of a repeated key. The streaming
// count must stop at the cap instead.
func TestDecodeCanvasTabsRejectsWithoutMaterializing(t *testing.T) {
	const tabs = 300_000
	var raw bytes.Buffer
	raw.WriteByte('[')
	for i := 0; i < tabs; i++ {
		if i > 0 {
			raw.WriteByte(',')
		}
		raw.WriteString("{}")
	}
	raw.WriteByte(']')

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := decodeCanvasTabs(raw.Bytes())
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc

	if err == nil {
		t.Fatalf("%d canvas tabs accepted (%d returned)", tabs, len(got))
	}
	// Materializing the array would cost ~38 MB here; the count scan reads
	// only the first maxCanvasTabs+1 elements.
	t.Logf("%d tabs (%d bytes of JSON) rejected after %d bytes allocated", tabs, raw.Len(), allocated)
	if allocated > 64<<10 {
		t.Errorf("counting %d canvas tabs allocated %d bytes; the array must not be materialized", tabs, allocated)
	}
}

// TestPostChatMessageAcceptsLargeRealisticCanvas guards against the caps being
// so tight that a legitimate canvas is refused: the chat component keeps at
// most 5 tabs open, so a request with the maximum tab count, a paragraph of
// digest each and option sets must still be served — and the tabs must reach
// the system prompt.
func TestPostChatMessageAcceptsLargeRealisticCanvas(t *testing.T) {
	srv, provider := limitsServer()

	tabs := make([]agent.CanvasTabSummary, maxCanvasTabs)
	for i := range tabs {
		tabs[i] = agent.CanvasTabSummary{
			TabID:     fmt.Sprintf("volume::vol%d::", i),
			Kind:      "volume",
			Name:      fmt.Sprintf("vol%d", i),
			Qualifier: "on SVM svm1",
			Status:    "ok",
			Digest:    strings.Repeat("x", 1024),
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
	if len(provider.Calls) == 0 {
		t.Fatal("provider was never called")
	}
	sys := provider.Calls[0].System
	for _, want := range []string{"Canvas Context", "| vol0 |", fmt.Sprintf("| vol%d |", maxCanvasTabs-1)} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

// TestPostChatMessageCanvasTabsShapes covers the wire shapes the raw-JSON
// decode has to keep behaving as before.
func TestPostChatMessageCanvasTabsShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"absent", `{"message":"hi"}`, http.StatusOK},
		{"null", `{"message":"hi","canvas_tabs":null}`, http.StatusOK},
		{"empty array", `{"message":"hi","canvas_tabs":[]}`, http.StatusOK},
		{"one tab", `{"message":"hi","canvas_tabs":[{"tab_id":"t","kind":"volume","name":"vol1"}]}`, http.StatusOK},
		{"not an array", `{"message":"hi","canvas_tabs":{}}`, http.StatusBadRequest},
		{"wrong element type", `{"message":"hi","canvas_tabs":[1,2]}`, http.StatusBadRequest},
		{"malformed", `{"message":"hi","canvas_tabs":[{`, http.StatusBadRequest},
		{"no message", `{"canvas_tabs":[]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := limitsServer()
			w := postChatMessage(srv, []byte(tc.body))
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
			}
		})
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

	// The aggregate budget catches what the per-field caps allow in bulk:
	// maxCanvasTabs tabs each with a maximal digest would otherwise put
	// half a megabyte of client text into the system prompt.
	bulk := make([]agent.CanvasTabSummary, maxCanvasTabs)
	for i := range bulk {
		bulk[i] = agent.CanvasTabSummary{Name: "n", Digest: strings.Repeat("d", maxCanvasDigestLen)}
	}
	if err := validateCanvasTabs(bulk); err == nil {
		t.Errorf("%d tabs x %d-byte digests accepted; aggregate budget not enforced", maxCanvasTabs, maxCanvasDigestLen)
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

// TestRunChatRejectsOversizedCanvasTabs covers the exported entry point used by
// embedders that decode their own transport: the caps must not depend on
// PostChatMessage having run.
func TestRunChatRejectsOversizedCanvasTabs(t *testing.T) {
	srv, provider := limitsServer()

	var events []string
	RunChat(context.Background(), srv.deps, ChatMessageRequest{
		Message:    "hi",
		CanvasTabs: make([]agent.CanvasTabSummary, maxCanvasTabs+1),
	}, func(event string, _ any) { events = append(events, event) }, nil)

	if len(provider.Calls) != 0 {
		t.Errorf("oversized canvas reached the provider")
	}
	if len(events) != 1 || events[0] != "error" {
		t.Errorf("events = %v, want a single error event", events)
	}
	if n := srv.deps.Sessions.Count(); n != 0 {
		t.Errorf("rejected request created %d session(s)", n)
	}
}

// TestControlEndpointsBoundBodies verifies the small control endpoints also
// stop reading at their cap instead of accepting an unbounded body.
func TestControlEndpointsBoundBodies(t *testing.T) {
	oversized := append(append([]byte(`{"session_id":"`),
		bytes.Repeat([]byte("a"), maxControlBodyBytes+1)...), []byte(`"}`)...)

	srv, _ := limitsServer()
	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"/chat/session":      srv.DeleteChatSession,
		"/chat/capabilities": srv.PostChatCapabilities,
		"/chat/approve":      srv.PostChatApprove,
		"/chat/deny":         srv.PostChatDeny,
		"/chat/stop":         srv.PostChatStop,
	}
	for path, h := range handlers {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(oversized))
			w := httptest.NewRecorder()
			h(w, req)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("%s status = %d, want 413; body=%s", path, w.Code, w.Body.String())
			}
		})
	}
}
