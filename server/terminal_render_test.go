package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

func TestRunChatWithOptionsCommitsTerminalCheckpointBeforeDoneAndResumes(t *testing.T) {
	canvas := "```canvas-dashboard\n" +
		`{"type":"dashboard","title":"Placement","panels":[{"type":"callout","title":"Ready","body":"Choose an action."}]}` +
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
	RunChatWithOptions(
		context.Background(),
		deps,
		ChatMessageRequest{Message: "show placement", SessionID: "terminal-session"},
		func(event string, _ any) {
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
