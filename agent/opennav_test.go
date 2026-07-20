package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// TestNewOpenNavTool_Definition verifies the generic open_nav_view tool
// definition: correct name and a required "destination" string argument.
func TestNewOpenNavTool_Definition(t *testing.T) {
	tool := NewOpenNavTool()
	if tool.Def.Name != OpenNavToolName {
		t.Errorf("Def.Name = %q, want %q", tool.Def.Name, OpenNavToolName)
	}
	if tool.Handler == nil {
		t.Error("Handler must not be nil")
	}
	if tool.Emit == nil {
		t.Error("Emit must not be nil")
	}

	var schema struct {
		Type       string `json:"type"`
		Properties struct {
			Destination struct {
				Type string `json:"type"`
			} `json:"destination"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.Def.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	if schema.Properties.Destination.Type != "string" {
		t.Errorf("destination arg type = %q, want string", schema.Properties.Destination.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "destination" {
		t.Errorf("required = %v, want [destination]", schema.Required)
	}
}

// TestOpenNavTool_InvocationEmitsEvent verifies that when the LLM calls
// open_nav_view, the agent invokes the host tool and emits an EventOpenNav
// carrying the destination argument (alongside the normal tool events).
// Destinations are generic — the engine never hardcodes NABox routes.
func TestOpenNavTool_InvocationEmitsEvent(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-nav", OpenNavToolName, map[string]any{"destination": "settings/general"}),
			llm.MockTextResponse("Opened it."),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(map[string]InternalTool{
		OpenNavToolName: NewOpenNavTool(),
	}))

	events := collectEvents(t, ag, []llm.Message{
		{Role: llm.RoleUser, Content: "go to general settings"},
	})

	var navEvents []Event
	var toolResults []Event
	for _, e := range events {
		switch e.Type {
		case EventOpenNav:
			navEvents = append(navEvents, e)
		case EventToolResult:
			toolResults = append(toolResults, e)
		}
	}

	if len(navEvents) != 1 {
		t.Fatalf("expected 1 open-nav event, got %d: %+v", len(navEvents), events)
	}
	if navEvents[0].OpenNav == nil {
		t.Fatal("OpenNav payload is nil")
	}
	if navEvents[0].OpenNav.Destination != "settings/general" {
		t.Errorf("Destination = %q, want %q", navEvents[0].OpenNav.Destination, "settings/general")
	}
	if len(toolResults) != 1 {
		t.Errorf("expected 1 tool_result, got %d", len(toolResults))
	}
}

// TestOpenNavTool_MissingDestination verifies the handler rejects an empty
// destination and no open-nav event is emitted.
func TestOpenNavTool_MissingDestination(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-nav", OpenNavToolName, map[string]any{"destination": "  "}),
			llm.MockTextResponse("sorry"),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(map[string]InternalTool{
		OpenNavToolName: NewOpenNavTool(),
	}))

	events := collectEvents(t, ag, []llm.Message{
		{Role: llm.RoleUser, Content: "navigate somewhere"},
	})

	for _, e := range events {
		if e.Type == EventOpenNav {
			t.Errorf("no open-nav event expected on invalid destination, got %+v", e)
		}
	}
	var hasErr bool
	for _, e := range events {
		if e.Type == EventToolError {
			hasErr = true
		}
	}
	if !hasErr {
		t.Error("expected a tool error for empty destination")
	}
}

// TestInternalTool_NoEmit_BackwardCompat verifies that an internal tool
// without an Emit hook (today's shape) produces no side-channel events — the
// backward-compat guarantee for existing render/ExtraTools.
func TestInternalTool_NoEmit_BackwardCompat(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-x", "plain_tool", map[string]any{}),
			llm.MockTextResponse("done"),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(map[string]InternalTool{
		"plain_tool": {
			Def: llm.ToolDef{
				Name:   "plain_tool",
				Schema: json.RawMessage(`{"type":"object","properties":{}}`),
			},
			Handler: func(_ context.Context, _ json.RawMessage) (string, error) { return "ok", nil },
		},
	}))
	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	for _, e := range events {
		if e.Type == EventOpenNav {
			t.Errorf("no open-nav event expected without Emit hook, got %+v", e)
		}
	}
}

// TestBuildSystemPrompt_CanvasDigest verifies the optional C5 `digest` field:
// present ⇒ a free-text detail block is appended; absent ⇒ output is
// byte-for-byte identical to a summary without digests.
func TestBuildSystemPrompt_CanvasDigest(t *testing.T) {
	router := mcpclient.NewMockRouter(nil)
	cfg := SystemPromptConfig{ProductName: "Test Assistant"}

	base := []CanvasTabSummary{
		{TabID: "nav", Kind: "nav-view", Name: "Alerting", Qualifier: "/alerting", Status: "ok"},
	}
	withDigest := []CanvasTabSummary{
		{TabID: "nav", Kind: "nav-view", Name: "Alerting", Qualifier: "/alerting", Status: "ok",
			Digest: "3 rules enabled, 1 disabled; latency rule firing on cluster cls1."},
	}

	promptNoDigest := BuildSystemPrompt(cfg, router, "", base...)
	promptDigest := BuildSystemPrompt(cfg, router, "", withDigest...)

	if strings.Contains(promptNoDigest, "Additional detail") {
		t.Error("no-digest prompt should not contain the digest detail block")
	}
	if !strings.Contains(promptDigest, "Additional detail") {
		t.Error("digest prompt should contain the digest detail block")
	}
	if !strings.Contains(promptDigest, "3 rules enabled") {
		t.Error("digest prompt should contain the digest text")
	}

	// Byte-for-byte compat: the digest block is strictly appended, so the
	// no-digest prompt (minus its trailing newline) must be a prefix of the
	// digest prompt.
	trimmed := strings.TrimRight(promptNoDigest, "\n")
	if !strings.HasPrefix(promptDigest, trimmed) {
		t.Error("digest prompt must be the no-digest prompt plus an appended block")
	}
}
