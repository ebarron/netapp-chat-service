package eval

import "github.com/ebarron/netapp-chat-service/capability"

// cap is a small constructor for an allow-state capability whose server name
// equals its ID (the synthetic-environment convention).
func cap(id, name, desc string) capability.Capability {
	return capability.Capability{
		ID:          id,
		Name:        name,
		Description: desc,
		State:       capability.StateAllow,
		ServerName:  id,
	}
}

// netappCaps is a representative multi-MCP environment modeled on a real
// NetApp-style deployment: issue tracking (jira), docs (confluence), source
// (bitbucket), storage telemetry (harvest/ONTAP), and meetings (zoom). Tool
// names are content-derived so the auto-generated group descriptions read the
// way production ones do.
func netappCaps() []capability.Capability {
	return []capability.Capability{
		cap("jira", "Jira", "Issue and ticket tracking"),
		cap("confluence", "Confluence", "Wiki pages, runbooks, and design docs"),
		cap("bitbucket", "Bitbucket", "Source repositories and pull requests"),
		cap("harvest", "Harvest", "ONTAP cluster and volume telemetry"),
		cap("zoom", "Zoom", "Meetings and scheduling"),
	}
}

func netappTools() []ToolSpec {
	return []ToolSpec{
		{Name: "jira_search_issues", Capability: "jira", Description: "Search Jira issues", ReadOnly: true},
		{Name: "jira_get_issue", Capability: "jira", Description: "Get a Jira issue", ReadOnly: true},
		{Name: "jira_create_issue", Capability: "jira", Description: "Create a Jira issue"},

		{Name: "confluence_search", Capability: "confluence", Description: "Search Confluence", ReadOnly: true},
		{Name: "confluence_get_page", Capability: "confluence", Description: "Get a Confluence page", ReadOnly: true},
		{Name: "confluence_create_page", Capability: "confluence", Description: "Create a Confluence page"},

		{Name: "bitbucket_list_prs", Capability: "bitbucket", Description: "List pull requests", ReadOnly: true},
		{Name: "bitbucket_get_pr", Capability: "bitbucket", Description: "Get a pull request", ReadOnly: true},

		{Name: "harvest_list_volumes", Capability: "harvest", Description: "List ONTAP volumes", ReadOnly: true},
		{Name: "harvest_get_volume", Capability: "harvest", Description: "Get volume capacity", ReadOnly: true},
		{Name: "harvest_get_cluster", Capability: "harvest", Description: "Get cluster health", ReadOnly: true},

		{Name: "zoom_list_meetings", Capability: "zoom", Description: "List Zoom meetings", ReadOnly: true},
		{Name: "zoom_create_meeting", Capability: "zoom", Description: "Schedule a Zoom meeting"},
	}
}

// DefaultScenarios is the seed routing-eval suite. Each scenario shares the
// netappCaps/netappTools environment so the menu the model sees is identical
// across cases — only the user message (and expected routing) changes. Extend
// this set as real misroutes are observed in production telemetry.
func DefaultScenarios() []Scenario {
	caps, tools := netappCaps(), netappTools()
	mk := func(name, msg string, expected ...string) Scenario {
		return Scenario{Name: name, Message: msg, Caps: caps, Tools: tools, Expected: expected}
	}
	expand := func(name, msg string, threshold int, expected ...string) Scenario {
		s := mk(name, msg, expected...)
		s.ExpandThreshold = threshold
		return s
	}
	return []Scenario{
		mk("jira_single", "Find the open Jira issues assigned to me.", "jira"),
		mk("confluence_single", "Search Confluence for the onboarding runbook.", "confluence"),
		mk("bitbucket_single", "List the open pull requests in the platform repo.", "bitbucket"),
		mk("ontap_single", "Show me the volumes on the production cluster and their capacity.", "harvest"),
		mk("zoom_single", "What Zoom meetings do I have scheduled today?", "zoom"),
		mk("jira_confluence", "Find the Jira bug about login failures and link the related Confluence design doc.", "jira", "confluence"),
		mk("ontap_jira", "Check volume capacity on the cluster and tell me which Jira tickets mention it.", "harvest", "jira"),
		mk("greeting_skip", "Hi there, what can you help me with?"),
		// S8: with expansion enabled, harvest (3 tools) is offered tool-by-tool.
		// The model should still route to harvest by loading individual tools,
		// which surfaces harvest in GroupsLoaded.
		expand("ontap_single_expanded", "Show me the volumes on the production cluster and their capacity.", 2, "harvest"),
	}
}
