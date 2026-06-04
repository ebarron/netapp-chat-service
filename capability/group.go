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
func BuildGroups(caps []Capability, enabled map[string]bool, toolsByCap map[string][]ToolInfo) []Group {
	groups := make([]Group, 0, len(caps))
	for _, c := range caps {
		if enabled != nil && !enabled[c.ID] {
			continue
		}
		tools := toolsByCap[c.ID]
		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			toolNames = append(toolNames, t.Name)
		}
		sort.Strings(toolNames)

		label := c.Name
		if label == "" {
			label = c.ID
		}
		desc := c.Description
		if desc == "" {
			desc = autoDescription(toolNames)
		}

		groups = append(groups, Group{
			ID:          c.ID,
			Label:       label,
			Description: desc,
			ToolNames:   toolNames,
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
	for _, g := range groups {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", g.ID, g.Label, g.Description)
	}
	return b.String()
}
