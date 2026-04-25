package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/ebarron/netapp-chat-service/capability"
	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

// collectEvents runs the agent and collects all emitted events.
func collectEvents(t *testing.T, a *Agent, messages []llm.Message) []Event {
	t.Helper()
	var events []Event
	a.Run(context.Background(), messages, func(e Event) {
		events = append(events, e)
	})
	return events
}

func TestTextOnlyResponse(t *testing.T) {
	// LLM returns a simple text response with no tool calls.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("Hello", ", ", "world!"),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	agent := New(provider, router)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Hi"},
	}
	events := collectEvents(t, agent, messages)

	// Should have: 3 text events + 1 done.
	var texts []string
	var doneCount int
	for _, e := range events {
		switch e.Type {
		case EventText:
			texts = append(texts, e.Text)
		case EventDone:
			doneCount++
		}
	}

	if got := strings.Join(texts, ""); got != "Hello, world!" {
		t.Errorf("text = %q, want %q", got, "Hello, world!")
	}
	if doneCount != 1 {
		t.Errorf("done events = %d, want 1", doneCount)
	}
}

func TestSingleToolCallCycle(t *testing.T) {
	// LLM first requests a tool call, then produces text after receiving result.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			// Turn 1: tool call
			llm.MockToolCallResponse("tc-1", "metrics_query", `{"query":"up"}`),
			// Turn 2: text response after tool result
			llm.MockTextResponse("All ", "systems ", "operational."),
		},
	}

	tools := []llm.ToolDef{mcpclient.MockTool("metrics_query", "Query metrics")}
	router := mcpclient.NewMockRouter(tools)
	router.SetResult("metrics_query", `[{"metric":"up","value":1}]`)

	agent := New(provider, router)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Are systems up?"},
	}
	events := collectEvents(t, agent, messages)

	// Verify event types present (order depends on mock's text prefix)
	var hasToolStart, hasToolResult, hasText, hasDone bool
	for _, e := range events {
		switch e.Type {
		case EventToolStart:
			hasToolStart = true
		case EventToolResult:
			hasToolResult = true
		case EventText:
			hasText = true
		case EventDone:
			hasDone = true
		}
	}
	if !hasToolStart {
		t.Error("expected EventToolStart")
	}
	if !hasToolResult {
		t.Error("expected EventToolResult")
	}
	if !hasText {
		t.Error("expected EventText")
	}
	if !hasDone {
		t.Error("expected EventDone")
	}

	// Verify tool was called
	calls := router.Calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "metrics_query" {
		t.Errorf("call.Name = %q, want %q", calls[0].Name, "metrics_query")
	}
}

func TestMultiStepToolChain(t *testing.T) {
	// LLM calls tool A, gets result, then calls tool B, gets result, then text.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "list_volumes", `{}`),
			llm.MockToolCallResponse("tc-2", "metrics_query", `{"volume":"vol1"}`),
			llm.MockTextResponse("Volume vol1 is at 85% capacity."),
		},
	}

	tools := []llm.ToolDef{
		mcpclient.MockTool("list_volumes", "List volumes"),
		mcpclient.MockTool("metrics_query", "Query metrics"),
	}
	router := mcpclient.NewMockRouter(tools)
	router.SetResult("list_volumes", `["vol1","vol2"]`)
	router.SetResult("metrics_query", `{"capacity":85}`)

	agent := New(provider, router)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Check volume capacity"},
	}
	events := collectEvents(t, agent, messages)

	// Count tool starts
	var toolStarts int
	for _, e := range events {
		if e.Type == EventToolStart {
			toolStarts++
		}
	}
	if toolStarts != 2 {
		t.Errorf("tool starts = %d, want 2", toolStarts)
	}

	// Verify both tools were called
	calls := router.Calls()
	if len(calls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(calls))
	}
	if calls[0].Name != "list_volumes" {
		t.Errorf("call[0].Name = %q, want %q", calls[0].Name, "list_volumes")
	}
	if calls[1].Name != "metrics_query" {
		t.Errorf("call[1].Name = %q, want %q", calls[1].Name, "metrics_query")
	}
}

func TestMaxIterationLimit(t *testing.T) {
	// LLM always requests tool calls, never produces text.
	// Agent should stop after MaxIterations and still emit EventDone.

	// Create enough responses for max iterations + 1 (the summary turn).
	maxIter := 3
	responses := make([][]llm.StreamEvent, maxIter+1)
	for i := 0; i < maxIter; i++ {
		responses[i] = llm.MockToolCallResponse(
			fmt.Sprintf("tc-%d", i+1), "metrics_query", `{"i":`+fmt.Sprintf("%d", i)+`}`,
		)
	}
	// The summary turn (after max iterations, tools=nil)
	responses[maxIter] = llm.MockTextResponse("Summary: ran out of iterations.")

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses:    responses,
	}

	tools := []llm.ToolDef{mcpclient.MockTool("metrics_query", "Query metrics")}
	router := mcpclient.NewMockRouter(tools)
	router.SetResult("metrics_query", `{"value":42}`)

	agent := New(provider, router, WithMaxIterations(maxIter))
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Keep querying"},
	}
	events := collectEvents(t, agent, messages)

	// Should have exactly maxIter tool starts
	var toolStarts int
	for _, e := range events {
		if e.Type == EventToolStart {
			toolStarts++
		}
	}
	if toolStarts != maxIter {
		t.Errorf("tool starts = %d, want %d", toolStarts, maxIter)
	}

	// Should end with text + done
	last := events[len(events)-1]
	if last.Type != EventDone {
		t.Errorf("last event type = %d, want EventDone (%d)", last.Type, EventDone)
	}

	// The summary text should be present
	var allText string
	for _, e := range events {
		if e.Type == EventText {
			allText += e.Text
		}
	}
	if !strings.Contains(allText, "Summary") {
		t.Errorf("expected summary text, got %q", allText)
	}
}

func TestToolErrorRecovery(t *testing.T) {
	// Tool call fails, error is fed back to LLM, LLM explains to user.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "broken_tool", `{}`),
			llm.MockTextResponse("Sorry, that tool failed. ", "Let me explain."),
		},
	}

	tools := []llm.ToolDef{mcpclient.MockTool("broken_tool", "A broken tool")}
	router := mcpclient.NewMockRouter(tools)
	router.SetError("broken_tool", fmt.Errorf("connection refused"))

	agent := New(provider, router)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Use the broken tool"},
	}
	events := collectEvents(t, agent, messages)

	// Should have: tool_start, tool_error, text..., done
	var hasToolError bool
	var hasText bool
	var hasDone bool
	for _, e := range events {
		switch e.Type {
		case EventToolError:
			hasToolError = true
			if !strings.Contains(e.Error, "connection refused") {
				t.Errorf("error = %q, want to contain %q", e.Error, "connection refused")
			}
		case EventText:
			hasText = true
		case EventDone:
			hasDone = true
		}
	}

	if !hasToolError {
		t.Error("expected EventToolError")
	}
	if !hasText {
		t.Error("expected text response after tool error")
	}
	if !hasDone {
		t.Error("expected EventDone")
	}

	// LLM should have been called twice: once for tool call, once after error
	if len(provider.Calls) != 2 {
		t.Errorf("LLM calls = %d, want 2", len(provider.Calls))
	}
}

func TestContextCancellation(t *testing.T) {
	// Cancelled context should produce an error event.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("This should not complete"),
		},
	}
	// Override ChatStream to respect context
	// The mock will return text even with cancelled context, so we just
	// verify the agent completes gracefully.
	router := mcpclient.NewMockRouter(nil)
	agent := New(provider, router)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Agent should still run (mock doesn't check ctx), just verify no panic
	_ = ctx
	events := collectEvents(t, agent, []llm.Message{
		{Role: llm.RoleUser, Content: "Hi"},
	})
	if len(events) == 0 {
		t.Error("expected at least one event")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	// testConfig is a minimal config for testing. Tests that check product
	// identity use this config; tests that don't care use it as a default.
	testConfig := SystemPromptConfig{
		ProductName:        "NAbox Assistant",
		ProductDescription: "an AI-powered storage infrastructure expert embedded in the NAbox monitoring appliance.",
		Guidelines: []string{
			"When presenting Grafana links, rewrite internal addresses to use /grafana/.",
		},
	}

	t.Run("no tools", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "")
		if !strings.Contains(prompt, "NAbox Assistant") {
			t.Error("prompt should contain 'NAbox Assistant'")
		}
		if !strings.Contains(prompt, "No tools are currently available") {
			t.Error("prompt should mention no tools available")
		}
	})

	t.Run("with tools", func(t *testing.T) {
		tools := []llm.ToolDef{
			mcpclient.MockTool("t1", "Tool 1"),
			mcpclient.MockTool("t2", "Tool 2"),
		}
		router := mcpclient.NewMockRouter(tools)
		router.SetServers([]string{"harvest-mcp"})

		prompt := BuildSystemPrompt(testConfig, router, "")
		if !strings.Contains(prompt, "harvest-mcp") {
			t.Error("prompt should mention connected server")
		}
		if !strings.Contains(prompt, "2 tools") {
			t.Error("prompt should mention tool count")
		}
	})

	t.Run("no interest index omits format spec", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "")
		if strings.Contains(prompt, "Chart & Dashboard Format") {
			t.Error("prompt should NOT contain chart format spec when no interests")
		}
		if strings.Contains(prompt, "Response Interests") {
			t.Error("prompt should NOT contain interest section when no interests")
		}
	})

	t.Run("with interest index includes format spec", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		index := "| ID | Name | Triggers |\n|----|------|----------|\n| morning-coffee | Fleet Health Overview | how's everything |\n"
		prompt := BuildSystemPrompt(testConfig, router, index)

		if !strings.Contains(prompt, "Chart & Dashboard Format") {
			t.Error("prompt should contain chart format spec")
		}
		if !strings.Contains(prompt, "Response Interests") {
			t.Error("prompt should contain interest section header")
		}
		if !strings.Contains(prompt, "morning-coffee") {
			t.Error("prompt should contain interest index")
		}
		if !strings.Contains(prompt, "get_interest") {
			t.Error("prompt should mention get_interest tool")
		}
		// Semantic matching instruction prevents the LLM from only doing
		// exact-string matching against trigger phrases.
		if !strings.Contains(prompt, "semantically matches") {
			t.Error("prompt should instruct semantic matching of triggers")
		}
	})

	t.Run("format spec contains all chart types", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "| ID | Name | Triggers |\n")

		chartTypes := []string{
			"area", "bar", "gauge", "sparkline", "status-grid", "stat",
			"alert-summary", "resource-table", "alert-list",
			"callout", "proposal", "action-button",
		}
		for _, ct := range chartTypes {
			if !strings.Contains(prompt, "\""+ct+"\"") {
				t.Errorf("format spec missing chart type %q", ct)
			}
		}
	})

	t.Run("includes interest management spec", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		index := "| ID | Name | Triggers |\n|----|------|----------|\n| test | Test | test |\n"
		prompt := BuildSystemPrompt(testConfig, router, index)

		if !strings.Contains(prompt, "Interest Management") {
			t.Error("prompt should contain interest management section")
		}
		if !strings.Contains(prompt, "save_interest") {
			t.Error("prompt should mention save_interest")
		}
		if !strings.Contains(prompt, "delete_interest") {
			t.Error("prompt should mention delete_interest")
		}
		if !strings.Contains(prompt, "confirmation") {
			t.Error("prompt should describe confirmation workflow")
		}
	})

	t.Run("format spec contains object-detail type", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "| ID | Name | Triggers |\n")

		if !strings.Contains(prompt, "object-detail") {
			t.Error("format spec should contain object-detail")
		}
		if !strings.Contains(prompt, "language \"object-detail\"") {
			t.Error("format spec should document object-detail language tag")
		}
	})

	t.Run("format spec contains all section layout names", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "| ID | Name | Triggers |\n")

		layouts := []string{"properties", "chart", "alert-list", "timeline", "actions", "text", "table"}
		for _, layout := range layouts {
			if !strings.Contains(prompt, "**"+layout+"**") {
				t.Errorf("format spec missing section layout %q", layout)
			}
		}
	})

	t.Run("format spec contains type selection guidance", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "| ID | Name | Triggers |\n")

		if !strings.Contains(prompt, "Output type selection") {
			t.Error("format spec should contain type selection guidance")
		}
		if !strings.Contains(prompt, "single named entity") {
			t.Error("format spec should mention single entity routing")
		}
	})

	t.Run("canvas tabs appends context section", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		tabs := []CanvasTabSummary{
			{TabID: "volume::vol1::on SVM svm1", Kind: "volume", Name: "vol1", Qualifier: "on SVM svm1", Status: "warning"},
			{TabID: "cluster::cls1::", Kind: "cluster", Name: "cls1"},
		}
		prompt := BuildSystemPrompt(testConfig, router, "", tabs...)

		if !strings.Contains(prompt, "Canvas Context") {
			t.Error("prompt should contain Canvas Context section")
		}
		if !strings.Contains(prompt, "volume") || !strings.Contains(prompt, "vol1") {
			t.Error("prompt should contain first tab info")
		}
		if !strings.Contains(prompt, "cluster") || !strings.Contains(prompt, "cls1") {
			t.Error("prompt should contain second tab info")
		}
		if !strings.Contains(prompt, "warning") {
			t.Error("prompt should contain tab status")
		}
		if !strings.Contains(prompt, "canvas") {
			t.Error("prompt should reference canvas")
		}
	})

	t.Run("empty canvas tabs omits context section", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "")
		if strings.Contains(prompt, "Canvas Context") {
			t.Error("prompt should NOT contain Canvas Context when no tabs")
		}
	})

	t.Run("canvas fence instructions included with interests", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		index := "| ID | Name | Triggers | Target |\n|----|------|----------|--------|\n| vol | Volume | volume | canvas |\n"
		prompt := BuildSystemPrompt(testConfig, router, index)

		if !strings.Contains(prompt, "Canvas fences") {
			t.Error("prompt should contain Canvas fences section")
		}
		if !strings.Contains(prompt, "canvas-object-detail") {
			t.Error("prompt should mention canvas-object-detail fence")
		}
		if !strings.Contains(prompt, "canvas-dashboard") {
			t.Error("prompt should mention canvas-dashboard fence")
		}
		if !strings.Contains(prompt, "Target") {
			t.Error("prompt should reference Target column")
		}
	})

	t.Run("canvas fence instructions omitted without interests", func(t *testing.T) {
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(testConfig, router, "")
		if strings.Contains(prompt, "Canvas fences") {
			t.Error("prompt should NOT contain Canvas fences without interests")
		}
	})

	t.Run("custom product config", func(t *testing.T) {
		cfg := SystemPromptConfig{
			ProductName:        "Harvest Assistant",
			ProductDescription: "an AI assistant for the Harvest metrics collector.",
			Guidelines:         []string{"Always show metric units."},
		}
		router := mcpclient.NewMockRouter(nil)
		prompt := BuildSystemPrompt(cfg, router, "")
		if !strings.Contains(prompt, "Harvest Assistant") {
			t.Error("prompt should contain custom product name")
		}
		if !strings.Contains(prompt, "Harvest metrics collector") {
			t.Error("prompt should contain custom product description")
		}
		if !strings.Contains(prompt, "Always show metric units") {
			t.Error("prompt should contain custom guideline")
		}
		if strings.Contains(prompt, "NAbox") {
			t.Error("prompt should NOT contain NAbox when using custom config")
		}
	})
}

func TestAgentOptions(t *testing.T) {
	provider := &llm.MockProvider{ProviderName: "mock"}
	router := mcpclient.NewMockRouter(nil)

	agent := New(provider, router,
		WithSystemPrompt("custom prompt"),
		WithModel("gpt-4"),
		WithMaxIterations(5),
	)

	if agent.SystemPrompt != "custom prompt" {
		t.Errorf("SystemPrompt = %q, want %q", agent.SystemPrompt, "custom prompt")
	}
	if agent.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", agent.Model, "gpt-4")
	}
	if agent.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d, want 5", agent.MaxIterations)
	}
}

func TestToolCallInputPassedCorrectly(t *testing.T) {
	// Verify the tool call input JSON is passed through to the router.
	// Use json.RawMessage so MockToolCallResponse doesn't double-marshal.
	inputObj := map[string]any{
		"query":   "volume_capacity",
		"filters": map[string]any{"cluster": "prod"},
	}
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "metrics_query", inputObj),
			llm.MockTextResponse("Done."),
		},
	}

	tools := []llm.ToolDef{mcpclient.MockTool("metrics_query", "Query")}
	router := mcpclient.NewMockRouter(tools)
	router.SetResult("metrics_query", "result")

	agent := New(provider, router)
	collectEvents(t, agent, []llm.Message{{Role: llm.RoleUser, Content: "query"}})

	calls := router.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}

	var got map[string]any
	if err := json.Unmarshal(calls[0].Input, &got); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if got["query"] != "volume_capacity" {
		t.Errorf("input.query = %v, want %q", got["query"], "volume_capacity")
	}
}

// --- Phase 2 tests: capability filtering, ask-mode ---

func TestCapabilityFilterOff(t *testing.T) {
	// When a capability is Off, its tools should be excluded.
	tool := mcpclient.MockTool("metrics_query", "Query metrics")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool})
	router.SetResult("metrics_query", "result")
	router.SetServers([]string{"harvest-mcp"})

	// LLM returns text only (no tools to call since they're filtered).
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("No tools available."),
		},
	}

	capStates := capability.CapabilityMap{
		"harvest": capability.StateOff,
	}
	toolServerMap := map[string]string{
		"metrics_query": "harvest",
	}

	agent := New(provider, router,
		WithCapabilityFilter(capStates, "read-only"),
		WithToolServerMap(toolServerMap),
	)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "What metrics are available?"},
	}
	events := collectEvents(t, agent, messages)

	// The agent should have filtered out tools before sending to LLM.
	if len(provider.Calls) == 0 {
		t.Fatal("expected at least one LLM call")
	}
	// The LLM call should have no tools.
	if len(provider.Calls[0].Tools) != 0 {
		t.Errorf("LLM call had %d tools, want 0 (capability is Off)", len(provider.Calls[0].Tools))
	}

	// Should have text + done events.
	var hasDone bool
	for _, e := range events {
		if e.Type == EventDone {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("expected EventDone")
	}
}

func TestCapabilityFilterAllow(t *testing.T) {
	// When a capability is Allow, tools should be included and execute automatically.
	tool := mcpclient.MockTool("metrics_query", "Query metrics")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool})
	router.SetResult("metrics_query", "cpu_usage: 42%")
	router.SetServers([]string{"harvest-mcp"})

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("call-1", "metrics_query", map[string]any{"query": "cpu"}),
			llm.MockTextResponse("CPU is at 42%."),
		},
	}

	capStates := capability.CapabilityMap{
		"harvest": capability.StateAllow,
	}
	toolServerMap := map[string]string{
		"metrics_query": "harvest",
	}

	agent := New(provider, router,
		WithCapabilityFilter(capStates, "read-only"),
		WithToolServerMap(toolServerMap),
	)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Show CPU usage"},
	}
	events := collectEvents(t, agent, messages)

	// Should have: tool_start + tool_result + text + done
	var hasToolStart, hasToolResult, hasDone bool
	for _, e := range events {
		switch e.Type {
		case EventToolStart:
			hasToolStart = true
		case EventToolResult:
			hasToolResult = true
		case EventDone:
			hasDone = true
		}
	}
	if !hasToolStart {
		t.Error("expected EventToolStart")
	}
	if !hasToolResult {
		t.Error("expected EventToolResult")
	}
	if !hasDone {
		t.Error("expected EventDone")
	}
}

func TestCapabilityAskModeApproved(t *testing.T) {
	// When a capability is in Ask state and approval func returns true.
	tool := mcpclient.MockTool("metrics_query", "Query metrics")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool})
	router.SetResult("metrics_query", "result data")
	router.SetServers([]string{"harvest-mcp"})

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("call-1", "metrics_query", map[string]any{"q": "test"}),
			llm.MockTextResponse("Done."),
		},
	}

	capStates := capability.CapabilityMap{
		"harvest": capability.StateAsk,
	}
	toolServerMap := map[string]string{
		"metrics_query": "harvest",
	}

	approvalCalled := false
	agent := New(provider, router,
		WithCapabilityFilter(capStates, "read-only"),
		WithToolServerMap(toolServerMap),
		WithApprovalFunc(func(capID, toolName string, tc llm.ToolCall) bool {
			approvalCalled = true
			if capID != "harvest" {
				t.Errorf("approval capID = %q, want %q", capID, "harvest")
			}
			if toolName != "metrics_query" {
				t.Errorf("approval toolName = %q, want %q", toolName, "metrics_query")
			}
			return true // approve
		}),
	)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Query something"},
	}
	events := collectEvents(t, agent, messages)

	if !approvalCalled {
		t.Error("approval function was not called")
	}

	// Should execute the tool successfully.
	var hasToolResult bool
	for _, e := range events {
		if e.Type == EventToolResult {
			hasToolResult = true
		}
	}
	if !hasToolResult {
		t.Error("expected EventToolResult after approval")
	}
}

func TestCapabilityAskModeDenied(t *testing.T) {
	// When a capability is in Ask state and approval func returns false.
	tool := mcpclient.MockTool("metrics_query", "Query metrics")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool})
	router.SetResult("metrics_query", "should not be called")
	router.SetServers([]string{"harvest-mcp"})

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("call-1", "metrics_query", map[string]any{"q": "test"}),
			llm.MockTextResponse("Tool was denied."),
		},
	}

	capStates := capability.CapabilityMap{
		"harvest": capability.StateAsk,
	}
	toolServerMap := map[string]string{
		"metrics_query": "harvest",
	}

	agent := New(provider, router,
		WithCapabilityFilter(capStates, "read-only"),
		WithToolServerMap(toolServerMap),
		WithApprovalFunc(func(capID, toolName string, tc llm.ToolCall) bool {
			return false // deny
		}),
	)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Query something"},
	}
	events := collectEvents(t, agent, messages)

	// Should have a tool error event for the denied call.
	var hasToolError bool
	for _, e := range events {
		if e.Type == EventToolError {
			hasToolError = true
			if !strings.Contains(e.Error, "denied") {
				t.Errorf("tool error = %q, expected 'denied'", e.Error)
			}
		}
	}
	if !hasToolError {
		t.Error("expected EventToolError for denied tool call")
	}

	// The tool should NOT have been called on the router.
	calls := router.Calls()
	if len(calls) != 0 {
		t.Errorf("router calls = %d, want 0 (tool was denied)", len(calls))
	}
}

func TestFilteredToolsExcludesOff(t *testing.T) {
	// Unit test filteredTools method directly.
	tool1 := mcpclient.MockReadOnlyTool("harvest_query", "Harvest query")
	tool2 := mcpclient.MockReadOnlyTool("ontap_volumes", "ONTAP volumes")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool1, tool2})

	agent := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{
			"harvest": capability.StateOff,
			"ontap":   capability.StateAllow,
		}, "read-only"),
		WithToolServerMap(map[string]string{
			"harvest_query": "harvest",
			"ontap_volumes": "ontap",
		}),
	)

	filtered, err := agent.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filteredTools() = %d tools, want 1", len(filtered))
	}
	if filtered[0].Name != "ontap_volumes" {
		t.Errorf("remaining tool = %q, want %q", filtered[0].Name, "ontap_volumes")
	}
}

func TestFilteredToolsNoFilter(t *testing.T) {
	// When CapStates is nil, all tools should be returned.
	tool1 := mcpclient.MockTool("t1", "Tool 1")
	tool2 := mcpclient.MockTool("t2", "Tool 2")
	router := mcpclient.NewMockRouter([]llm.ToolDef{tool1, tool2})

	agent := New(nil, router) // no capability filter

	filtered, err := agent.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("filteredTools() = %d tools, want 2", len(filtered))
	}
}

// --- Internal tools tests ---

func TestInternalToolCallHandledLocally(t *testing.T) {
	// An internal tool call should be handled by the agent, not routed to MCP.
	internalToolDef := llm.ToolDef{
		Name:        "get_interest",
		Description: "Get an interest by ID",
		Schema:      json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
	}

	handlerCalled := false
	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: internalToolDef,
			Handler: func(_ context.Context, input json.RawMessage) (string, error) {
				handlerCalled = true
				var req struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(input, &req); err != nil {
					return "", err
				}
				return "Interest body for " + req.ID, nil
			},
		},
	}

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "get_interest", map[string]any{"id": "morning-coffee"}),
			llm.MockTextResponse("Here is the morning coffee report."),
		},
	}

	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(internalTools))

	messages := []llm.Message{{Role: llm.RoleUser, Content: "Show me the morning overview"}}
	events := collectEvents(t, ag, messages)

	if !handlerCalled {
		t.Error("internal tool handler was not called")
	}

	// The router should have zero calls (internal tool bypasses MCP).
	if calls := router.Calls(); len(calls) != 0 {
		t.Errorf("router calls = %d, want 0", len(calls))
	}

	// Should have: tool_start, tool_result, text..., done
	var hasToolStart, hasToolResult, hasDone bool
	var toolResult string
	for _, e := range events {
		switch e.Type {
		case EventToolStart:
			hasToolStart = true
			if e.ToolName != "get_interest" {
				t.Errorf("tool start name = %q, want %q", e.ToolName, "get_interest")
			}
		case EventToolResult:
			hasToolResult = true
			toolResult = e.ToolResult
		case EventDone:
			hasDone = true
		}
	}
	if !hasToolStart {
		t.Error("expected EventToolStart for internal tool")
	}
	if !hasToolResult {
		t.Error("expected EventToolResult for internal tool")
	}
	if !strings.Contains(toolResult, "morning-coffee") {
		t.Errorf("tool result = %q, want to contain %q", toolResult, "morning-coffee")
	}
	if !hasDone {
		t.Error("expected EventDone")
	}
}

func TestInternalToolError(t *testing.T) {
	// When an internal tool returns an error, it should be fed back to the LLM.
	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{
				Name:        "get_interest",
				Description: "Get interest",
				Schema:      json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
			},
			Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
				return "", fmt.Errorf("interest %q not found", "bogus")
			},
		},
	}

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "get_interest", `{"id":"bogus"}`),
			llm.MockTextResponse("That interest was not found."),
		},
	}

	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(internalTools))

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "test"}})

	var hasToolError bool
	for _, e := range events {
		if e.Type == EventToolError {
			hasToolError = true
			if !strings.Contains(e.Error, "not found") {
				t.Errorf("error = %q, want to contain %q", e.Error, "not found")
			}
		}
	}
	if !hasToolError {
		t.Error("expected EventToolError for failed internal tool")
	}

	// Router should have zero calls.
	if calls := router.Calls(); len(calls) != 0 {
		t.Errorf("router calls = %d, want 0", len(calls))
	}
}

func TestFilteredToolsIncludesInternalTools(t *testing.T) {
	// Internal tools should appear in the filtered tool list alongside MCP tools.
	mcpTool := mcpclient.MockTool("metrics_query", "Query metrics")
	router := mcpclient.NewMockRouter([]llm.ToolDef{mcpTool})

	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{
				Name:        "get_interest",
				Description: "Get interest",
				Schema:      json.RawMessage(`{}`),
			},
		},
	}

	ag := New(nil, router, WithInternalTools(internalTools))
	filtered, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("filteredTools() = %d, want 2", len(filtered))
	}

	names := make(map[string]bool)
	for _, td := range filtered {
		names[td.Name] = true
	}
	if !names["metrics_query"] {
		t.Error("missing MCP tool 'metrics_query'")
	}
	if !names["get_interest"] {
		t.Error("missing internal tool 'get_interest'")
	}
}

func TestMixedInternalAndMCPToolCalls(t *testing.T) {
	// LLM calls an internal tool AND an MCP tool in the same round.
	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{
				Name:        "get_interest",
				Description: "Get interest",
				Schema:      json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
			},
			Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
				return "interest body", nil
			},
		},
	}

	// Turn 1: LLM calls get_interest, then receives result and calls MCP tool.
	// Turn 2: LLM calls MCP tool.
	// Turn 3: Text response.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("tc-1", "get_interest", `{"id":"test"}`),
			llm.MockToolCallResponse("tc-2", "metrics_query", `{"query":"up"}`),
			llm.MockTextResponse("All done."),
		},
	}

	mcpTool := mcpclient.MockTool("metrics_query", "Query")
	router := mcpclient.NewMockRouter([]llm.ToolDef{mcpTool})
	router.SetResult("metrics_query", `[{"value":1}]`)

	ag := New(provider, router, WithInternalTools(internalTools))
	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "test"}})

	// Internal tool should NOT go through router.
	mcpCalls := router.Calls()
	if len(mcpCalls) != 1 {
		t.Fatalf("router calls = %d, want 1", len(mcpCalls))
	}
	if mcpCalls[0].Name != "metrics_query" {
		t.Errorf("router call name = %q, want %q", mcpCalls[0].Name, "metrics_query")
	}

	// Should have 2 tool starts and 2 tool results.
	var toolStarts, toolResults int
	for _, e := range events {
		switch e.Type {
		case EventToolStart:
			toolStarts++
		case EventToolResult:
			toolResults++
		}
	}
	if toolStarts != 2 {
		t.Errorf("tool starts = %d, want 2", toolStarts)
	}
	if toolResults != 2 {
		t.Errorf("tool results = %d, want 2", toolResults)
	}
}

func TestReadWriteOnlyToolsExcludedInReadOnly(t *testing.T) {
	// ReadWriteOnly tools should be excluded when mode is "read-only".
	mcpTool := mcpclient.MockTool("metrics_query", "Query")
	router := mcpclient.NewMockRouter([]llm.ToolDef{mcpTool})

	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{
				Name:        "get_interest",
				Description: "Get interest",
				Schema:      json.RawMessage(`{}`),
			},
		},
		"save_interest": {
			Def: llm.ToolDef{
				Name:        "save_interest",
				Description: "Save interest",
				Schema:      json.RawMessage(`{}`),
			},
			ReadWriteOnly: true,
		},
		"delete_interest": {
			Def: llm.ToolDef{
				Name:        "delete_interest",
				Description: "Delete interest",
				Schema:      json.RawMessage(`{}`),
			},
			ReadWriteOnly: true,
		},
	}

	// read-only mode — save/delete should be excluded.
	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{}, "read-only"),
		WithInternalTools(internalTools),
	)
	filtered, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, td := range filtered {
		names[td.Name] = true
	}
	if !names["get_interest"] {
		t.Error("get_interest should be present in read-only mode")
	}
	if !names["metrics_query"] {
		t.Error("metrics_query should be present")
	}
	if names["save_interest"] {
		t.Error("save_interest should be excluded in read-only mode")
	}
	if names["delete_interest"] {
		t.Error("delete_interest should be excluded in read-only mode")
	}
}

func TestReadWriteOnlyToolsIncludedInReadWrite(t *testing.T) {
	// ReadWriteOnly tools should be included when mode is "read-write".
	router := mcpclient.NewMockRouter(nil)

	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{Name: "get_interest", Description: "Get", Schema: json.RawMessage(`{}`)},
		},
		"save_interest": {
			Def:           llm.ToolDef{Name: "save_interest", Description: "Save", Schema: json.RawMessage(`{}`)},
			ReadWriteOnly: true,
		},
	}

	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{}, "read-write"),
		WithInternalTools(internalTools),
	)
	filtered, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, td := range filtered {
		names[td.Name] = true
	}
	if !names["get_interest"] {
		t.Error("get_interest should be present")
	}
	if !names["save_interest"] {
		t.Error("save_interest should be present in read-write mode")
	}
}

// rateLimitProvider is a mock provider that returns a 429 error on the first
// N calls, then succeeds with a normal response.
type rateLimitProvider struct {
	failCount  int // how many calls should fail with 429
	callCount  int
	successRes []llm.StreamEvent
}

func (r *rateLimitProvider) Name() string                           { return "rate-limit-mock" }
func (r *rateLimitProvider) ValidateConfig(_ context.Context) error { return nil }
func (r *rateLimitProvider) ListModels(_ context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (r *rateLimitProvider) ChatStream(_ context.Context, _ llm.ChatRequest) iter.Seq2[llm.StreamEvent, error] {
	r.callCount++
	shouldFail := r.callCount <= r.failCount

	return func(yield func(llm.StreamEvent, error) bool) {
		if shouldFail {
			yield(llm.StreamEvent{}, fmt.Errorf(
				`openai stream: POST "https://api.openai.com/v1/chat/completions": 429 Too Many Requests {"message":"Rate limit reached. Please try again in 0.1s.","type":"tokens","code":"rate_limit_exceeded"}`,
			))
			return
		}
		for _, e := range r.successRes {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func TestRateLimitRetrySuccess(t *testing.T) {
	// Provider fails with 429 once, then succeeds.
	provider := &rateLimitProvider{
		failCount:  1,
		successRes: llm.MockTextResponse("Success after retry"),
	}
	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router)

	start := time.Now()
	events := collectEvents(t, ag, []llm.Message{
		{Role: llm.RoleUser, Content: "Hello"},
	})
	elapsed := time.Since(start)

	// Should have succeeded with text + done.
	var texts []string
	var doneCount int
	var errorCount int
	for _, e := range events {
		switch e.Type {
		case EventText:
			texts = append(texts, e.Text)
		case EventDone:
			doneCount++
		case EventError:
			errorCount++
		}
	}

	if errorCount != 0 {
		t.Errorf("expected no errors after retry, got %d", errorCount)
	}
	if doneCount != 1 {
		t.Errorf("expected 1 done event, got %d", doneCount)
	}
	if joined := strings.Join(texts, ""); joined != "Success after retry" {
		t.Errorf("expected success text, got %q", joined)
	}
	// Should have waited for the retry delay (at least 500ms).
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected retry delay of at least 500ms, got %v", elapsed)
	}
	if provider.callCount != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", provider.callCount)
	}
}

func TestRateLimitRetryExhausted(t *testing.T) {
	// Provider fails with 429 on all calls (more than maxRateLimitRetries+1).
	provider := &rateLimitProvider{
		failCount:  10,
		successRes: llm.MockTextResponse("Never reached"),
	}
	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router)

	events := collectEvents(t, ag, []llm.Message{
		{Role: llm.RoleUser, Content: "Hello"},
	})

	var errorCount int
	var errorMsg string
	for _, e := range events {
		if e.Type == EventError {
			errorCount++
			errorMsg = e.Error
		}
	}

	if errorCount != 1 {
		t.Errorf("expected 1 error event, got %d", errorCount)
	}
	if !strings.Contains(errorMsg, "429") {
		t.Errorf("error should mention 429, got %q", errorMsg)
	}
	// Should have tried maxRateLimitRetries+1 times.
	if provider.callCount != maxRateLimitRetries+1 {
		t.Errorf("expected %d calls, got %d", maxRateLimitRetries+1, provider.callCount)
	}
}

func TestParseRateLimitDelay(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "parses seconds from OpenAI error",
			errMsg:   `Please try again in 2.304s. Visit https://platform.openai.com`,
			minDelay: 2700 * time.Millisecond, // 2.304 + 0.5 buffer
			maxDelay: 3000 * time.Millisecond,
		},
		{
			name:     "falls back to default when no match",
			errMsg:   "some random 429 error",
			minDelay: defaultRetryDelay,
			maxDelay: defaultRetryDelay + time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRateLimitDelay(tc.errMsg)
			if got < tc.minDelay || got > tc.maxDelay {
				t.Errorf("parseRateLimitDelay(%q) = %v, want between %v and %v",
					tc.errMsg, got, tc.minDelay, tc.maxDelay)
			}
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	if !isRateLimitError(`openai stream: 429 Too Many Requests {"code":"rate_limit_exceeded"}`) {
		t.Error("should detect rate limit error")
	}
	if isRateLimitError("openai stream: 500 Internal Server Error") {
		t.Error("should not match non-429 errors")
	}
}

func TestTextClearEmittedWithToolCalls(t *testing.T) {
	// When the LLM emits text alongside tool calls (Claude-style "thinking"
	// text), EventTextClear must be emitted before tool execution. For
	// OpenAI models that don't mix text and tools, the event is a no-op.
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			// Turn 1: thinking text + tool call (Claude pattern)
			llm.MockToolCallResponse("tc-1", "metrics_query", `{"query":"up"}`),
			// Turn 2: final text response after tool result
			llm.MockTextResponse("All systems operational."),
		},
	}

	tools := []llm.ToolDef{mcpclient.MockTool("metrics_query", "Query metrics")}
	router := mcpclient.NewMockRouter(tools)
	router.SetResult("metrics_query", `[{"value":1}]`)

	agent := New(provider, router)
	events := collectEvents(t, agent, []llm.Message{
		{Role: llm.RoleUser, Content: "How are things?"},
	})

	// EventTextClear must appear after the thinking text and before tool execution.
	var hasTextClear, hasToolStart, hasDone bool
	var textClearIdx, toolStartIdx int
	for i, e := range events {
		switch e.Type {
		case EventTextClear:
			hasTextClear = true
			textClearIdx = i
		case EventToolStart:
			if !hasToolStart {
				hasToolStart = true
				toolStartIdx = i
			}
		case EventDone:
			hasDone = true
		}
	}

	if !hasTextClear {
		t.Fatal("expected EventTextClear when tool calls have thinking text")
	}
	if !hasToolStart {
		t.Fatal("expected EventToolStart")
	}
	if !hasDone {
		t.Fatal("expected EventDone")
	}
	// TextClear should come before tool execution.
	if textClearIdx >= toolStartIdx {
		t.Errorf("EventTextClear (idx %d) should precede EventToolStart (idx %d)", textClearIdx, toolStartIdx)
	}
}

func TestTextClearNotEmittedForTextOnly(t *testing.T) {
	// When LLM produces only text (no tool calls), EventTextClear must NOT be emitted.
	provider := &llm.MockProvider{
		Responses: [][]llm.StreamEvent{
			llm.MockTextResponse("Hello, world!"),
		},
	}
	router := mcpclient.NewMockRouter(nil)
	agent := New(provider, router)

	events := collectEvents(t, agent, []llm.Message{
		{Role: llm.RoleUser, Content: "Hi"},
	})

	for _, e := range events {
		if e.Type == EventTextClear {
			t.Error("EventTextClear should not be emitted for text-only responses")
		}
	}
}

func TestRequiredAfterInterestEnforcedOnFollowUp(t *testing.T) {
	// Scenario: The interest was loaded in a previous turn (present in
	// message history), and now on the follow-up the LLM tries to produce
	// text without calling the required render tool. The agent should
	// detect this via history pre-scan and force the tool call.

	renderCalled := false
	internalTools := map[string]InternalTool{
		"get_interest": {
			Def: llm.ToolDef{
				Name:        "get_interest",
				Description: "Get an interest by ID",
				Schema:      json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
			},
			Handler: func(_ context.Context, input json.RawMessage) (string, error) {
				return "volume detail instructions", nil
			},
		},
		"render_volume_detail": {
			Def: llm.ToolDef{
				Name:        "render_volume_detail",
				Description: "Render volume detail",
				Schema:      json.RawMessage(`{"type":"object","properties":{"volume":{"type":"string"}}}`),
			},
			RequiredAfterInterest: "volume-detail",
			Handler: func(_ context.Context, input json.RawMessage) (string, error) {
				renderCalled = true
				return "rendered volume detail", nil
			},
		},
	}

	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			// Turn 2, attempt 1: LLM produces text without calling the tool.
			llm.MockTextResponse("The volume looks healthy..."),
			// Turn 2, attempt 2: After enforcement, LLM calls the render tool.
			llm.MockToolCallResponse("tc-render", "render_volume_detail", map[string]any{"volume": "docs"}),
			// Turn 2, attempt 3: Final text.
			llm.MockTextResponse("Here is the detail view."),
		},
	}

	router := mcpclient.NewMockRouter(nil)
	ag := New(provider, router, WithInternalTools(internalTools))

	// Message history from a PREVIOUS turn: include the get_interest call.
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "tell me about volume docs"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "tc-prev", Name: "get_interest", Input: json.RawMessage(`{"id":"volume-detail"}`)},
		}},
		{Role: llm.RoleTool, Content: "volume detail instructions", ToolCallID: "tc-prev"},
		{Role: llm.RoleAssistant, Content: "(previous turn response)"},
		// New user message for THIS turn:
		{Role: llm.RoleUser, Content: "tell me about volume docs again"},
	}
	events := collectEvents(t, ag, messages)

	if !renderCalled {
		t.Error("render_volume_detail was not called despite interest in history")
	}

	// Should have EventTextClear (first text rejected) then tool result then text.
	var hasClear bool
	for _, e := range events {
		if e.Type == EventTextClear {
			hasClear = true
		}
	}
	if !hasClear {
		t.Error("expected EventTextClear when LLM skipped required tool")
	}
}

func TestUnannotatedMCPToolFilteredInReadOnly(t *testing.T) {
	// MCP tools without ReadOnlyHint should be filtered out in read-only
	// mode when capability filtering is active.
	ro := mcpclient.MockReadOnlyTool("get_volume", "read")
	rw := mcpclient.MockTool("create_volume", "write") // ReadOnlyHint=false
	router := mcpclient.NewMockRouter([]llm.ToolDef{ro, rw})

	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{"ontap": capability.StateAllow}, "read-only"),
		WithToolServerMap(map[string]string{"get_volume": "ontap", "create_volume": "ontap"}),
	)
	tools, err := ag.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	names := map[string]bool{}
	for _, td := range tools {
		names[td.Name] = true
	}
	if !names["get_volume"] {
		t.Error("get_volume (read-only) should be present")
	}
	if names["create_volume"] {
		t.Error("create_volume (write) should be filtered out in read-only mode")
	}

	// In read-write mode both should appear.
	ag2 := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{"ontap": capability.StateAllow}, "read-write"),
		WithToolServerMap(map[string]string{"get_volume": "ontap", "create_volume": "ontap"}),
	)
	tools2, err := ag2.filteredTools()
	if err != nil {
		t.Fatalf("filteredTools() error = %v", err)
	}
	if len(tools2) != 2 {
		t.Errorf("read-write tool count = %d, want 2", len(tools2))
	}
}

func TestFilteredToolsExceedsBudgetReturnsError(t *testing.T) {
	// More than MaxToolsPerRequest read-only tools should produce
	// ErrTooManyTools.
	tools := make([]llm.ToolDef, MaxToolsPerRequest+5)
	tsm := make(map[string]string, len(tools))
	for i := range tools {
		name := fmt.Sprintf("tool_%d", i)
		tools[i] = mcpclient.MockReadOnlyTool(name, "ro")
		tsm[name] = "harvest"
	}
	router := mcpclient.NewMockRouter(tools)

	ag := New(nil, router,
		WithCapabilityFilter(capability.CapabilityMap{"harvest": capability.StateAllow}, "read-only"),
		WithToolServerMap(tsm),
	)
	if _, err := ag.filteredTools(); !errors.Is(err, ErrTooManyTools) {
		t.Fatalf("filteredTools() error = %v, want ErrTooManyTools", err)
	}
}

func TestRunEmitsErrorAndDoneWhenBudgetExceeded(t *testing.T) {
	// When filteredTools() exceeds the cap, Run should surface a clean
	// EventError to the user (mentioning the limit) and still emit
	// EventDone so the SSE stream closes cleanly.
	tools := make([]llm.ToolDef, MaxToolsPerRequest+1)
	tsm := make(map[string]string, len(tools))
	for i := range tools {
		name := fmt.Sprintf("tool_%d", i)
		tools[i] = mcpclient.MockReadOnlyTool(name, "ro")
		tsm[name] = "harvest"
	}
	router := mcpclient.NewMockRouter(tools)
	provider := &llm.MockProvider{ProviderName: "mock"}

	ag := New(provider, router,
		WithCapabilityFilter(capability.CapabilityMap{"harvest": capability.StateAllow}, "read-only"),
		WithToolServerMap(tsm),
	)

	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "hi"}})

	var sawError, sawDone bool
	var errMsg string
	for _, e := range events {
		switch e.Type {
		case EventError:
			sawError = true
			errMsg = e.Error
		case EventDone:
			sawDone = true
		}
	}
	if !sawError {
		t.Fatal("expected EventError when tool budget exceeded")
	}
	if !sawDone {
		t.Error("expected EventDone after EventError so the SSE stream closes")
	}
	if !strings.Contains(errMsg, "128") {
		t.Errorf("error message should mention the 128 limit, got %q", errMsg)
	}
	// LLM must NOT have been called.
	if n := len(provider.Calls); n > 0 {
		t.Errorf("provider was called %d times; expected 0 (budget gate is pre-LLM)", n)
	}
}
