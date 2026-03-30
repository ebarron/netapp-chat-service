package agent

import (
	"encoding/json"
	"strings"
	"testing"
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

	// Should fall back to emitting as regular text.
	var textEvents []Event
	for _, e := range events {
		if e.Type == EventText {
			textEvents = append(textEvents, e)
		}
	}
	if len(textEvents) == 0 {
		t.Fatal("expected text fallback for malformed JSON")
	}
	var canvasEvents []Event
	for _, e := range events {
		if e.Type == EventCanvasOpen {
			canvasEvents = append(canvasEvents, e)
		}
	}
	if len(canvasEvents) != 0 {
		t.Errorf("expected no canvas events for malformed JSON, got %d", len(canvasEvents))
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

	// Should emit as regular text (graceful fallback).
	var textEvents []Event
	for _, e := range events {
		if e.Type == EventText {
			textEvents = append(textEvents, e)
		}
	}
	if len(textEvents) == 0 {
		t.Fatal("incomplete fence should be flushed as text")
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
		{"`", true},          // at start, 1 backtick — could start a fence
		{"``", true},         // at start, 2 backticks
		{"```", true},        // at start, 3 backticks — fence-start position
		{"```c", true},       // heading toward canvas
		{"```canvas-", true},
		{"```canvas-d", true},
		{"```d", false},      // diverged — regular fence
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
