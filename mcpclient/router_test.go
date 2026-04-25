package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ebarron/netapp-chat-service/llm"
)

func TestMockRouterTools(t *testing.T) {
	tools := []llm.ToolDef{
		MockTool("get_volumes", "List volumes"),
		MockTool("metrics_query", "Query metrics"),
	}
	m := NewMockRouter(tools)

	got := m.Tools()
	if len(got) != 2 {
		t.Fatalf("got %d tools, want 2", len(got))
	}
	if got[0].Name != "get_volumes" {
		t.Errorf("tool[0].Name = %q, want %q", got[0].Name, "get_volumes")
	}
	if got[1].Name != "metrics_query" {
		t.Errorf("tool[1].Name = %q, want %q", got[1].Name, "metrics_query")
	}
}

func TestMockRouterCallTool(t *testing.T) {
	m := NewMockRouter([]llm.ToolDef{MockTool("greet", "Say hello")})
	m.SetResult("greet", "Hello, world!")

	input, _ := json.Marshal(map[string]string{"name": "world"})
	tc := llm.ToolCall{ID: "tc-1", Name: "greet", Input: input}

	result, err := m.CallTool(context.Background(), tc)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result != "Hello, world!" {
		t.Errorf("result = %q, want %q", result, "Hello, world!")
	}

	calls := m.Calls()
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Name != "greet" {
		t.Errorf("call.Name = %q, want %q", calls[0].Name, "greet")
	}
}

func TestMockRouterCallToolError(t *testing.T) {
	m := NewMockRouter([]llm.ToolDef{MockTool("fail", "Always fails")})
	m.SetError("fail", errors.New("something broke"))

	tc := llm.ToolCall{ID: "tc-2", Name: "fail", Input: json.RawMessage(`{}`)}
	_, err := m.CallTool(context.Background(), tc)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "something broke" {
		t.Errorf("error = %q, want %q", err.Error(), "something broke")
	}
}

func TestMockRouterCallToolUnconfigured(t *testing.T) {
	m := NewMockRouter(nil)
	tc := llm.ToolCall{ID: "tc-3", Name: "unknown", Input: json.RawMessage(`{}`)}
	_, err := m.CallTool(context.Background(), tc)
	if err == nil {
		t.Fatal("expected error for unconfigured tool")
	}
}

func TestMockRouterConnectedServers(t *testing.T) {
	m := NewMockRouter(nil)

	if got := m.ConnectedServers(); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}

	m.SetServers([]string{"harvest-mcp", "ontap-mcp"})
	got := m.ConnectedServers()
	want := []string{"harvest-mcp", "ontap-mcp"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ConnectedServers() = %v, want %v", got, want)
	}
}

func TestMockToolHelper(t *testing.T) {
	td := MockTool("test_tool", "A test tool")
	if td.Name != "test_tool" {
		t.Errorf("Name = %q, want %q", td.Name, "test_tool")
	}
	if td.Description != "A test tool" {
		t.Errorf("Description = %q, want %q", td.Description, "A test tool")
	}

	var schema map[string]any
	if err := json.Unmarshal(td.Schema, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, want %q", schema["type"], "object")
	}
}

func TestRouterNewEmpty(t *testing.T) {
	r := NewRouter(nil)
	tools := r.Tools()
	if len(tools) != 0 {
		t.Errorf("expected no tools, got %d", len(tools))
	}
	servers := r.ConnectedServers()
	if len(servers) != 0 {
		t.Errorf("expected no servers, got %d", len(servers))
	}
}

func TestRouterClose(t *testing.T) {
	r := NewRouter(nil)
	if err := r.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestRouterDisconnectNonexistent(t *testing.T) {
	r := NewRouter(nil)
	if err := r.Disconnect("nonexistent"); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
}

func TestRouterCallToolUnknown(t *testing.T) {
	r := NewRouter(nil)
	tc := llm.ToolCall{ID: "tc-1", Name: "nonexistent", Input: json.RawMessage(`{}`)}
	_, err := r.CallTool(context.Background(), tc)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRouterConnectAllUnreachable(t *testing.T) {
	// ConnectAll with unreachable servers should log errors but not panic.
	r := NewRouter(nil)
	servers := []ServerConfig{
		{Name: "bad-server", Endpoint: "http://127.0.0.1:19999"},
	}
	// Use 1 attempt with minimal delay to keep the test fast.
	r.ConnectAll(servers, 1, 0)

	// Server should not appear in connected list.
	if got := r.ConnectedServers(); len(got) != 0 {
		t.Errorf("expected 0 connected servers, got %v", got)
	}
	// No tools should be available.
	if got := r.Tools(); len(got) != 0 {
		t.Errorf("expected 0 tools, got %d", len(got))
	}
}

func TestRouterConnectAllEmpty(t *testing.T) {
	// ConnectAll with an empty list should be a no-op.
	r := NewRouter(nil)
	r.ConnectAll(nil, 10, 0)

	if got := r.ConnectedServers(); len(got) != 0 {
		t.Errorf("expected 0 connected servers, got %v", got)
	}
}

func TestConvertToolPropagatesAnnotations(t *testing.T) {
	r := NewRouter(nil)
	destructive := true
	tool := &mcp.Tool{
		Name:        "list_volumes",
		Description: "list",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &destructive,
		},
	}
	def := r.convertTool(tool, "harvest", nil)
	if !def.ReadOnlyHint {
		t.Error("expected ReadOnlyHint=true from annotations")
	}
	if !def.DestructiveHint {
		t.Error("expected DestructiveHint=true from annotations")
	}

	// No annotations + allowlist override.
	bare := &mcp.Tool{Name: "metric_query", Description: ""}
	def2 := r.convertTool(bare, "harvest", map[string]bool{"metric_query": true})
	if !def2.ReadOnlyHint {
		t.Error("expected allowlist to set ReadOnlyHint=true")
	}

	// No annotations and not on allowlist → defaults to write.
	def3 := r.convertTool(bare, "harvest", nil)
	if def3.ReadOnlyHint {
		t.Error("expected unannotated tool to default to ReadOnlyHint=false")
	}
}
