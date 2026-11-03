package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

const terminalCanvas = "```canvas-dashboard\n" +
	`{"type":"dashboard","title":"Placement","panels":[{"type":"callout","title":"Ready","body":"Choose an action."}]}` +
	"\n```"

func terminalTool(name, result string, err error) InternalTool {
	return InternalTool{
		Def: llm.ToolDef{
			Name:   name,
			Schema: json.RawMessage(`{"type":"object"}`),
		},
		Handler: func(context.Context, json.RawMessage) (string, error) {
			return result, err
		},
		EmitResult: true,
	}
}

func eventIndex(events []Event, typ EventType, tool string) int {
	for i, event := range events {
		if event.Type == typ && (tool == "" || event.ToolName == tool) {
			return i
		}
	}
	return -1
}

func TestTerminalInteractiveRenderStopsAfterValidatedCanvas(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
		llm.MockTextResponse("must not be called"),
	}}
	ag := New(
		provider,
		mcpclient.NewMockRouter(nil),
		WithInternalTools(map[string]InternalTool{
			"render_placement": terminalTool("render_placement", terminalCanvas, nil),
		}),
		WithTerminalTools("render_placement"),
	)

	var events []Event
	var canvasFlushedAt, doneAt time.Time
	ag.Run(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "show placement"}}, func(event Event) {
		events = append(events, event)
		switch event.Type {
		case EventCanvasOpen:
			canvasFlushedAt = time.Now()
		case EventDone:
			doneAt = time.Now()
		}
	})

	if len(provider.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
	}
	order := []EventType{
		EventTextClear,
		EventToolStart,
		EventCanvasOpen,
		EventToolResult,
		EventAssistantCheckpoint,
		EventDone,
	}
	last := -1
	for _, typ := range order {
		index := eventIndex(events, typ, "")
		if index < 0 {
			t.Fatalf("missing event type %d in %+v", typ, events)
		}
		if index <= last {
			t.Fatalf("event type %d at %d, want after %d", typ, index, last)
		}
		last = index
	}
	if canvasFlushedAt.IsZero() || doneAt.IsZero() {
		t.Fatal("missing canvas/done timestamps")
	}
	if latency := doneAt.Sub(canvasFlushedAt); latency >= 250*time.Millisecond {
		t.Fatalf("render-to-done latency = %s, target <250ms", latency)
	}
	for _, event := range events[eventIndex(events, EventCanvasOpen, "")+1:] {
		if event.Type == EventText || event.Type == EventToolStart {
			t.Fatalf("unexpected post-render work: %+v", event)
		}
	}
}

func TestTerminalInteractiveRenderDefaultStillContinues(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
		llm.MockTextResponse("Placement is ready."),
	}}
	ag := New(provider, mcpclient.NewMockRouter(nil), WithInternalTools(map[string]InternalTool{
		"render_placement": terminalTool("render_placement", terminalCanvas, nil),
	}))

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
	if len(provider.Calls) != 2 {
		t.Fatalf("provider calls = %d, want legacy 2", len(provider.Calls))
	}
	if eventIndex(events, EventAssistantCheckpoint, "") >= 0 {
		t.Fatal("default behavior must not emit a terminal checkpoint")
	}
	var text strings.Builder
	for _, event := range events {
		if event.Type == EventText {
			text.WriteString(event.Text)
		}
	}
	if !strings.Contains(text.String(), "Placement is ready.") {
		t.Fatalf("missing follow-on provider text: %q", text.String())
	}
}

func TestTerminalInteractiveRenderEmitsCanvasWithoutEmitResult(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
	}}
	tool := terminalTool("render_placement", terminalCanvas, nil)
	tool.EmitResult = false
	ag := New(
		provider,
		mcpclient.NewMockRouter(nil),
		WithInternalTools(map[string]InternalTool{"render_placement": tool}),
		WithTerminalTools("render_placement"),
	)

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
	canvas := eventIndex(events, EventCanvasOpen, "")
	result := eventIndex(events, EventToolResult, "render_placement")
	if canvas < 0 || result <= canvas {
		t.Fatalf("canvas/result order = %d/%d, events: %+v", canvas, result, events)
	}
	for _, event := range events {
		if event.Type == EventText && strings.Contains(event.Text, "```canvas-") {
			t.Fatalf("terminal canvas must not leak as text: %+v", events)
		}
	}
}

func TestTerminalInteractiveRenderContinuesOnInvalidCanvasAndClose(t *testing.T) {
	tests := map[string]string{
		"invalid": "```canvas-dashboard\n{}\n```",
		"close":   "```canvas-dashboard\n" + `{"type":"dashboard","title":"Placement","close":true}` + "\n```",
	}
	for name, result := range tests {
		t.Run(name, func(t *testing.T) {
			provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
				llm.MockToolCallResponse("render-1", "render_placement", `{}`),
				llm.MockTextResponse("I could not open that view."),
			}}
			ag := New(
				provider,
				mcpclient.NewMockRouter(nil),
				WithInternalTools(map[string]InternalTool{
					"render_placement": terminalTool("render_placement", result, nil),
				}),
				WithTerminalTools("render_placement"),
			)
			events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
			if len(provider.Calls) != 2 {
				t.Fatalf("provider calls = %d, want 2", len(provider.Calls))
			}
			if eventIndex(events, EventAssistantCheckpoint, "") >= 0 {
				t.Fatal("invalid/close canvas must not terminate")
			}
		})
	}
}

func TestTerminalInteractiveRenderMixedBatchSemantics(t *testing.T) {
	t.Run("successful sibling settles before done", func(t *testing.T) {
		provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{{
			{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "render-1", Name: "render_placement", Input: json.RawMessage(`{}`)}},
			{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "audit-1", Name: "record_audit", Input: json.RawMessage(`{}`)}},
			{Type: llm.EventDone},
		}}}
		ag := New(
			provider,
			mcpclient.NewMockRouter(nil),
			WithInternalTools(map[string]InternalTool{
				"render_placement": terminalTool("render_placement", terminalCanvas, nil),
				"record_audit":     terminalTool("record_audit", "recorded", nil),
			}),
			WithTerminalTools("render_placement"),
		)
		events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
		if len(provider.Calls) != 1 {
			t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
		}
		siblingResult := eventIndex(events, EventToolResult, "record_audit")
		checkpoint := eventIndex(events, EventAssistantCheckpoint, "")
		if siblingResult < 0 || checkpoint <= siblingResult {
			t.Fatalf("checkpoint index %d must follow sibling result %d", checkpoint, siblingResult)
		}
	})

	t.Run("any sibling error continues", func(t *testing.T) {
		provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
			{
				{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "render-1", Name: "render_placement", Input: json.RawMessage(`{}`)}},
				{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "audit-1", Name: "record_audit", Input: json.RawMessage(`{}`)}},
				{Type: llm.EventDone},
			},
			llm.MockTextResponse("The audit step failed."),
		}}
		ag := New(
			provider,
			mcpclient.NewMockRouter(nil),
			WithInternalTools(map[string]InternalTool{
				"render_placement": terminalTool("render_placement", terminalCanvas, nil),
				"record_audit":     terminalTool("record_audit", "", errors.New("audit unavailable")),
			}),
			WithTerminalTools("render_placement"),
		)
		events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
		if len(provider.Calls) != 2 {
			t.Fatalf("provider calls = %d, want 2", len(provider.Calls))
		}
		if eventIndex(events, EventAssistantCheckpoint, "") >= 0 {
			t.Fatal("failed sibling must prevent terminal completion")
		}
	})

	t.Run("denied sibling continues", func(t *testing.T) {
		provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
			{
				{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "render-1", Name: "render_placement", Input: json.RawMessage(`{}`)}},
				{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "audit-1", Name: "record_audit", Input: json.RawMessage(`{}`)}},
				{Type: llm.EventDone},
			},
			llm.MockTextResponse("The audit step was denied."),
		}}
		router := mcpclient.NewMockRouter([]llm.ToolDef{{
			Name:   "record_audit",
			Schema: json.RawMessage(`{"type":"object"}`),
		}})
		ag := New(
			provider,
			router,
			WithInternalTools(map[string]InternalTool{
				"render_placement": terminalTool("render_placement", terminalCanvas, nil),
			}),
			WithTerminalTools("render_placement"),
			WithCapabilityFilter(capability.CapabilityMap{"audit": capability.StateAsk}, "read-write"),
			WithToolServerMap(map[string]string{"record_audit": "audit"}),
		)
		ag.ApprovalFunc = func(string, string, llm.ToolCall) bool { return false }

		events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}})
		if len(provider.Calls) != 2 {
			t.Fatalf("provider calls = %d, want 2", len(provider.Calls))
		}
		if eventIndex(events, EventAssistantCheckpoint, "") >= 0 {
			t.Fatal("denied sibling must prevent terminal completion")
		}
	})
}

func TestTerminalInteractiveRenderCancellationDoesNotComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
		llm.MockTextResponse("cancelled"),
	}}
	tool := terminalTool("render_placement", terminalCanvas, nil)
	tool.Handler = func(context.Context, json.RawMessage) (string, error) {
		cancel()
		return terminalCanvas, nil
	}
	ag := New(
		provider,
		mcpclient.NewMockRouter(nil),
		WithInternalTools(map[string]InternalTool{"render_placement": tool}),
		WithTerminalTools("render_placement"),
	)
	events := collectEventsWithContext(ag, ctx)
	if eventIndex(events, EventAssistantCheckpoint, "") >= 0 {
		t.Fatal("cancelled context must not terminally complete")
	}
}

func collectEventsWithContext(ag *Agent, ctx context.Context) []Event {
	var events []Event
	ag.Run(ctx, []llm.Message{{Role: llm.RoleUser, Content: "show placement"}}, func(event Event) {
		events = append(events, event)
	})
	return events
}
