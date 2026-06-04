package capability

import (
	"reflect"
	"strings"
	"testing"
)

// fixedCaps is a representative capability set used across the group tests.
func fixedCaps() []Capability {
	return []Capability{
		{ID: "jira", Name: "Jira", Description: "Issue tracking", State: StateAllow, ServerName: "jira-mcp"},
		{ID: "zoom", Name: "Zoom", State: StateAllow, ServerName: "zoom-mcp"},
		{ID: "ontap", State: StateAllow, ServerName: "ontap-mcp"},
	}
}

func fixedTools() map[string][]ToolInfo {
	return map[string][]ToolInfo{
		"jira":  {{Name: "create_issue"}, {Name: "search_issues"}},
		"zoom":  {{Name: "list_meetings"}},
		"ontap": {{Name: "list_volumes"}, {Name: "get_cluster"}},
	}
}

// TestBuildGroupsDerivation verifies the menu is derived deterministically
// (ordered by ID) with configured names/descriptions and auto-derived
// descriptions as a fallback.
func TestBuildGroupsDerivation(t *testing.T) {
	groups := BuildGroups(fixedCaps(), nil, fixedTools())

	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	// Ordered by ID: jira, ontap, zoom.
	wantOrder := []string{"jira", "ontap", "zoom"}
	for i, g := range groups {
		if g.ID != wantOrder[i] {
			t.Errorf("groups[%d].ID = %q, want %q", i, g.ID, wantOrder[i])
		}
	}

	// jira: configured description wins.
	if groups[0].Description != "Issue tracking" {
		t.Errorf("jira description = %q, want configured", groups[0].Description)
	}
	// ontap: no configured name → label falls back to ID; no description →
	// auto-derived from sorted tool names.
	if groups[1].Label != "ontap" {
		t.Errorf("ontap label = %q, want fallback to ID", groups[1].Label)
	}
	if groups[1].Description != "Tools: get_cluster, list_volumes" {
		t.Errorf("ontap description = %q, want auto-derived sorted tools", groups[1].Description)
	}
}

// TestBuildGroupsEnabledFilter verifies that capabilities absent/false in the
// enabled map are omitted, and re-enabling re-adds them (the connect/disconnect
// and off→on behavior).
func TestBuildGroupsEnabledFilter(t *testing.T) {
	caps := fixedCaps()
	tools := fixedTools()

	enabled := map[string]bool{"jira": true, "ontap": true} // zoom disabled/disconnected
	groups := BuildGroups(caps, enabled, tools)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (zoom omitted)", len(groups))
	}
	for _, g := range groups {
		if g.ID == "zoom" {
			t.Errorf("zoom should be omitted when not enabled")
		}
	}

	// Re-enable zoom → it reappears, deterministically ordered.
	enabled["zoom"] = true
	groups = BuildGroups(caps, enabled, tools)
	if len(groups) != 3 {
		t.Fatalf("got %d groups after re-enable, want 3", len(groups))
	}
}

// TestBuildGroupsEmptyToolset verifies a group with no currently-available
// tools still appears with a clear placeholder description.
func TestBuildGroupsEmptyToolset(t *testing.T) {
	caps := []Capability{{ID: "search", State: StateAllow, ServerName: "search-mcp"}}
	groups := BuildGroups(caps, nil, map[string][]ToolInfo{})
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if !strings.Contains(groups[0].Description, "no tools") {
		t.Errorf("description = %q, want a no-tools placeholder", groups[0].Description)
	}
	if len(groups[0].ToolNames) != 0 {
		t.Errorf("ToolNames = %v, want empty", groups[0].ToolNames)
	}
}

// TestBuildGroupsAutoDescTruncation verifies the auto-derived description is
// bounded so the prompt menu stays compact for fan-out servers.
func TestBuildGroupsAutoDescTruncation(t *testing.T) {
	var tools []ToolInfo
	for i := 0; i < maxAutoDescTools+5; i++ {
		tools = append(tools, ToolInfo{Name: "tool_" + string(rune('a'+i))})
	}
	caps := []Capability{{ID: "big", State: StateAllow, ServerName: "big-mcp"}}
	groups := BuildGroups(caps, nil, map[string][]ToolInfo{"big": tools})
	desc := groups[0].Description
	if !strings.Contains(desc, "tools total") {
		t.Errorf("description = %q, want a truncation suffix", desc)
	}
}

// TestRenderGroupIndex verifies the markdown table lists every group in order
// and that an empty menu renders nothing (no dangling header).
func TestRenderGroupIndex(t *testing.T) {
	groups := BuildGroups(fixedCaps(), nil, fixedTools())
	idx := RenderGroupIndex(groups)

	if !strings.Contains(idx, "| Group ID | Name | Covers |") {
		t.Errorf("index missing header:\n%s", idx)
	}
	for _, id := range []string{"jira", "ontap", "zoom"} {
		if !strings.Contains(idx, "| "+id+" |") {
			t.Errorf("index missing group %q:\n%s", id, idx)
		}
	}
	// jira appears before zoom (ID order).
	if strings.Index(idx, "| jira |") > strings.Index(idx, "| zoom |") {
		t.Errorf("groups not ordered by ID:\n%s", idx)
	}

	if got := RenderGroupIndex(nil); got != "" {
		t.Errorf("RenderGroupIndex(nil) = %q, want empty", got)
	}
}

// TestBuildGroupsNoHostSemantics is a property-style guard: group IDs and
// labels must derive only from capability IDs/names + the tool names supplied
// by the server. None of the inputs here carry product/view strings, so none
// must appear in the output.
func TestBuildGroupsNoHostSemantics(t *testing.T) {
	caps := fixedCaps()
	groups := BuildGroups(caps, nil, fixedTools())

	allowedIDs := map[string]bool{}
	allowedLabels := map[string]bool{}
	for _, c := range caps {
		allowedIDs[c.ID] = true
		label := c.Name
		if label == "" {
			label = c.ID
		}
		allowedLabels[label] = true
	}
	for _, g := range groups {
		if !allowedIDs[g.ID] {
			t.Errorf("group ID %q not derived from a capability ID", g.ID)
		}
		if !allowedLabels[g.Label] {
			t.Errorf("group label %q not derived from capability name/ID", g.Label)
		}
	}
}

// TestBuildGroupsToolNamesSorted verifies ToolNames are sorted for stable
// output regardless of input order.
func TestBuildGroupsToolNamesSorted(t *testing.T) {
	tools := map[string][]ToolInfo{"jira": {{Name: "search_issues"}, {Name: "create_issue"}}}
	caps := []Capability{{ID: "jira", State: StateAllow, ServerName: "jira-mcp"}}
	groups := BuildGroups(caps, nil, tools)
	want := []string{"create_issue", "search_issues"}
	if !reflect.DeepEqual(groups[0].ToolNames, want) {
		t.Errorf("ToolNames = %v, want sorted %v", groups[0].ToolNames, want)
	}
}
