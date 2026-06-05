package eval

import (
	"context"
	"os"
	"testing"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
)

// twoCapScenario is a fixed jira/confluence environment used by the
// deterministic harness tests. The message is irrelevant for the mock
// provider (which is scripted), but expected drives scoring.
func twoCapScenario(name string, expected ...string) Scenario {
	return Scenario{
		Name:    name,
		Message: "do the thing",
		Caps: []capability.Capability{
			cap("jira", "Jira", "Issues"),
			cap("confluence", "Confluence", "Docs"),
		},
		Tools: []ToolSpec{
			{Name: "jira_search", Capability: "jira", ReadOnly: true},
			{Name: "confluence_search", Capability: "confluence", ReadOnly: true},
		},
		Expected: expected,
	}
}

func loadThenAnswer(groups ...string) *llm.MockProvider {
	return &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockToolCallResponse("t1", "load_tools", map[string]any{"groups": groups}),
		llm.MockTextResponse("done"),
	}}
}

// A scripted load of exactly the expected group must score as Hit + Exact and
// must not register as a skip.
func TestRunScenarioExactHit(t *testing.T) {
	res := RunScenario(context.Background(), loadThenAnswer("jira"), twoCapScenario("jira", "jira"))
	if !res.Hit || !res.Exact {
		t.Fatalf("Hit=%v Exact=%v, want both true (loaded=%v)", res.Hit, res.Exact, res.Loaded)
	}
	if res.Skipped {
		t.Error("Skipped=true, want false (a group was loaded)")
	}
	if res.LoadCalls != 1 {
		t.Errorf("LoadCalls=%d, want 1", res.LoadCalls)
	}
}

// Loading the wrong group is a miss (not a hit, not exact).
func TestRunScenarioMisroute(t *testing.T) {
	res := RunScenario(context.Background(), loadThenAnswer("confluence"), twoCapScenario("jira", "jira"))
	if res.Hit || res.Exact {
		t.Fatalf("Hit=%v Exact=%v, want both false on a misroute (loaded=%v)", res.Hit, res.Exact, res.Loaded)
	}
}

// Loading the expected group plus an extra one is a Hit (recall 1.0) but not
// Exact (over-loaded).
func TestRunScenarioOverLoadIsHitNotExact(t *testing.T) {
	res := RunScenario(context.Background(), loadThenAnswer("jira", "confluence"), twoCapScenario("jira", "jira"))
	if !res.Hit {
		t.Errorf("Hit=false, want true (expected group was loaded)")
	}
	if res.Exact {
		t.Errorf("Exact=true, want false (an extra group was loaded)")
	}
}

// A multi-group expectation requires every expected group to be loaded.
func TestRunScenarioMultiGroup(t *testing.T) {
	sc := twoCapScenario("both", "jira", "confluence")
	full := RunScenario(context.Background(), loadThenAnswer("jira", "confluence"), sc)
	if !full.Hit || !full.Exact {
		t.Errorf("both loaded: Hit=%v Exact=%v, want both true", full.Hit, full.Exact)
	}
	partial := RunScenario(context.Background(), loadThenAnswer("jira"), sc)
	if partial.Hit {
		t.Errorf("only jira loaded: Hit=true, want false (confluence missing)")
	}
}

// A model that answers without loading anything must register as Skipped, and
// for a scenario that expected a load it is not a Hit. Two text responses cover
// the one-shot forced-first-step nudge.
func TestRunScenarioSkip(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockTextResponse("hello!"),
		llm.MockTextResponse("still hello!"),
	}}
	res := RunScenario(context.Background(), provider, twoCapScenario("jira", "jira"))
	if !res.Skipped {
		t.Errorf("Skipped=false, want true (model loaded nothing)")
	}
	if res.Hit {
		t.Errorf("Hit=true, want false (expected a load but got a skip)")
	}
}

// An empty Expected (e.g. a greeting) is a Hit precisely when nothing loads.
func TestRunScenarioCorrectSkipIsHit(t *testing.T) {
	provider := &llm.MockProvider{Responses: [][]llm.StreamEvent{
		llm.MockTextResponse("hi!"),
		llm.MockTextResponse("hi again!"),
	}}
	res := RunScenario(context.Background(), provider, twoCapScenario("greet"))
	if !res.Hit {
		t.Errorf("Hit=false, want true (correctly loaded nothing for empty expectation)")
	}
}

// RunSuite aggregates per-scenario results into the headline metrics.
func TestRunSuiteAggregateMetrics(t *testing.T) {
	report := Report{Results: []Result{
		{Name: "a", Expected: []string{"jira"}, Loaded: []string{"jira"}, Hit: true, Exact: true},
		{Name: "b", Expected: []string{"jira"}, Loaded: []string{"jira", "confluence"}, Hit: true},
		{Name: "c", Expected: []string{"jira"}, Skipped: true},
		{Name: "d", Expected: nil, Hit: true},
	}}
	if got := report.Top1Accuracy(); got != 0.75 {
		t.Errorf("Top1Accuracy=%.2f, want 0.75", got)
	}
	if got := report.ExactAccuracy(); got != 0.25 {
		t.Errorf("ExactAccuracy=%.2f, want 0.25", got)
	}
	// 1 skip out of 3 scenarios that expected a load (d expected nothing).
	if got := report.SkipRate(); got < 0.33 || got > 0.34 {
		t.Errorf("SkipRate=%.3f, want ~0.333", got)
	}
}

// DefaultScenarios must be self-consistent: every expected group ID must exist
// in the environment's capability set. This guards fixtures without needing a
// model.
func TestDefaultScenariosWellFormed(t *testing.T) {
	for _, sc := range DefaultScenarios() {
		ids := make(map[string]bool)
		for _, c := range sc.Caps {
			ids[c.ID] = true
		}
		for _, e := range sc.Expected {
			if !ids[e] {
				t.Errorf("scenario %q expects unknown group %q", sc.Name, e)
			}
		}
	}
}

// TestRealProviderEval runs the seed suite against a live LLM. It is opt-in:
// set CHAT_EVAL_PROVIDER, CHAT_EVAL_ENDPOINT, CHAT_EVAL_MODEL (and usually
// CHAT_EVAL_API_KEY) to enable it. Without those it skips, keeping CI
// hermetic. The run logs the full report and asserts only a permissive floor
// so it surfaces regressions without being flaky on a single bad sample.
func TestRealProviderEval(t *testing.T) {
	providerName := os.Getenv("CHAT_EVAL_PROVIDER")
	if providerName == "" {
		t.Skip("set CHAT_EVAL_PROVIDER/CHAT_EVAL_ENDPOINT/CHAT_EVAL_MODEL to run the live routing eval")
	}
	provider, err := llm.NewProvider(llm.ProviderConfig{
		Provider: providerName,
		Endpoint: os.Getenv("CHAT_EVAL_ENDPOINT"),
		APIKey:   os.Getenv("CHAT_EVAL_API_KEY"),
		Model:    os.Getenv("CHAT_EVAL_MODEL"),
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}

	report := RunSuite(context.Background(), provider, DefaultScenarios())
	t.Logf("\n%s", report.String())

	if got := report.Top1Accuracy(); got < 0.5 {
		t.Errorf("Top1Accuracy=%.2f below 0.50 floor — routing quality regressed", got)
	}
}
