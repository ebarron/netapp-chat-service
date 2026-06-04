package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
	"github.com/ebarron/netapp-chat-service/session"
)

// newTestServer wires a Server with a mock router pre-loaded with the given
// tools, all assigned to the "harvest" capability.
func newTestServer(t *testing.T, tools []llm.ToolDef) *Server {
	t.Helper()
	router := mcpclient.NewMockRouter(tools)
	router.SetServers([]string{"harvest-mcp"})
	for _, tool := range tools {
		router.SetToolServer(tool.Name, "harvest-mcp")
	}
	caps := []capability.Capability{
		{ID: "harvest", Name: "Harvest", State: capability.StateAllow, ServerName: "harvest-mcp"},
	}
	return New(&ChatDeps{
		Router:       router,
		Capabilities: caps,
	})
}

func TestGetCapabilitiesIncludesBudget(t *testing.T) {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("get_a", "ro"),
		mcpclient.MockReadOnlyTool("get_b", "ro"),
		mcpclient.MockTool("write_c", "rw"),
	}
	srv := newTestServer(t, tools)

	req := httptest.NewRequest(http.MethodGet, "/chat/capabilities", nil)
	w := httptest.NewRecorder()
	srv.GetChatCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Capabilities []capability.Capability `json:"capabilities"`
		ToolBudget   struct {
			Used int    `json:"used"`
			Max  int    `json:"max"`
			Mode string `json:"mode"`
		} `json:"tool_budget"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ToolBudget.Mode != "read-only" {
		t.Errorf("mode = %q, want read-only", resp.ToolBudget.Mode)
	}
	if resp.ToolBudget.Used != 2 {
		t.Errorf("used = %d, want 2 (read-only tools only)", resp.ToolBudget.Used)
	}
	if resp.ToolBudget.Max != agent.MaxToolsPerRequest {
		t.Errorf("max = %d, want %d", resp.ToolBudget.Max, agent.MaxToolsPerRequest)
	}

	// Per-cap counts.
	if len(resp.Capabilities) != 1 {
		t.Fatalf("got %d caps, want 1", len(resp.Capabilities))
	}
	if resp.Capabilities[0].ToolsCount != 3 {
		t.Errorf("tools_count = %d, want 3", resp.Capabilities[0].ToolsCount)
	}
	if resp.Capabilities[0].ReadOnlyToolsCount != 2 {
		t.Errorf("read_only_tools_count = %d, want 2", resp.Capabilities[0].ReadOnlyToolsCount)
	}
}

func TestGetCapabilitiesReadWriteBudget(t *testing.T) {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("get_a", "ro"),
		mcpclient.MockTool("write_c", "rw"),
	}
	srv := newTestServer(t, tools)

	req := httptest.NewRequest(http.MethodGet, "/chat/capabilities?mode=read-write", nil)
	w := httptest.NewRecorder()
	srv.GetChatCapabilities(w, req)

	var resp struct {
		ToolBudget struct {
			Used int    `json:"used"`
			Mode string `json:"mode"`
		} `json:"tool_budget"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ToolBudget.Mode != "read-write" {
		t.Errorf("mode = %q, want read-write", resp.ToolBudget.Mode)
	}
	if resp.ToolBudget.Used != 2 {
		t.Errorf("used = %d, want 2 (both tools count in read-write)", resp.ToolBudget.Used)
	}
}

func TestPostCapabilitiesRejectsOverBudget(t *testing.T) {
	// Build > 128 read-only tools so enabling the cap would exceed the budget.
	n := agent.MaxToolsPerRequest + 5
	tools := make([]llm.ToolDef, n)
	for i := range tools {
		tools[i] = mcpclient.MockReadOnlyTool(fmt.Sprintf("t%d", i), "ro")
	}
	srv := newTestServer(t, tools)
	// Start in StateOff so the toggle to allow triggers the budget check.
	srv.deps.Capabilities[0].State = capability.StateOff

	body, _ := json.Marshal(map[string]any{
		"capabilities": map[string]string{"harvest": "allow"},
		"mode":         "read-only",
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/capabilities", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatCapabilities(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	// State must NOT have been mutated.
	if srv.deps.Capabilities[0].State != capability.StateOff {
		t.Errorf("state mutated despite budget rejection: %s", srv.deps.Capabilities[0].State)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["tool_budget"]; !ok {
		t.Error("response should include tool_budget for diagnostics")
	}
}

func TestPostCapabilitiesAcceptsWithinBudget(t *testing.T) {
	tools := []llm.ToolDef{mcpclient.MockReadOnlyTool("t1", "ro")}
	srv := newTestServer(t, tools)
	srv.deps.Capabilities[0].State = capability.StateOff

	body, _ := json.Marshal(map[string]any{
		"capabilities": map[string]string{"harvest": "allow"},
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/capabilities", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if srv.deps.Capabilities[0].State != capability.StateAllow {
		t.Errorf("state = %s, want allow", srv.deps.Capabilities[0].State)
	}
}

// twoServerDeps builds ChatDeps with a jira and bitbucket capability, each
// with one read-only tool, and a mock provider replaying the given responses.
func twoServerDeps(provider llm.Provider, mode string) (*ChatDeps, *mcpclient.MockRouter) {
	router := mcpclient.NewMockRouter([]llm.ToolDef{
		mcpclient.MockReadOnlyTool("create_issue", "Create a Jira issue"),
		mcpclient.MockReadOnlyTool("search_repo", "Search Bitbucket"),
	})
	router.SetServers([]string{"jira-mcp", "bitbucket-mcp"})
	router.SetToolServer("create_issue", "jira-mcp")
	router.SetToolServer("search_repo", "bitbucket-mcp")
	router.SetResult("create_issue", "ok")
	deps := &ChatDeps{
		Sessions: session.NewManager(10),
		Provider: provider,
		Router:   router,
		Logger:   slogDiscard(),
		Capabilities: []capability.Capability{
			{ID: "jira", Name: "Jira", State: capability.StateAllow, ServerName: "jira-mcp"},
			{ID: "bitbucket", Name: "Bitbucket", State: capability.StateAllow, ServerName: "bitbucket-mcp"},
		},
		ToolRoutingMode:      mode,
		ToolRoutingForceLoad: mode == agent.ToolRoutingInBand,
	}
	return deps, router
}

// TestRunChatInBandWiring verifies that with mode in-band the system prompt
// carries the group menu and the offered tools include the internal
// load_tools tool, while the MCP tools are withheld until a group is loaded.
func TestRunChatInBandWiring(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "load_tools", map[string]any{"groups": []string{"jira"}}),
			llm.MockTextResponse("done"),
		},
	}
	deps, _ := twoServerDeps(provider, agent.ToolRoutingInBand)

	req := ChatMessageRequest{Message: "create a jira issue", Mode: "read-only"}
	RunChat(context.Background(), deps, req, func(string, any) {}, nil)

	if len(provider.Calls) == 0 {
		t.Fatal("provider was never called")
	}
	sys := provider.Calls[0].System
	for _, want := range []string{"Tool Groups", "load_tools", "| jira |", "| bitbucket |"} {
		if !contains(sys, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}

	// First turn: only the internal load_tools tool is offered (MCP tools
	// withheld until a group loads).
	first := provider.Calls[0].Tools
	if !hasTool(first, "load_tools") {
		t.Error("first turn should offer load_tools")
	}
	if hasTool(first, "create_issue") || hasTool(first, "search_repo") {
		t.Error("MCP tools must be withheld before any group is loaded")
	}
	// Second turn (after load_tools([jira])): jira's tool becomes available,
	// bitbucket's does not.
	if len(provider.Calls) < 2 {
		t.Fatalf("expected at least 2 LLM calls, got %d", len(provider.Calls))
	}
	second := provider.Calls[1].Tools
	if !hasTool(second, "create_issue") {
		t.Error("create_issue should be offered after loading jira")
	}
	if hasTool(second, "search_repo") {
		t.Error("search_repo should remain withheld (bitbucket not loaded)")
	}
}

// TestRunChatModeOffWiring verifies that with routing off the system prompt
// has no group menu, no load_tools tool is offered, and all enabled tools are
// available immediately (today's behavior).
func TestRunChatModeOffWiring(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses:    [][]llm.StreamEvent{llm.MockTextResponse("hi")},
	}
	deps, _ := twoServerDeps(provider, agent.ToolRoutingOff)

	req := ChatMessageRequest{Message: "hello", Mode: "read-only"}
	RunChat(context.Background(), deps, req, func(string, any) {}, nil)

	if len(provider.Calls) == 0 {
		t.Fatal("provider was never called")
	}
	if contains(provider.Calls[0].System, "Tool Groups") {
		t.Error("mode off must not emit a Tool Groups section")
	}
	tools := provider.Calls[0].Tools
	if hasTool(tools, "load_tools") {
		t.Error("mode off must not offer load_tools")
	}
	if !hasTool(tools, "create_issue") || !hasTool(tools, "search_repo") {
		t.Error("mode off should offer all enabled tools immediately")
	}
}

func hasTool(tools []llm.ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGetCapabilitiesIncludesDualBudgets(t *testing.T) {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("get_a", "ro"),
		mcpclient.MockReadOnlyTool("get_b", "ro"),
		mcpclient.MockTool("write_c", "rw"),
	}
	srv := newTestServer(t, tools)

	req := httptest.NewRequest(http.MethodGet, "/chat/capabilities", nil)
	w := httptest.NewRecorder()
	srv.GetChatCapabilities(w, req)

	var resp struct {
		ToolBudgets struct {
			ReadOnly  struct{ Used, Max int } `json:"read_only"`
			ReadWrite struct{ Used, Max int } `json:"read_write"`
		} `json:"tool_budgets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ToolBudgets.ReadOnly.Used != 2 {
		t.Errorf("read_only.used = %d, want 2", resp.ToolBudgets.ReadOnly.Used)
	}
	if resp.ToolBudgets.ReadWrite.Used != 3 {
		t.Errorf("read_write.used = %d, want 3", resp.ToolBudgets.ReadWrite.Used)
	}
	if resp.ToolBudgets.ReadOnly.Max != agent.MaxToolsPerRequest ||
		resp.ToolBudgets.ReadWrite.Max != agent.MaxToolsPerRequest {
		t.Error("max should be set in both budgets")
	}
}
