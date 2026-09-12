package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ebarron/netapp-chat-service/llm"
	"github.com/ebarron/netapp-chat-service/mcpclient"
)

func TestCanvasFenceInterceptor_DetectsObjectDetail(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)

	// Simulate streaming tokens that form a canvas-object-detail fence.
	tokens := []string{
		"Here is your volume.\n\n",
		"```canvas-object-detail\n",
		`{"type":"object-detail","kind":"volume","name":"vol1","qualifier":"on SVM svm1 on cluster cls1","sections":[]}`,
		"\n```\n",
	}
	for _, tok := range tokens {
		ci.HandleToken(tok)
	}
	ci.Flush()

	// Expect: one text event ("Here is your volume.\n\n"), one canvas event.
	var textEvents, canvasEvents []Event
	for _, e := range events {
		switch e.Type {
		case EventText:
			textEvents = append(textEvents, e)
		case EventCanvasOpen:
			canvasEvents = append(canvasEvents, e)
		}
	}

	if len(canvasEvents) != 1 {
		t.Fatalf("expected 1 canvas event, got %d: %+v", len(canvasEvents), events)
	}

	ce := canvasEvents[0]
	if ce.Canvas == nil {
		t.Fatal("canvas payload is nil")
	}
	if ce.Canvas.Kind != "volume" {
		t.Errorf("Kind = %q, want %q", ce.Canvas.Kind, "volume")
	}
	if ce.Canvas.Title != "vol1" {
		t.Errorf("Title = %q, want %q", ce.Canvas.Title, "vol1")
	}
	if ce.Canvas.TabID != "volume::vol1::on SVM svm1 on cluster cls1" {
		t.Errorf("TabID = %q, want %q", ce.Canvas.TabID, "volume::vol1::on SVM svm1 on cluster cls1")
	}

	// The inline text before the fence should have been emitted.
	var allText string
	for _, e := range textEvents {
		allText += e.Text
	}
	if !strings.Contains(allText, "Here is your volume.") {
		t.Errorf("expected pre-fence text, got %q", allText)
	}
	// The fence JSON should NOT appear in text events.
	if strings.Contains(allText, "object-detail") {
		t.Errorf("fence content should not appear in text events, got %q", allText)
	}
}

func TestCanvasFenceInterceptor_DetectsDashboard(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	ci.HandleToken("```canvas-dashboard\n")
	ci.HandleToken(`{"type":"dashboard","title":"Provision Plan","panels":[]}`)
	ci.HandleToken("\n```\n")
	ci.Flush()

	var canvasEvents []Event
	for _, e := range events {
		if e.Type == EventCanvasOpen {
			canvasEvents = append(canvasEvents, e)
		}
	}
	if len(canvasEvents) != 1 {
		t.Fatalf("expected 1 canvas event, got %d", len(canvasEvents))
	}
	if canvasEvents[0].Canvas.Kind != "dashboard" {
		t.Errorf("Kind = %q, want %q", canvasEvents[0].Canvas.Kind, "dashboard")
	}
	if canvasEvents[0].Canvas.Title != "Provision Plan" {
		t.Errorf("Title = %q, want %q", canvasEvents[0].Canvas.Title, "Provision Plan")
	}
}

func TestCanvasFenceInterceptor_MalformedJSON(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	ci.HandleToken("```canvas-object-detail\n")
	ci.HandleToken(`{not valid json`)
	ci.HandleToken("\n```\n")
	ci.Flush()

	// Malformed structured content is rejected rather than entering UI state
	// or leaking its raw model-generated contents into the transcript.
	for _, e := range events {
		if e.Type == EventCanvasOpen || e.Type == EventText {
			t.Fatalf("expected malformed canvas payload to be suppressed, got %+v", events)
		}
	}
}

func TestCanvasFenceInterceptor_RegularFencePassthrough(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	// Regular object-detail fence (not canvas-) should pass through.
	ci.HandleToken("```object-detail\n")
	ci.HandleToken(`{"type":"object-detail","kind":"volume","name":"vol1","sections":[]}`)
	ci.HandleToken("\n```\n")
	ci.Flush()

	// Should all be text events, no canvas events.
	var canvasEvents []Event
	for _, e := range events {
		if e.Type == EventCanvasOpen {
			canvasEvents = append(canvasEvents, e)
		}
	}
	if len(canvasEvents) != 0 {
		t.Errorf("regular fence should not produce canvas events, got %d", len(canvasEvents))
	}

	// Text should contain the fence content.
	var allText string
	for _, e := range events {
		if e.Type == EventText {
			allText += e.Text
		}
	}
	if !strings.Contains(allText, "object-detail") {
		t.Error("regular fence content should appear in text events")
	}
}

func TestCanvasFenceInterceptor_IncompleteFlush(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	// Start a canvas fence but never close it.
	ci.HandleToken("```canvas-object-detail\n")
	ci.HandleToken(`{"type":"object-detail"}`)
	// Stream ends without closing fence.
	ci.Flush()

	// Partial structured content is rejected and never enters UI state.
	if len(events) != 0 {
		t.Fatalf("expected incomplete canvas payload to be suppressed, got %+v", events)
	}
}

func TestCanvasFenceInterceptor_EmptyIncompleteFenceDoesNotSwallowLaterText(t *testing.T) {
	var events []Event
	ci := newCanvasFenceInterceptor(func(e Event) { events = append(events, e) })

	ci.HandleToken("```canvas-dashboard\n")
	ci.Flush()
	ci.HandleToken("ordinary follow-up text")
	ci.Flush()

	if len(events) != 1 || events[0].Type != EventText || events[0].Text != "ordinary follow-up text" {
		t.Fatalf("interceptor remained stuck after empty partial fence: %+v", events)
	}
}

func TestCanvasFenceInterceptor_RejectsMalformedDuplicateAndPreservesValid(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }
	ci := newCanvasFenceInterceptor(emit)

	valid := `{"type":"dashboard","title":"Alert Rule","panels":[{"type":"action-button","buttons":[{"label":"Delete","action":"message","message":"delete it"}]}]}`
	malformed := `{"type":"dashboard","title":"Alert Rule","panels":[{"type":"action-button"}]}`

	ci.HandleToken("```canvas-dashboard\n" + valid + "\n```\n")
	ci.HandleToken("```canvas-dashboard\n" + malformed + "\n```\n")
	ci.Flush()

	var canvasEvents []Event
	for _, event := range events {
		if event.Type == EventCanvasOpen {
			canvasEvents = append(canvasEvents, event)
		}
	}
	if len(canvasEvents) != 1 {
		t.Fatalf("expected only the valid dashboard event, got %d: %+v", len(canvasEvents), events)
	}
	if !strings.Contains(string(canvasEvents[0].Canvas.Content), `"buttons"`) {
		t.Fatalf("valid dashboard was not preserved: %s", canvasEvents[0].Canvas.Content)
	}
}

func TestAgentRun_PreservesEmitResultDashboardWhenFinalModelDuplicateIsMalformed(t *testing.T) {
	valid := `{"type":"dashboard","title":"Alert Rule","panels":[{"type":"action-button","buttons":[{"label":"Delete","action":"message","message":"delete it"}]}]}`
	malformed := `{"type":"dashboard","title":"Alert Rule","panels":[{"type":"action-button"}]}`
	provider := &llm.MockProvider{
		ProviderName: "mock",
		Responses: [][]llm.StreamEvent{
			llm.MockToolCallResponse("render-1", "render_dashboard", map[string]any{}),
			llm.MockTextResponse("```canvas-dashboard\n", malformed, "\n```\n"),
		},
	}
	tools := map[string]InternalTool{
		"render_dashboard": {
			Def: llm.ToolDef{
				Name:        "render_dashboard",
				Description: "Render a dashboard",
				Schema:      json.RawMessage(`{"type":"object"}`),
			},
			Handler: func(context.Context, json.RawMessage) (string, error) {
				return "```canvas-dashboard\n" + valid + "\n```", nil
			},
			EmitResult: true,
		},
	}
	ag := New(provider, mcpclient.NewMockRouter(nil), WithInternalTools(tools))
	events := collectEvents(t, ag, []llm.Message{{Role: llm.RoleUser, Content: "clone the rule"}})

	var canvases []*CanvasPayload
	for _, event := range events {
		if event.Type == EventCanvasOpen {
			canvases = append(canvases, event.Canvas)
		}
	}
	if len(canvases) != 1 {
		t.Fatalf("expected only the valid EmitResult dashboard, got %d canvas events", len(canvases))
	}
	if !strings.Contains(string(canvases[0].Content), `"buttons"`) {
		t.Fatalf("valid EmitResult dashboard was not preserved: %s", canvases[0].Content)
	}
}

func TestCanvasFenceInterceptor_RejectsDashboardMissingButtons(t *testing.T) {
	var events []Event
	ci := newCanvasFenceInterceptor(func(e Event) { events = append(events, e) })

	ci.HandleToken("```canvas-dashboard\n")
	ci.HandleToken(`{"type":"dashboard","title":"Broken","panels":[{"type":"action-button"}]}`)
	ci.HandleToken("\n```\n")
	ci.Flush()

	for _, event := range events {
		if event.Type == EventCanvasOpen {
			t.Fatalf("dashboard with an incomplete action-button must be rejected: %+v", event)
		}
	}
}

func TestCanvasFenceInterceptor_SuppressesExactDuplicate(t *testing.T) {
	var events []Event
	ci := newCanvasFenceInterceptor(func(e Event) { events = append(events, e) })
	payload := `{"type":"dashboard","title":"Fleet","panels":[]}`

	ci.HandleToken("```canvas-dashboard\n" + payload + "\n```\n")
	ci.HandleToken("```canvas-dashboard\n" + payload + "\n```\n")
	ci.Flush()

	count := 0
	for _, event := range events {
		if event.Type == EventCanvasOpen {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one canvas event for an exact duplicate, got %d", count)
	}
}

func TestNormalizeCanvasPayload_DefaultsOptionalCollections(t *testing.T) {
	payload := `{
		"type":"dashboard",
		"title":"Optional collections",
		"panels":[
			{"type":"area","title":"Trend","xKey":"time","series":[],"data":[]},
			{"type":"action-form","fields":[{"key":"mode","label":"Mode","type":"select"}],"submit":{"label":"Apply","tool":"apply"}}
		]
	}`

	normalized, _, err := normalizeCanvasPayload(payload)
	if err != nil {
		t.Fatalf("normalizeCanvasPayload returned error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(normalized, &got); err != nil {
		t.Fatal(err)
	}
	panels := got["panels"].([]any)
	area := panels[0].(map[string]any)
	if annotations, ok := area["annotations"].([]any); !ok || len(annotations) != 0 {
		t.Fatalf("annotations not normalized to []: %#v", area["annotations"])
	}
	form := panels[1].(map[string]any)
	field := form["fields"].([]any)[0].(map[string]any)
	if options, ok := field["options"].([]any); !ok || len(options) != 0 {
		t.Fatalf("options not normalized to []: %#v", field["options"])
	}
}

func TestCanvasPayload_JSON(t *testing.T) {
	payload := CanvasPayload{
		TabID:     "volume::vol1::on cluster cls1",
		Title:     "vol1",
		Kind:      "volume",
		Qualifier: "on cluster cls1",
		Content:   json.RawMessage(`{"type":"object-detail"}`),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"tab_id"`) || !strings.Contains(s, `"kind"`) {
		t.Errorf("unexpected JSON: %s", s)
	}
}

func TestCanvasFenceInterceptor_RegularDashboardSplitTokens(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	// Simulate LLM streaming backticks and "dashboard" as separate tokens.
	ci.HandleToken("```")
	ci.HandleToken("dashboard\n")
	ci.HandleToken(`{"title":"Fleet","panels":[]}`)
	ci.HandleToken("\n```\n")
	ci.Flush()

	// Should all be text events, no canvas events.
	for _, e := range events {
		if e.Type == EventCanvasOpen {
			t.Fatal("regular dashboard fence should not produce canvas events")
		}
	}

	var allText string
	for _, e := range events {
		if e.Type == EventText {
			allText += e.Text
		}
	}
	// The full fence must be reconstructed in text output.
	if !strings.Contains(allText, "```dashboard") {
		t.Errorf("regular dashboard fence not preserved in text output: %q", allText)
	}
}

func TestCanvasFenceInterceptor_CanvasDashboardSplitTokens(t *testing.T) {
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	ci := newCanvasFenceInterceptor(emit)
	// Simulate LLM tokenizing canvas-dashboard fence across 3 tokens.
	ci.HandleToken("```")
	ci.HandleToken("canvas")
	ci.HandleToken("-dashboard\n")
	ci.HandleToken(`{"type":"dashboard","title":"Provision","panels":[]}` + "\n")
	ci.HandleToken("```\n")
	ci.Flush()

	var canvasEvents []Event
	for _, e := range events {
		if e.Type == EventCanvasOpen {
			canvasEvents = append(canvasEvents, e)
		}
	}
	if len(canvasEvents) != 1 {
		t.Fatalf("expected 1 canvas event, got %d; events: %v", len(canvasEvents), events)
	}
	if canvasEvents[0].Canvas.Kind != "dashboard" {
		t.Errorf("expected kind=dashboard, got %q", canvasEvents[0].Canvas.Kind)
	}
}

func TestCanvasPartialMatch(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"`", true},    // at start, 1 backtick — could start a fence
		{"``", true},   // at start, 2 backticks
		{"```", true},  // at start, 3 backticks — fence-start position
		{"```c", true}, // heading toward canvas
		{"```canvas-", true},
		{"```canvas-d", true},
		{"```d", false}, // diverged — regular fence
		{"```dashboard", false},
		{"```object-detail", false},
		{"hello```", false},  // backticks not at fence-start (no preceding newline)
		{"hello```c", true},  // 4+ char prefix match
		{"hello\n```", true}, // backticks after newline — fence-start
		{"hello\n``", true},  // partial backticks after newline
		{"hello\n`", true},   // single backtick after newline
	}
	for _, tt := range tests {
		got := canvasPartialMatch(tt.input)
		if got != tt.want {
			t.Errorf("canvasPartialMatch(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
