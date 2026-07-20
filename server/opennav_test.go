package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

type emitted struct {
	event string
	data  any
}

// TestRunChat_OpenNavSSE verifies the open_nav_view ExtraTool, when called by
// the model, is relayed by RunChat as an "open_nav" SSE event carrying the
// destination — alongside (not replacing) the existing event stream.
func TestRunChat_OpenNavSSE(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-nav", agent.OpenNavToolName, map[string]any{"destination": "settings/general"}),
			llm.MockTextResponse("Done."),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	deps := &ChatDeps{
		Sessions: session.NewManager(10),
		Provider: provider,
		Router:   router,
		Logger:   slogDiscard(),
		ExtraTools: map[string]agent.InternalTool{
			agent.OpenNavToolName: agent.NewOpenNavTool(),
		},
	}

	var events []emitted
	RunChat(context.Background(), deps, ChatMessageRequest{Message: "go to general settings"},
		func(event string, data any) { events = append(events, emitted{event, data}) }, nil)

	var navCount, doneCount int
	var dest string
	for _, e := range events {
		switch e.event {
		case "open_nav":
			navCount++
			// The payload must serialize as {"destination": "..."} — exactly
			// the wire shape the frontend parses.
			b, err := json.Marshal(e.data)
			if err != nil {
				t.Fatalf("marshal open_nav: %v", err)
			}
			var p struct {
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatalf("open_nav payload not valid JSON: %v", err)
			}
			dest = p.Destination
		case "done":
			doneCount++
		}
	}

	if navCount != 1 {
		t.Fatalf("expected exactly 1 open_nav event, got %d (events=%+v)", navCount, events)
	}
	if dest != "settings/general" {
		t.Errorf("open_nav destination = %q, want %q", dest, "settings/general")
	}
	if doneCount != 1 {
		t.Errorf("existing 'done' event must still serialize; got %d", doneCount)
	}
}

// TestRunChat_NoOpenNav_BackwardCompat verifies that without the open_nav_view
// tool registered, a normal turn emits no open_nav event (byte-for-byte event
// set unchanged for existing consumers).
func TestRunChat_NoOpenNav_BackwardCompat(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses:    [][]llm.StreamEvent{llm.MockTextResponse("hi")},
	}
	deps := &ChatDeps{
		Sessions: session.NewManager(10),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slogDiscard(),
	}
	var events []emitted
	RunChat(context.Background(), deps, ChatMessageRequest{Message: "hello"},
		func(event string, data any) { events = append(events, emitted{event, data}) }, nil)

	for _, e := range events {
		if e.event == "open_nav" {
			t.Errorf("unexpected open_nav event for a plain turn: %+v", e)
		}
	}
}

// TestRunChat_CanvasDigestRelay verifies the optional C5 `digest` field on a
// canvas_tabs summary flows into the system prompt when present and is omitted
// when absent.
func TestRunChat_CanvasDigestRelay(t *testing.T) {
	newDeps := func(provider llm.Provider) *ChatDeps {
		return &ChatDeps{
			Sessions: session.NewManager(10),
			Provider: provider,
			Router:   mcpclient.NewMockRouter(nil),
			Logger:   slogDiscard(),
		}
	}

	// With digest.
	p1 := &llm.MockProvider{ProviderName: "mock", Responses: [][]llm.StreamEvent{llm.MockTextResponse("ok")}}
	RunChat(context.Background(), newDeps(p1), ChatMessageRequest{
		Message: "what's on screen?",
		CanvasTabs: []agent.CanvasTabSummary{
			{TabID: "nav", Kind: "nav-view", Name: "Alerting", Qualifier: "/alerting",
				Digest: "3 rules enabled, 1 disabled."},
		},
	}, func(string, any) {}, nil)
	if len(p1.Calls) == 0 {
		t.Fatal("provider never called")
	}
	if !strings.Contains(p1.Calls[0].System, "3 rules enabled") {
		t.Error("system prompt should contain the digest text when present")
	}

	// Without digest.
	p2 := &llm.MockProvider{ProviderName: "mock", Responses: [][]llm.StreamEvent{llm.MockTextResponse("ok")}}
	RunChat(context.Background(), newDeps(p2), ChatMessageRequest{
		Message: "what's on screen?",
		CanvasTabs: []agent.CanvasTabSummary{
			{TabID: "nav", Kind: "nav-view", Name: "Alerting", Qualifier: "/alerting"},
		},
	}, func(string, any) {}, nil)
	if strings.Contains(p2.Calls[0].System, "Additional detail") {
		t.Error("system prompt should not contain the digest block when absent")
	}
}
