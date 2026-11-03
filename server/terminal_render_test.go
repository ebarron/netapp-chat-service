package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

func TestRunChatWithOptionsCommitsTerminalCheckpointBeforeDoneAndResumes(t *testing.T) {
	canvas := "```canvas-dashboard\n" +
		`{"type":"dashboard","title":"Placement","panels":[{"type":"action-button","buttons":[{"label":"Continue","action":"message","message":"continue"}]}]}` +
		"\n```"
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
		llm.MockTextResponse("The selected placement remains available."),
	}}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExtraTools: map[string]agent.InternalTool{
			"render_placement": {
				Def: llm.ToolDef{
					Name:   "render_placement",
					Schema: json.RawMessage(`{"type":"object"}`),
				},
				Handler: func(context.Context, json.RawMessage) (string, error) {
					return canvas, nil
				},
				EmitResult: true,
			},
		},
	}
	terminal := RunChatOptions{
		TerminalToolPolicy: func(toolName string) bool {
			return toolName == "render_placement"
		},
	}

	var firstEvents []string
	var terminalCanvasEvent *agent.CanvasPayload
	RunChatWithOptions(
		context.Background(),
		deps,
		ChatMessageRequest{Message: "show placement", SessionID: "terminal-session"},
		func(event string, data any) {
			if event == "canvas_open" {
				terminalCanvasEvent, _ = data.(*agent.CanvasPayload)
			}
			if event == "done" {
				msgs := deps.Sessions.Get("terminal-session").Messages
				if len(msgs) != 2 || msgs[1].Role != llm.RoleAssistant {
					t.Fatalf("history was not checkpointed before done: %+v", msgs)
				}
			}
			firstEvents = append(firstEvents, event)
		},
		nil,
		terminal,
	)

	if len(provider.Calls) != 1 {
		t.Fatalf("first turn provider calls = %d, want 1", len(provider.Calls))
	}
	wantEvents := []string{"message", "text_clear", "tool_call", "canvas_open", "tool_result", "done"}
	if len(firstEvents) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", firstEvents, wantEvents)
	}
	for i := range wantEvents {
		if firstEvents[i] != wantEvents[i] {
			t.Fatalf("events = %v, want %v", firstEvents, wantEvents)
		}
	}
	if terminalCanvasEvent == nil || !terminalCanvasEvent.TerminalInteractive {
		t.Fatalf("terminal canvas metadata = %+v, want terminal_interactive", terminalCanvasEvent)
	}
	msgs := deps.Sessions.Get("terminal-session").Messages
	if got := msgs[len(msgs)-1]; got.Role != llm.RoleAssistant ||
		got.Content != "Interactive view rendered successfully." {
		t.Fatalf("terminal checkpoint = %+v", got)
	}

	RunChat(
		context.Background(),
		deps,
		ChatMessageRequest{Message: "is it still available?", SessionID: "terminal-session"},
		func(string, any) {},
		nil,
	)
	if len(provider.Calls) != 2 {
		t.Fatalf("provider calls after resume = %d, want 2", len(provider.Calls))
	}
	resumeMessages := provider.Calls[1].Messages
	if len(resumeMessages) < 3 {
		t.Fatalf("resume request history too short: %+v", resumeMessages)
	}
	if got := resumeMessages[len(resumeMessages)-2]; got.Role != llm.RoleAssistant ||
		got.Content != "Interactive view rendered successfully." {
		t.Fatalf("resume checkpoint = %+v", got)
	}
	if got := resumeMessages[len(resumeMessages)-1]; got.Role != llm.RoleUser ||
		got.Content != "is it still available?" {
		t.Fatalf("resume user message = %+v", got)
	}
}

func TestRunChatInlineTerminalRendersOnceAndRehydratesHistoryWithoutDuplicate(t *testing.T) {
	inline := "```dashboard\n" +
		`{"type":"dashboard","title":"Fleet Health","panels":[{"type":"callout","title":"Healthy","body":"All systems operational."}]}` +
		"\n```"
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_fleet_health", `{}`),
		llm.MockTextResponse("The prior fleet view remains available."),
	}}
	handler := func(context.Context, json.RawMessage) (string, error) {
		return inline, nil
	}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExtraTools: map[string]agent.InternalTool{
			"render_fleet_health": {
				Def: llm.ToolDef{
					Name:   "render_fleet_health",
					Schema: json.RawMessage(`{"type":"object"}`),
				},
				Handler:    agent.WithInlineRenderCompletion(handler),
				EmitResult: true,
			},
		},
	}

	var firstVisible strings.Builder
	var canvasEvents int
	RunChat(
		context.Background(),
		deps,
		ChatMessageRequest{
			Message: "what is the health of my fleet?", SessionID: "inline-terminal",
		},
		func(event string, data any) {
			switch event {
			case "message":
				payload := data.(map[string]string)
				firstVisible.WriteString(payload["content"])
			case "canvas_open":
				canvasEvents++
			}
		},
		nil,
	)

	if len(provider.Calls) != 1 {
		t.Fatalf("first turn provider calls = %d, want 1", len(provider.Calls))
	}
	if canvasEvents != 0 {
		t.Fatalf("inline result emitted %d Canvas handoff event(s)", canvasEvents)
	}
	if got := strings.Count(firstVisible.String(), "```dashboard"); got != 1 {
		t.Fatalf("live inline dashboard count = %d, want 1: %q", got, firstVisible.String())
	}
	history := deps.Sessions.Get("inline-terminal").Messages
	if len(history) != 2 ||
		strings.Count(history[1].Content, "```dashboard") != 1 {
		t.Fatalf("terminal history = %+v, want one persisted inline dashboard", history)
	}

	RunChat(
		context.Background(),
		deps,
		ChatMessageRequest{
			Message: "is that still current?", SessionID: "inline-terminal",
		},
		func(string, any) {},
		nil,
	)
	if len(provider.Calls) != 2 {
		t.Fatalf("provider calls after resume = %d, want 2", len(provider.Calls))
	}
	resumed := provider.Calls[1].Messages
	if len(resumed) < 3 ||
		strings.Count(resumed[len(resumed)-2].Content, "```dashboard") != 1 {
		t.Fatalf("rehydrated history duplicated inline dashboard: %+v", resumed)
	}
}

func TestNewWithOptionsConfiguresTerminalPolicyWithoutChangingNew(t *testing.T) {
	deps := &ChatDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if got := New(deps).runChatOptions.TerminalToolPolicy; got != nil {
		t.Fatal("New must preserve default continue-after-tool behavior")
	}

	srv := NewWithOptions(deps, WithTerminalToolPolicy(func(name string) bool {
		return name == "render_placement"
	}))
	if srv.runChatOptions.TerminalToolPolicy == nil ||
		!srv.runChatOptions.TerminalToolPolicy("render_placement") ||
		srv.runChatOptions.TerminalToolPolicy("other") {
		t.Fatal("NewWithOptions did not retain terminal tool policy")
	}
}

func TestNewWithOptionsAppliesTerminalPolicyThroughHTTP(t *testing.T) {
	canvas := "```canvas-dashboard\n" +
		`{"type":"dashboard","title":"Placement","panels":[{"type":"action-button","buttons":[{"label":"Continue","action":"message","message":"continue"}]}]}` +
		"\n```"
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
		llm.MockTextResponse("must not be called"),
	}}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExtraTools: map[string]agent.InternalTool{
			"render_placement": {
				Def: llm.ToolDef{Name: "render_placement", Schema: json.RawMessage(`{"type":"object"}`)},
				Handler: func(context.Context, json.RawMessage) (string, error) {
					return canvas, nil
				},
				EmitResult: true,
			},
		},
	}
	srv := NewWithOptions(deps, WithTerminalToolPolicy(func(name string) bool {
		return name == "render_placement"
	}))
	req := httptest.NewRequest(http.MethodPost, "/chat/message",
		bytes.NewBufferString(`{"message":"show placement"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.PostChatMessage(rec, req)

	if len(provider.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
	}
	if deps.Sessions.Count() != 1 {
		t.Fatalf("sessions = %d, want exactly one resolution for empty ID", deps.Sessions.Count())
	}
	body := rec.Body.String()
	for _, event := range []string{"event: canvas_open", "event: tool_result", "event: done"} {
		if !strings.Contains(body, event) {
			t.Fatalf("HTTP stream missing %q:\n%s", event, body)
		}
	}
	if !strings.Contains(body, `"terminal_interactive":true`) {
		t.Fatalf("HTTP stream missing terminal handoff metadata:\n%s", body)
	}
}

func TestTerminalCheckpointPreservesVisibleSiblingHistory(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{{
		{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "render-1", Name: "render_placement", Input: json.RawMessage(`{}`)}},
		{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "note-1", Name: "visible_note", Input: json.RawMessage(`{}`)}},
		{Type: llm.EventDone},
	}}}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExtraTools: map[string]agent.InternalTool{
			"render_placement": {
				Def:        llm.ToolDef{Name: "render_placement", Schema: json.RawMessage(`{"type":"object"}`)},
				Handler:    func(context.Context, json.RawMessage) (string, error) { return terminalServerCanvas(), nil },
				EmitResult: true,
			},
			"visible_note": {
				Def:        llm.ToolDef{Name: "visible_note", Schema: json.RawMessage(`{"type":"object"}`)},
				Handler:    func(context.Context, json.RawMessage) (string, error) { return "Visible sibling result.", nil },
				EmitResult: true,
			},
		},
	}
	RunChatWithOptions(
		context.Background(),
		deps,
		ChatMessageRequest{Message: "show placement", SessionID: "visible-sibling"},
		func(string, any) {},
		nil,
		RunChatOptions{TerminalToolPolicy: func(name string) bool { return name == "render_placement" }},
	)

	msgs := deps.Sessions.Get("visible-sibling").Messages
	got := msgs[len(msgs)-1]
	if got.Role != llm.RoleAssistant || got.Content != "Visible sibling result.\n\n" {
		t.Fatalf("assistant history = %+v, want visible sibling output", got)
	}
}

func TestCancelledTurnCommitsHiddenHistoryPairWithoutDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("render-1", "render_placement", `{}`),
	}}
	deps := &ChatDeps{
		Sessions: session.NewManager(20),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		ExtraTools: map[string]agent.InternalTool{
			"render_placement": {
				Def: llm.ToolDef{Name: "render_placement", Schema: json.RawMessage(`{"type":"object"}`)},
				Handler: func(context.Context, json.RawMessage) (string, error) {
					cancel()
					return terminalServerCanvas(), nil
				},
				EmitResult: true,
			},
		},
	}
	var events []string
	RunChatWithOptions(
		ctx,
		deps,
		ChatMessageRequest{Message: "show placement", SessionID: "cancelled-turn"},
		func(event string, _ any) { events = append(events, event) },
		nil,
		RunChatOptions{TerminalToolPolicy: func(string) bool { return true }},
	)

	if len(provider.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
	}
	for _, event := range events {
		if event == "done" || event == "canvas_open" {
			t.Fatalf("cancelled turn emitted %q: %v", event, events)
		}
	}
	msgs := deps.Sessions.Get("cancelled-turn").Messages
	if len(msgs) != 2 || msgs[1].Role != llm.RoleAssistant ||
		msgs[1].Content != "The previous turn was cancelled." {
		t.Fatalf("cancelled history = %+v", msgs)
	}
}

func terminalServerCanvas() string {
	return "```canvas-dashboard\n" +
		`{"type":"dashboard","title":"Placement","panels":[{"type":"action-button","buttons":[{"label":"Continue","action":"message","message":"continue"}]}]}` +
		"\n```"
}
