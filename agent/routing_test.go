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

// --- S8: intra-group (tool-level) selection ---

// oversizedGroupAgent builds an in-band agent with an expandable "ontap" group
// (3 read-only tools) and a small "jira" group (1 tool). It mirrors the menu a
// server would emit with group_expand_threshold set below ontap's tool count.
func oversizedGroupAgent(mode string) *Agent {
	tools := []llm.ToolDef{
		mcpclient.MockReadOnlyTool("ontap_get_volume", "Get a volume"),
		mcpclient.MockReadOnlyTool("ontap_list_volumes", "List volumes"),
		mcpclient.MockReadOnlyTool("ontap_get_cluster", "Cluster health"),
		mcpclient.MockReadOnlyTool("jira_search", "Search Jira"),
	}
	router := mcpclient.NewMockRouter(tools)
	toolServer := map[string]string{
		"ontap_get_volume":   "ontap",
		"ontap_list_volumes": "ontap",
		"ontap_get_cluster":  "ontap",
		"jira_search":        "jira",
	}
	groups := []capability.Group{
		{ID: "jira", Label: "Jira", ToolNames: []string{"jira_search"}},
		{ID: "ontap", Label: "ONTAP", Expandable: true, ToolNames: []string{"ontap_get_cluster", "ontap_get_volume", "ontap_list_volumes"}},
	}
	return New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{
			"ontap": capability.StateAllow,
			"jira":  capability.StateAllow,
		}, mode),
		WithToolServerMap(toolServer),
		WithToolRouting(ToolRoutingInBand, groups, nil, 0, true),
	)
}

func loadIndividualTools(t *testing.T, a *Agent, tools ...string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]any{"tools": tools})
	res, err := a.handleLoadTools(context.Background(), in)
	if err != nil {
		t.Fatalf("handleLoadTools(tools=%v) error = %v", tools, err)
	}
	return res
}

// TestLoadToolsIndividualActivatesOnlyThatTool verifies S8: loading a single
// tool from an oversized group activates exactly that tool, not the whole
// group — the entire point of tool-level selection.
func TestLoadToolsIndividualActivatesOnlyThatTool(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	loadIndividualTools(t, ag, "ontap_get_volume")

	ft, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	got := toolNameSet(ft)
	if !got["ontap_get_volume"] {
		t.Error("ontap_get_volume should be active after loading it individually")
	}
	if got["ontap_list_volumes"] || got["ontap_get_cluster"] {
		t.Errorf("loading one tool must NOT activate the rest of the group; got %v", got)
	}
	if got["jira_search"] {
		t.Error("jira_search should not be active — neither its group nor the tool was loaded")
	}
}

// TestLoadToolsMixedGroupAndTool verifies a single call can load a whole small
// group and an individual tool from an oversized one.
func TestLoadToolsMixedGroupAndTool(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	in, _ := json.Marshal(map[string]any{
		"groups": []string{"jira"},
		"tools":  []string{"ontap_get_cluster"},
	})
	if _, err := ag.handleLoadTools(context.Background(), in); err != nil {
		t.Fatalf("handleLoadTools error = %v", err)
	}
	got := toolNameSet(mustFilter(t, ag))
	if !got["jira_search"] {
		t.Error("jira_search should be active (whole jira group loaded)")
	}
	if !got["ontap_get_cluster"] {
		t.Error("ontap_get_cluster should be active (loaded individually)")
	}
	if got["ontap_get_volume"] || got["ontap_list_volumes"] {
		t.Errorf("other ontap tools must stay inactive; got %v", got)
	}
}

// TestLoadToolsUnknownTool verifies an unknown tool name is recoverable (not a
// hard error) and activates nothing.
func TestLoadToolsUnknownTool(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	in, _ := json.Marshal(map[string]any{"tools": []string{"ontap_nonexistent"}})
	res, err := ag.handleLoadTools(context.Background(), in)
	if err != nil {
		t.Fatalf("handleLoadTools error = %v", err)
	}
	if !strings.Contains(strings.ToLower(res), "unknown tool") {
		t.Errorf("result = %q, want it to report the unknown tool", res)
	}
	if len(toolNameSet(mustFilter(t, ag))) > 1 { // only load_tools internal remains
		t.Errorf("unknown tool must not activate anything")
	}
}

// TestLoadToolsRequiresGroupsOrTools verifies an empty call is a usage error.
func TestLoadToolsRequiresGroupsOrTools(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	if _, err := ag.handleLoadTools(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Error("expected an error when neither groups nor tools is supplied")
	}
}

// TestToolLevelLoadCountsAsSelection verifies a tool-level load suppresses the
// forced-first-step nudge (the model has made a selection).
func TestToolLevelLoadCountsAsSelection(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	if !ag.shouldForceGroupLoad() {
		t.Fatal("precondition: nudge should fire before any selection")
	}
	loadIndividualTools(t, ag, "ontap_get_volume")
	if ag.shouldForceGroupLoad() {
		t.Error("a tool-level load should count as a selection and suppress the nudge")
	}
}

// TestToolLevelTelemetry verifies a tool-level load surfaces in telemetry: the
// owning group appears in GroupsLoaded and the tool in ToolsLoaded.
func TestToolLevelTelemetry(t *testing.T) {
	ag := oversizedGroupAgent("read-only")
	loadIndividualTools(t, ag, "ontap_get_volume")

	stats := ag.LastRoutingStats()
	if len(stats.GroupsLoaded) != 1 || stats.GroupsLoaded[0] != "ontap" {
		t.Errorf("GroupsLoaded = %v, want [ontap] (owning group of the loaded tool)", stats.GroupsLoaded)
	}
	if len(stats.ToolsLoaded) != 1 || stats.ToolsLoaded[0] != "ontap_get_volume" {
		t.Errorf("ToolsLoaded = %v, want [ontap_get_volume]", stats.ToolsLoaded)
	}
}

func mustFilter(t *testing.T, a *Agent) []llm.ToolDef {
	t.Helper()
	ft, err := a.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	return ft
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
