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

// routingBudgetServer builds a Server with two capabilities whose tool counts
// sum to more than MaxToolsPerRequest but are each individually under it.
// secondState seeds the ontap capability's state; routingOn toggles in-band
// tool routing.
func routingBudgetServer(perCap int, secondState capability.State, routingOn bool) *Server {
	tools := make([]llm.ToolDef, 0, perCap*2)
	for i := 0; i < perCap; i++ {
		tools = append(tools, mcpclient.MockReadOnlyTool(fmt.Sprintf("h%d", i), "ro"))
	}
	for i := 0; i < perCap; i++ {
		tools = append(tools, mcpclient.MockReadOnlyTool(fmt.Sprintf("o%d", i), "ro"))
	}
	router := mcpclient.NewMockRouter(tools)
	router.SetServers([]string{"harvest-mcp", "ontap-mcp"})
	for i := 0; i < perCap; i++ {
		router.SetToolServer(fmt.Sprintf("h%d", i), "harvest-mcp")
		router.SetToolServer(fmt.Sprintf("o%d", i), "ontap-mcp")
	}
	caps := []capability.Capability{
		{ID: "harvest", Name: "Harvest", State: capability.StateAllow, ServerName: "harvest-mcp"},
		{ID: "ontap", Name: "ONTAP", State: secondState, ServerName: "ontap-mcp"},
	}
	deps := &ChatDeps{Router: router, Capabilities: caps}
	if routingOn {
		deps.ToolRoutingMode = agent.ToolRoutingInBand
	}
	return New(deps)
}

// With in-band routing, GetChatCapabilities must report the largest single
// capability (the per-turn floor), not the sum across capabilities — otherwise
// the UI's read-write toggle blocks deployments whose total exceeds 128 even
// though each routed turn stays under the cap. Regression for RTB tripping the
// "N tools would be sent (max 128)" guard with routing enabled.
func TestRoutingBudgetReportsMaxNotSum(t *testing.T) {
	per := 80 // sum 160 > 128, each alone < 128
	srv := routingBudgetServer(per, capability.StateAllow, true)

	req := httptest.NewRequest(http.MethodGet, "/chat/capabilities?mode=read-write", nil)
	w := httptest.NewRecorder()
	srv.GetChatCapabilities(w, req)

	var resp struct {
		ToolBudgets struct {
			ReadWrite struct {
				Used int `json:"used"`
				Max  int `json:"max"`
			} `json:"read_write"`
		} `json:"tool_budgets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ToolBudgets.ReadWrite.Used != per {
		t.Errorf("routed used = %d, want %d (max single cap, not sum %d)",
			resp.ToolBudgets.ReadWrite.Used, per, per*2)
	}

	// Sanity: with routing OFF the same deployment reports the sum.
	srvOff := routingBudgetServer(per, capability.StateAllow, false)
	wOff := httptest.NewRecorder()
	srvOff.GetChatCapabilities(wOff, httptest.NewRequest(http.MethodGet, "/chat/capabilities?mode=read-write", nil))
	_ = json.Unmarshal(wOff.Body.Bytes(), &resp)
	if resp.ToolBudgets.ReadWrite.Used != per*2 {
		t.Errorf("non-routed used = %d, want %d (sum)", resp.ToolBudgets.ReadWrite.Used, per*2)
	}
}

// With in-band routing, enabling a capability that pushes the SUM over 128 must
// be allowed as long as no single capability alone exceeds the cap.
func TestPostCapabilitiesRoutingAllowsOverSumBudget(t *testing.T) {
	per := 80 // enabling ontap makes the sum 160, but each cap is 80 < 128
	srv := routingBudgetServer(per, capability.StateOff, true)

	body, _ := json.Marshal(map[string]any{
		"capabilities": map[string]string{"ontap": "allow"},
		"mode":         "read-write",
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/capabilities", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatCapabilities(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (routing splits the budget per server)", w.Code)
	}

	// Without routing the same enable must be rejected (sum exceeds the cap).
	srvOff := routingBudgetServer(per, capability.StateOff, false)
	wOff := httptest.NewRecorder()
	srvOff.PostChatCapabilities(wOff, httptest.NewRequest(http.MethodPost, "/chat/capabilities", bytes.NewReader(body)))
	if wOff.Code != http.StatusConflict {
		t.Fatalf("non-routed status = %d, want 409", wOff.Code)
	}
}

// S6b: the in-band routing group menu must be built from the mode-filtered
// tool set — in read-only mode a server's write tools are excluded, so the
// model is never offered tools it cannot call and read-only-annotated servers
// present a smaller group.
func TestRoutingMenuRespectsReadOnlyMode(t *testing.T) {
	router := mcpclient.NewMockRouter([]llm.ToolDef{
		mcpclient.MockReadOnlyTool("get_thing", "read"),
		mcpclient.MockTool("write_thing", "write"),
	})
	router.SetServers([]string{"harvest-mcp"})
	router.SetToolServer("get_thing", "harvest-mcp")
	router.SetToolServer("write_thing", "harvest-mcp")

	caps := []capability.Capability{
		{ID: "harvest", Name: "Harvest", State: capability.StateAllow, ServerName: "harvest-mcp"},
	}
	capStates := capability.ToMap(caps)
	connected := map[string]bool{"harvest-mcp": true}
	tsm := map[string]string{"get_thing": "harvest", "write_thing": "harvest"}

	ro := buildRoutingGroups(caps, capStates, "read-only", connected, tsm, router, 0)
	if len(ro) != 1 {
		t.Fatalf("read-only: got %d groups, want 1", len(ro))
	}
	if len(ro[0].ToolNames) != 1 || ro[0].ToolNames[0] != "get_thing" {
		t.Errorf("read-only menu tools = %v, want [get_thing] (write tool dropped)", ro[0].ToolNames)
	}

	rw := buildRoutingGroups(caps, capStates, "read-write", connected, tsm, router, 0)
	if len(rw) != 1 || len(rw[0].ToolNames) != 2 {
		t.Errorf("read-write menu tools = %v, want both tools", rw[0].ToolNames)
	}
}

// S8: with a group_expand_threshold set, an oversized capability is marked
// expandable in the routing menu so the model can load individual tools.
func TestRoutingMenuExpandsOversizedGroup(t *testing.T) {
	router := mcpclient.NewMockRouter([]llm.ToolDef{
		mcpclient.MockReadOnlyTool("ontap_a", "a"),
		mcpclient.MockReadOnlyTool("ontap_b", "b"),
		mcpclient.MockReadOnlyTool("ontap_c", "c"),
		mcpclient.MockReadOnlyTool("jira_search", "search"),
	})
	router.SetServers([]string{"ontap-mcp", "jira-mcp"})
	for tool, server := range map[string]string{
		"ontap_a": "ontap-mcp", "ontap_b": "ontap-mcp", "ontap_c": "ontap-mcp", "jira_search": "jira-mcp",
	} {
		router.SetToolServer(tool, server)
	}
	caps := []capability.Capability{
		{ID: "ontap", Name: "ONTAP", State: capability.StateAllow, ServerName: "ontap-mcp"},
		{ID: "jira", Name: "Jira", State: capability.StateAllow, ServerName: "jira-mcp"},
	}
	capStates := capability.ToMap(caps)
	connected := map[string]bool{"ontap-mcp": true, "jira-mcp": true}
	tsm := map[string]string{"ontap_a": "ontap", "ontap_b": "ontap", "ontap_c": "ontap", "jira_search": "jira"}

	groups := buildRoutingGroups(caps, capStates, "read-only", connected, tsm, router, 2)
	byID := map[string]capability.Group{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	if !byID["ontap"].Expandable {
		t.Errorf("ontap (3 tools > threshold 2) should be expandable")
	}
	if byID["jira"].Expandable {
		t.Errorf("jira (1 tool) should not be expandable")
	}
}

// A single capability whose own tools exceed the cap must still be rejected
// even with routing on — routing cannot split one server.
func TestPostCapabilitiesRoutingRejectsIrreducibleServer(t *testing.T) {
	per := agent.MaxToolsPerRequest + 5 // one server alone over the cap
	srv := routingBudgetServer(per, capability.StateOff, true)

	body, _ := json.Marshal(map[string]any{
		"capabilities": map[string]string{"ontap": "allow"},
		"mode":         "read-write",
	})
	req := httptest.NewRequest(http.MethodPost, "/chat/capabilities", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.PostChatCapabilities(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (single server exceeds cap)", w.Code)
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
