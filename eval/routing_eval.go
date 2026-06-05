// Package eval is the offline harness for measuring in-band tool-routing
// (S7a) quality — the Layer 6 evaluation called out in
// docs/high-tool-count-scaling.md. Given a fixed environment of MCP
// capabilities + tools and a user message, it runs the agent once and checks
// which capability groups the model chose to load (via load_tools) against an
// expected set. The aggregate top-1 / exact / skip rates are the empirical
// basis for the S7a→S7b graduation decision.
//
// The harness is provider-agnostic and performs no network I/O itself: pass a
// deterministic *llm.MockProvider for CI self-tests, or a real provider for an
// opt-in accuracy run. See routing_eval_test.go, which gates the real-provider
// run behind an environment variable so CI stays hermetic.
package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ebarron/netapp-chat-service/agent"
	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// ToolSpec describes one MCP tool in a scenario's synthetic environment. The
// owning capability ID doubles as the server name (the synthetic environment
// keeps a 1:1 server↔capability mapping, matching production).
type ToolSpec struct {
	Name        string
	Capability  string // capability/group ID this tool belongs to
	Description string
	ReadOnly    bool
}

// Scenario is one routing eval case: a fixed capability/tool environment, a
// user message, and the capability groups we expect the model to load. An
// empty Expected means the model should load nothing (e.g. a greeting).
type Scenario struct {
	Name     string
	Message  string
	Mode     string // "read-only" (default) or "read-write"
	Caps     []capability.Capability
	Tools    []ToolSpec
	Expected []string
	// ExpandThreshold enables S8 intra-group expansion for this scenario: any
	// group larger than this is offered tool-by-tool. 0 keeps every group
	// whole (group-level S7a). Loading an individual tool still surfaces its
	// owning group in GroupsLoaded, so Expected is scored the same way.
	ExpandThreshold int
}

func (sc Scenario) mode() string {
	if sc.Mode == "" {
		return "read-only"
	}
	return sc.Mode
}

// Result is the scored outcome of one scenario.
type Result struct {
	Name      string
	Expected  []string
	Loaded    []string
	Hit       bool // every expected group was loaded (recall == 1.0)
	Exact     bool // loaded set == expected set
	Skipped   bool // model loaded nothing despite a non-empty menu
	LoadCalls int
}

// RunScenario builds the synthetic environment, runs the agent once with the
// supplied provider, and scores which groups were loaded. It never calls the
// network itself; any network access comes from the provider.
func RunScenario(ctx context.Context, provider llm.Provider, sc Scenario) Result {
	tools := make([]llm.ToolDef, 0, len(sc.Tools))
	toolServer := make(map[string]string, len(sc.Tools))
	toolsByCap := make(map[string][]capability.ToolInfo)
	for _, ts := range sc.Tools {
		def := mcpclient.MockTool(ts.Name, ts.Description)
		def.ReadOnlyHint = ts.ReadOnly
		tools = append(tools, def)
		toolServer[ts.Name] = ts.Capability
		toolsByCap[ts.Capability] = append(toolsByCap[ts.Capability],
			capability.ToolInfo{Name: ts.Name, Description: ts.Description})
	}

	router := mcpclient.NewMockRouter(tools)
	for name, capID := range toolServer {
		router.SetToolServer(name, capID)
		// Give every tool a benign result so a real model can complete its turn
		// after loading a group; routing telemetry is captured regardless.
		router.SetResult(name, "(ok)")
	}

	servers := make([]string, 0, len(sc.Caps))
	states := make(capability.CapabilityMap, len(sc.Caps))
	enabled := make(map[string]bool, len(sc.Caps))
	for _, c := range sc.Caps {
		server := c.ServerName
		if server == "" {
			server = c.ID
		}
		servers = append(servers, server)
		st := c.State
		if st == "" {
			st = capability.StateAllow
		}
		states[c.ID] = st
		enabled[c.ID] = st != capability.StateOff
	}
	router.SetServers(servers)

	groups := capability.BuildGroupsExpanding(sc.Caps, enabled, toolsByCap, sc.ExpandThreshold)

	ag := agent.New(provider, router,
		agent.WithCapabilityFilter(states, sc.mode()),
		agent.WithToolServerMap(toolServer),
		agent.WithToolRouting(agent.ToolRoutingInBand, groups, nil, 0, true),
	)
	ag.Run(ctx, []llm.Message{{Role: llm.RoleUser, Content: sc.Message}}, func(agent.Event) {})

	return score(sc, ag.LastRoutingStats())
}

func score(sc Scenario, stats agent.RoutingStats) Result {
	exp := toSet(sc.Expected)
	got := toSet(stats.GroupsLoaded)

	hit := true
	if len(exp) == 0 {
		hit = len(got) == 0 // a "should skip" case is a hit only if nothing loaded
	} else {
		for e := range exp {
			if !got[e] {
				hit = false
				break
			}
		}
	}

	return Result{
		Name:      sc.Name,
		Expected:  sortedKeys(exp),
		Loaded:    append([]string(nil), stats.GroupsLoaded...),
		Hit:       hit,
		Exact:     len(exp) == len(got) && hit,
		Skipped:   stats.Skipped,
		LoadCalls: stats.LoadCalls,
	}
}

// Report aggregates scenario results into headline routing-quality metrics.
type Report struct {
	Results []Result
}

// RunSuite runs every scenario against the same provider and returns a Report.
func RunSuite(ctx context.Context, provider llm.Provider, scenarios []Scenario) Report {
	r := Report{Results: make([]Result, 0, len(scenarios))}
	for _, sc := range scenarios {
		r.Results = append(r.Results, RunScenario(ctx, provider, sc))
	}
	return r
}

// Top1Accuracy is the fraction of scenarios whose expected groups were all
// loaded (recall == 1.0), including correctly-skipped cases.
func (r Report) Top1Accuracy() float64 { return r.frac(func(x Result) bool { return x.Hit }) }

// ExactAccuracy is the fraction of scenarios where the loaded set exactly
// matched the expected set (no over- or under-loading).
func (r Report) ExactAccuracy() float64 { return r.frac(func(x Result) bool { return x.Exact }) }

// SkipRate is the fraction of scenarios that *expected a load* but where the
// model loaded nothing — the key risk signal for in-band routing.
func (r Report) SkipRate() float64 {
	var denom, skipped int
	for _, x := range r.Results {
		if len(x.Expected) == 0 {
			continue
		}
		denom++
		if x.Skipped {
			skipped++
		}
	}
	if denom == 0 {
		return 0
	}
	return float64(skipped) / float64(denom)
}

func (r Report) frac(pred func(Result) bool) float64 {
	if len(r.Results) == 0 {
		return 0
	}
	var n int
	for _, x := range r.Results {
		if pred(x) {
			n++
		}
	}
	return float64(n) / float64(len(r.Results))
}

// String renders a human-readable per-scenario table plus the headline metrics.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-22s %-7s %-22s %-22s\n", "SCENARIO", "RESULT", "EXPECTED", "LOADED")
	for _, x := range r.Results {
		verdict := "MISS"
		switch {
		case x.Exact:
			verdict = "EXACT"
		case x.Hit:
			verdict = "HIT"
		case x.Skipped:
			verdict = "SKIP"
		}
		fmt.Fprintf(&b, "%-22s %-7s %-22s %-22s\n",
			truncate(x.Name, 22), verdict,
			truncate(strings.Join(x.Expected, ","), 22),
			truncate(strings.Join(x.Loaded, ","), 22))
	}
	fmt.Fprintf(&b, "\nscenarios=%d  top1=%.0f%%  exact=%.0f%%  skip=%.0f%%\n",
		len(r.Results), r.Top1Accuracy()*100, r.ExactAccuracy()*100, r.SkipRate()*100)
	return b.String()
}

func toSet(xs []string) map[string]bool {
	s := make(map[string]bool, len(xs))
	for _, x := range xs {
		s[x] = true
	}
	return s
}

func sortedKeys(s map[string]bool) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
