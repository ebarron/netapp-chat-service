package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/config"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/server"
	"github.com/ebarron/netapp-chat-service/session"
)

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestApplyOpenNavTool_Disabled verifies that with navigation-by-prompt off
// (the zero value / default), the binary registers no open_nav_view tool —
// binary consumers that don't opt in are byte-for-byte unaffected.
func TestApplyOpenNavTool_Disabled(t *testing.T) {
	deps := &server.ChatDeps{}
	if registered := applyOpenNavTool(config.OpenNavConfig{}, deps); registered {
		t.Error("applyOpenNavTool reported a registration when disabled")
	}
	if _, ok := deps.ExtraTools[agent.OpenNavToolName]; ok {
		t.Error("open_nav_view must not be registered when open_nav.enabled is false")
	}
}

// TestApplyOpenNavTool_Enabled verifies the config-driven registration wires
// agent.NewOpenNavTool() into ExtraTools and honors required_after_interest.
func TestApplyOpenNavTool_Enabled(t *testing.T) {
	deps := &server.ChatDeps{}
	registered := applyOpenNavTool(config.OpenNavConfig{
		Enabled:               true,
		RequiredAfterInterest: "navigation",
	}, deps)
	if !registered {
		t.Fatal("applyOpenNavTool should report a registration when enabled")
	}
	tool, ok := deps.ExtraTools[agent.OpenNavToolName]
	if !ok {
		t.Fatal("open_nav_view must be registered when open_nav.enabled is true")
	}
	if tool.RequiredAfterInterest != "navigation" {
		t.Errorf("RequiredAfterInterest = %q, want %q", tool.RequiredAfterInterest, "navigation")
	}
	if tool.Emit == nil {
		t.Error("registered open_nav_view tool must carry the Emit side-channel hook")
	}
}

// TestBinaryOpenNav_EndToEnd proves the config-registered tool relays an
// "open_nav" SSE event end-to-end through server.RunChat, mirroring
// server/opennav_test.go but exercising the binary's registration path.
func TestBinaryOpenNav_EndToEnd(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-nav", agent.OpenNavToolName, map[string]any{"destination": "settings/general"}),
			llm.MockTextResponse("Done."),
		},
	}
	deps := &server.ChatDeps{
		Sessions: session.NewManager(10),
		Provider: provider,
		Router:   mcpclient.NewMockRouter(nil),
		Logger:   slogDiscard(),
	}
	if !applyOpenNavTool(config.OpenNavConfig{Enabled: true}, deps) {
		t.Fatal("expected tool to be registered")
	}

	var navCount int
	var dest string
	server.RunChat(context.Background(), deps,
		server.ChatMessageRequest{Message: "go to general settings"},
		func(event string, data any) {
			if event != "open_nav" {
				return
			}
			navCount++
			b, err := json.Marshal(data)
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
		}, nil)

	if navCount != 1 {
		t.Fatalf("expected exactly 1 open_nav event, got %d", navCount)
	}
	if dest != "settings/general" {
		t.Errorf("open_nav destination = %q, want %q", dest, "settings/general")
	}
}

// TestOpenNavConfigParse verifies the open_nav config block round-trips through
// config parsing (the operator-facing knob).
func TestOpenNavConfigParse(t *testing.T) {
	cfg := parseTestConfig(t, `
llm:
  provider: anthropic
  model: test
open_nav:
  enabled: true
  required_after_interest: navigation
`)
	if !cfg.OpenNav.Enabled {
		t.Error("open_nav.enabled should parse to true")
	}
	if cfg.OpenNav.RequiredAfterInterest != "navigation" {
		t.Errorf("required_after_interest = %q, want navigation", cfg.OpenNav.RequiredAfterInterest)
	}

	// Absent block ⇒ disabled (default, behavior unchanged).
	def := parseTestConfig(t, "llm:\n  provider: anthropic\n  model: test\n")
	if def.OpenNav.Enabled {
		t.Error("open_nav.enabled should default to false when the block is omitted")
	}
}

func parseTestConfig(t *testing.T, yaml string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}
