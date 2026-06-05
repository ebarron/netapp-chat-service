package capability

import (
	"fmt"
	"sort"
	"strings"
)

// Group is one entry in the tool-routing group menu (S7a). Each group maps
// 1:1 to a capability / MCP server: the menu is derived directly from the
// capability set, never from a hardcoded product→tools map, so it stays
// host-agnostic.
type Group struct {
	// ID is the capability ID; this is the value the model passes to
	// load_tools.
	ID string
	// Label is a human-readable name (capability name, falling back to ID).
	Label string
	// Description tells the model what the group covers. It is the operator-
	// configured capability description when set, otherwise auto-derived from
	// the group's tool names.
	Description string
	// ToolNames are the tools that become available when this group is loaded.
	ToolNames []string
	// Tools carries the per-tool metadata (name + one-line description) used
	// to render an expandable sub-index for oversized groups (S8). Sorted by
	// name; mirrors ToolNames.
	Tools []ToolInfo
	// Expandable is true when this group exceeds the configured size threshold
	// (S8): the routing menu lists its individual tools so the model can load
	// just the ones it needs via load_tools(tools:[…]) instead of pulling in
	// the whole server. Small groups load wholesale and leave this false.
	Expandable bool
}

// ToolInfo is the minimal per-tool metadata the registry needs to auto-derive
// a group description when the operator did not configure one.
type ToolInfo struct {
	Name        string
	Description string
}

// maxAutoDescTools bounds how many tool names are listed in an auto-derived
// group description, keeping the system-prompt menu compact.
const maxAutoDescTools = 8

// BuildGroups derives the ordered tool-routing group menu from the capability
// set. It is a pure function (no router/IO dependency) so it can be unit
// tested table-style.
//
// Only capabilities present and true in enabled are included; this mirrors
// interest.Catalog.BuildIndex(enabled), so the menu self-updates as MCP
// servers connect/disconnect or capabilities are toggled off — no restart.
// When enabled is nil, all supplied capabilities are included.
//
// toolsByCap maps capability ID -> the tools that belong to it (already
// resolved from tool→server→capability upstream). It is used both to populate
// Group.ToolNames and to auto-derive a Description when the capability has no
// configured one.
//
// Groups are ordered by capability ID for determinism.
//
// BuildGroups keeps every group whole (group-level S7a routing). Use
// BuildGroupsExpanding to enable tool-level expansion of oversized groups (S8).
func BuildGroups(caps []Capability, enabled map[string]bool, toolsByCap map[string][]ToolInfo) []Group {
	return BuildGroupsExpanding(caps, enabled, toolsByCap, 0)
}

// BuildGroupsExpanding is BuildGroups plus S8 intra-group expansion: any group
// whose tool count exceeds expandThreshold (when > 0) is marked Expandable and
// carries its per-tool metadata so the routing menu can list individual tools.
// expandThreshold <= 0 disables expansion, reproducing BuildGroups exactly.
func BuildGroupsExpanding(caps []Capability, enabled map[string]bool, toolsByCap map[string][]ToolInfo, expandThreshold int) []Group {
	groups := make([]Group, 0, len(caps))
	for _, c := range caps {
		if enabled != nil && !enabled[c.ID] {
			continue
		}
		tools := append([]ToolInfo(nil), toolsByCap[c.ID]...)
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			toolNames = append(toolNames, t.Name)
		}

		label := c.Name
		if label == "" {
			label = c.ID
		}
		desc := c.Description
		if desc == "" {
			desc = autoDescription(toolNames)
		}

		expandable := expandThreshold > 0 && len(toolNames) > expandThreshold

		groups = append(groups, Group{
			ID:          c.ID,
			Label:       label,
			Description: desc,
			ToolNames:   toolNames,
			Tools:       tools,
			Expandable:  expandable,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups
}

// autoDescription builds a compact description from a group's tool names. The
// names come from the MCP server itself (content-derived), so this carries no
// host semantics.
func autoDescription(toolNames []string) string {
	if len(toolNames) == 0 {
		return "(no tools currently available)"
	}
	shown := toolNames
	suffix := ""
	if len(shown) > maxAutoDescTools {
		shown = shown[:maxAutoDescTools]
		suffix = fmt.Sprintf(", … (%d tools total)", len(toolNames))
	}
	return "Tools: " + strings.Join(shown, ", ") + suffix
}

// RenderGroupIndex produces the compact markdown table injected into the
// system prompt for in-band tool routing. It mirrors the shape of the
// interest index. Returns "" when there are no groups, so the caller can omit
// the section entirely (and emit no dangling header).
func RenderGroupIndex(groups []Group) string {
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| Group ID | Name | Covers |\n")
	b.WriteString("|----------|------|--------|\n")
	var expandable []Group
	for _, g := range groups {
		covers := g.Description
		if g.Expandable {
			covers = fmt.Sprintf("Large server (%d tools) — load only the individual tools you need (listed below), not the whole group", len(g.ToolNames))
			expandable = append(expandable, g)
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", g.ID, g.Label, covers)
	}
	// S8: render a per-tool sub-index for each oversized group so the model can
	// pick a handful of tools instead of loading the whole server.
	for _, g := range expandable {
		fmt.Fprintf(&b, "\n#### Tools in group \"%s\" (load individually via load_tools tools:[…])\n\n", g.ID)
		b.WriteString("| Tool | Description |\n")
		b.WriteString("|------|-------------|\n")
		for _, t := range g.Tools {
			desc := oneLine(t.Description)
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&b, "| %s | %s |\n", t.Name, desc)
		}
	}
	return b.String()
}

// oneLine collapses internal whitespace/newlines so a tool description renders
// as a single clean table cell.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
