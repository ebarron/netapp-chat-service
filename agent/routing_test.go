package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// twoGroupAgent builds an in-band agent with a jira group (create_issue,
// read-only) and a bitbucket group (search_repo, read-only), plus an optional
// jira write tool (delete_issue). Capabilities are all "allow"; mode is the
// supplied agent mode. forceLoad toggles the forced-first-step nudge.
func twoGroupAgent(provider llm.Provider, mode string, alwaysOn []string, maxTools int, forceLoad bool) *Agent {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("create_issue", "Create a Jira issue"),
		mcpclient.MockReadOnlyTool("search_repo", "Search Bitbucket"),
		mcpclient.MockTool("delete_issue", "Delete a Jira issue (write)"), // not read-only
	}
	router := mcpclient.NewMockRouter(tools)
	toolServer := map[string]string{
		"create_issue": "jira",
		"search_repo":  "bitbucket",
		"delete_issue": "jira",
	}
	groups := []capability.Group{{ID: "bitbucket", Label: "Bitbucket"}, {ID: "jira", Label: "Jira"}}
	return New(provider, router,
		WithCapabilityFilter(capability.CapabilityMap{
			"jira":      capability.StateAllow,
			"bitbucket": capability.StateAllow,
		}, mode),
		WithToolServerMap(toolServer),
		WithToolRouting(ToolRoutingInBand, groups, alwaysOn, maxTools, forceLoad),
	)
}

// toolNames extracts the set of tool names from a filtered tool list.
func toolNameSet(tools []llm.ToolDef) map[string]bool {
	s := make(map[string]bool, len(tools))
	for _, t := range tools {
		s[t.Name] = true
	}
	return s
}

func loadGroups(t *testing.T, a *Agent, groups ...string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]any{"groups": groups})
	res, err := a.handleLoadTools(context.Background(), in)
	if err != nil {
		t.Fatalf("handleLoadTools(%v) error = %v", groups, err)
	}
	return res
}

// --- Layer 3: load_tools tool + per-message state ---

// TestLoadToolsActivatesGroup verifies that load_tools activates exactly the
// named group's tools on the next filteredTools call, and that nothing is
// available before a load (only the internal load_tools tool).
func TestLoadToolsActivatesGroup(t *testing.T) {
	ag := twoGroupAgent(nil, "read-only", nil, 0, true)

	// Nothing loaded → only the internal load_tools tool is present.
	ft, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	got := toolNameSet(ft)
	if !got["load_tools"] {
		t.Error("load_tools internal tool should always be present in in-band mode")
	}
	if got["create_issue"] || got["search_repo"] {
		t.Errorf("no MCP tools should be present before loading; got %v", got)
	}

	loadGroups(t, ag, "jira")

	ft, err = ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	got = toolNameSet(ft)
	if !got["create_issue"] {
		t.Error("create_issue (jira) should be available after load_tools([jira])")
	}
	if got["search_repo"] {
		t.Error("search_repo (bitbucket) should NOT be available — bitbucket not loaded")
	}
}

// TestLoadToolsUnknownGroup verifies an unknown group yields a recoverable
// structured result and leaves the active set unchanged (no panic).
func TestLoadToolsUnknownGroup(t *testing.T) {
	ag := twoGroupAgent(nil, "read-only", nil, 0, true)

	res := loadGroups(t, ag, "does-not-exist")
	if !strings.Contains(strings.ToLower(res), "unknown") {
		t.Errorf("result = %q, want it to report the unknown group", res)
	}
	// Set unchanged: still no MCP tools available.
	ft, _ := ag.filteredTools()
	got := toolNameSet(ft)
	if got["create_issue"] || got["search_repo"] {
		t.Errorf("unknown group must not activate anything; got %v", got)
	}
}

// TestLoadToolsIdempotentUnion verifies repeated calls union the active set.
func TestLoadToolsIdempotentUnion(t *testing.T) {
	ag := twoGroupAgent(nil, "read-only", nil, 0, true)

	loadGroups(t, ag, "jira")
	loadGroups(t, ag, "jira")      // duplicate — no-op
	loadGroups(t, ag, "bitbucket") // union

	ft, _ := ag.filteredTools()
	got := toolNameSet(ft)
	if !got["create_issue"] || !got["search_repo"] {
		t.Errorf("both groups should be active after union; got %v", got)
	}
}

// TestLoadToolsAlwaysOn verifies always_on groups are active from turn 1
// without any load_tools call.
func TestLoadToolsAlwaysOn(t *testing.T) {
	ag := twoGroupAgent(nil, "read-only", []string{"jira"}, 0, true)

	ft, _ := ag.filteredTools()
	got := toolNameSet(ft)
	if !got["create_issue"] {
		t.Error("always_on jira tool should be active without a load_tools call")
	}
	if got["search_repo"] {
		t.Error("bitbucket should not be active — not always_on and not loaded")
	}
}

// TestLoadToolsHonorsReadOnlyMode verifies a loaded group still honors the
// read-only mode filter: write tools stay filtered out even after the group
// is loaded.
func TestLoadToolsHonorsReadOnlyMode(t *testing.T) {
	ag := twoGroupAgent(nil, "read-only", nil, 0, true)
	loadGroups(t, ag, "jira")

	ft, _ := ag.filteredTools()
	got := toolNameSet(ft)
	if !got["create_issue"] {
		t.Error("read-only jira tool should be present")
	}
	if got["delete_issue"] {
		t.Error("write tool delete_issue must stay filtered in read-only mode even when jira is loaded")
	}

	// In read-write the write tool appears once jira is loaded.
	agRW := twoGroupAgent(nil, "read-write", nil, 0, true)
	loadGroups(t, agRW, "jira")
	ftRW, _ := agRW.filteredTools()
	if !toolNameSet(ftRW)["delete_issue"] {
		t.Error("write tool delete_issue should be present in read-write mode after loading jira")
	}
}

// --- Layer 4: filteredTools restriction, budget, enforcement ---

// TestRoutingModeOffUnaffected verifies that with mode off, filteredTools
// behaves exactly as before: no load_tools tool, all enabled tools present.
func TestRoutingModeOffUnaffected(t *testing.T) {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("create_issue", ""),
		mcpclient.MockReadOnlyTool("search_repo", ""),
	}
	router := mcpclient.NewMockRouter(tools)
	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{
			"jira": capability.StateAllow, "bitbucket": capability.StateAllow,
		}, "read-only"),
		WithToolServerMap(map[string]string{"create_issue": "jira", "search_repo": "bitbucket"}),
		WithToolRouting(ToolRoutingOff, nil, nil, 0, false),
	)
	ft, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	got := toolNameSet(ft)
	if got["load_tools"] {
		t.Error("mode off must not register the load_tools tool")
	}
	if !got["create_issue"] || !got["search_repo"] {
		t.Errorf("mode off must return all enabled tools; got %v", got)
	}
}

// TestRoutedBudgetOverflowNamesGroup verifies a single loaded group whose tool
// count exceeds the budget produces ErrTooManyTools naming that group.
func TestRoutedBudgetOverflowNamesGroup(t *testing.T) {
	n := MaxToolsPerRequest + 3
	tools := make([]llm.ToolDef, n)
	toolServer := make(map[string]string, n)
	for i := range tools {
		name := "big_tool_" + itoa(i)
		tools[i] = mcpclient.MockReadOnlyTool(name, "")
		toolServer[name] = "big"
	}
	router := mcpclient.NewMockRouter(tools)
	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{"big": capability.StateAllow}, "read-only"),
		WithToolServerMap(toolServer),
		WithToolRouting(ToolRoutingInBand, []capability.Group{{ID: "big"}}, nil, 0, false),
	)
	loadGroups(t, ag, "big")

	_, err := ag.filteredTools()
	if !errors.Is(err, ErrTooManyTools) {
		t.Fatalf("filteredTools() error = %v, want ErrTooManyTools", err)
	}
	if !strings.Contains(err.Error(), "big") {
		t.Errorf("error should name the offending group: %v", err)
	}
}

// TestRoutedBudgetMaxToolsCap verifies the optional max_tools cap is enforced
// below MaxToolsPerRequest.
func TestRoutedBudgetMaxToolsCap(t *testing.T) {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("a", ""),
		mcpclient.MockReadOnlyTool("b", ""),
		mcpclient.MockReadOnlyTool("c", ""),
	}
	router := mcpclient.NewMockRouter(tools)
	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{"g": capability.StateAllow}, "read-only"),
		WithToolServerMap(map[string]string{"a": "g", "b": "g", "c": "g"}),
		WithToolRouting(ToolRoutingInBand, []capability.Group{{ID: "g"}}, nil, 2, false),
	)
	loadGroups(t, ag, "g")
	if _, err := ag.filteredTools(); !errors.Is(err, ErrTooManyTools) {
		t.Fatalf("expected ErrTooManyTools with max_tools=2 and 3 loaded tools, got %v", err)
	}
}

// TestForcedFirstStepNudgeThenLoad verifies that when the model answers
// without loading a group, it gets one corrective nudge, after which it loads
// and completes (compliant).
func TestForcedFirstStepNudgeThenLoad(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("I'll answer directly."), // skip attempt
			llm.MockToolCallResponse("tc-1", "load_tools", map[string]any{"groups": []string{"jira"}}),
			llm.MockTextResponse("Here is your answer."),
		},
	}
	ag := twoGroupAgent(provider, "read-only", nil, 0, true)
	ag.Router.(*mcpclient.MockRouter).SetResult("create_issue", "ok")

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "do a jira thing"}})

	var doneCount int
	for _, e := range events {
		if e.Type == EventDone {
			doneCount++
		}
	}
	if doneCount == 0 {
		t.Fatal("expected the run to complete with EventDone")
	}
	stats := ag.LastRoutingStats()
	if stats.LoadCalls != 1 {
		t.Errorf("LoadCalls = %d, want 1 (model loaded after the nudge)", stats.LoadCalls)
	}
	if !stats.Compliant {
		t.Error("stats.Compliant should be true after a load")
	}
	// All three mock responses should have been consumed (nudge gave a 2nd
	// turn, then a 3rd for the final answer).
	if len(provider.Responses) != 0 {
		t.Errorf("provider had %d unconsumed responses, want 0", len(provider.Responses))
	}
}

// TestForcedFirstStepGracefulFallback verifies that after a single nudge the
// model is allowed to give a tool-less answer (no infinite loop), and the run
// is recorded as a skip.
func TestForcedFirstStepGracefulFallback(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("Answering without tools."),
			llm.MockTextResponse("Still answering without tools."),
		},
	}
	ag := twoGroupAgent(provider, "read-only", nil, 0, true)

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "hello"}})

	var doneCount int
	for _, e := range events {
		if e.Type == EventDone {
			doneCount++
		}
	}
	if doneCount != 1 {
		t.Fatalf("expected exactly one EventDone (graceful fallback), got %d", doneCount)
	}
	stats := ag.LastRoutingStats()
	if stats.LoadCalls != 0 {
		t.Errorf("LoadCalls = %d, want 0", stats.LoadCalls)
	}
	if !stats.Skipped {
		t.Error("stats.Skipped should be true when the model answered without loading")
	}
}

// --- Layer 5: telemetry ---

// TestRoutingTelemetryCompliant verifies a first-call load is recorded as
// compliant and not a skip.
func TestRoutingTelemetryCompliant(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "load_tools", map[string]any{"groups": []string{"jira"}}),
			llm.MockTextResponse("Done."),
		},
	}
	ag := twoGroupAgent(provider, "read-only", nil, 0, true)

	collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "jira please"}})

	stats := ag.LastRoutingStats()
	if stats.Mode != ToolRoutingInBand {
		t.Errorf("Mode = %q, want %q", stats.Mode, ToolRoutingInBand)
	}
	if stats.GroupsOffered != 2 {
		t.Errorf("GroupsOffered = %d, want 2", stats.GroupsOffered)
	}
	if stats.LoadCalls != 1 || stats.Reloads != 0 {
		t.Errorf("LoadCalls=%d Reloads=%d, want 1/0", stats.LoadCalls, stats.Reloads)
	}
	if stats.Skipped {
		t.Error("Skipped should be false for a compliant first-call load")
	}
	if !stats.Compliant {
		t.Error("Compliant should be true")
	}
	if len(stats.GroupsLoaded) != 1 || stats.GroupsLoaded[0] != "jira" {
		t.Errorf("GroupsLoaded = %v, want [jira]", stats.GroupsLoaded)
	}
}

// TestRoutingTelemetryReloads verifies mid-task re-loads are counted.
func TestRoutingTelemetryReloads(t *testing.T) {
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "load_tools", map[string]any{"groups": []string{"jira"}}),
			llm.MockToolCallResponse("tc-2", "load_tools", map[string]any{"groups": []string{"bitbucket"}}),
			llm.MockTextResponse("Done."),
		},
	}
	ag := twoGroupAgent(provider, "read-only", nil, 0, true)

	collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "jira then bitbucket"}})

	stats := ag.LastRoutingStats()
	if stats.LoadCalls != 2 {
		t.Errorf("LoadCalls = %d, want 2", stats.LoadCalls)
	}
	if stats.Reloads != 1 {
		t.Errorf("Reloads = %d, want 1", stats.Reloads)
	}
	if len(stats.GroupsLoaded) != 2 {
		t.Errorf("GroupsLoaded = %v, want both groups", stats.GroupsLoaded)
	}
}

// itoa is a tiny strconv.Itoa to avoid importing strconv only for tests.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
