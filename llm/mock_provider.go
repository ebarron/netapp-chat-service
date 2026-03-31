package llm

import (
	"context"
	"encoding/json"
	"iter"
)

// MockProvider is a deterministic Provider for tests. It replays a fixed
// sequence of StreamEvents for each ChatStream call. No network calls.
//
// Design ref: docs/chatbot-design-spec.md §9.3 (mock strategy)
type MockProvider struct {
	ProviderName string
	// Responses is a queue of event sequences. Each ChatStream call pops the
	// first entry. If empty, returns a single EventDone.
	Responses [][]StreamEvent
	// Calls records every ChatRequest received, for assertion.
	Calls []ChatRequest
	// ValidateErr if set, ValidateConfig returns this error.
	ValidateErr error
}

func (m *MockProvider) Name() string {
	if m.ProviderName != "" {
		return m.ProviderName
	}
	return "mock"
}

func (m *MockProvider) ValidateConfig(_ context.Context) error {
	return m.ValidateErr
}

func (m *MockProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: "mock-model", DisplayName: "Mock Model"}}, nil
}

func (m *MockProvider) ChatStream(_ context.Context, req ChatRequest) iter.Seq2[StreamEvent, error] {
	m.Calls = append(m.Calls, req)

	var events []StreamEvent
	if len(m.Responses) > 0 {
		events = m.Responses[0]
		m.Responses = m.Responses[1:]
	}
	if len(events) == 0 {
		events = []StreamEvent{{Type: EventDone}}
	}

	return func(yield func(StreamEvent, error) bool) {
		for _, e := range events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// MockTextResponse is a helper that builds a response sequence for a simple
// text reply split into token chunks.
func MockTextResponse(tokens ...string) []StreamEvent {
	events := make([]StreamEvent, 0, len(tokens)+1)
	for _, t := range tokens {
		events = append(events, StreamEvent{Type: EventText, Delta: t})
	}
	events = append(events, StreamEvent{Type: EventDone})
	return events
}

// MockToolCallResponse builds a response sequence where the LLM requests a
// tool call.
func MockToolCallResponse(id, name string, input any) []StreamEvent {
	raw, _ := json.Marshal(input)
	return []StreamEvent{
		{Type: EventText, Delta: "Let me check..."},
		{Type: EventToolCall, ToolCall: &ToolCall{ID: id, Name: name, Input: raw}},
	}
}

// MockToolThenText builds a two-turn response: first a tool call, then (after
// the tool result is fed back) a text reply. Use two entries in Responses.
func MockToolThenText(toolID, toolName string, input any, textTokens ...string) [][]StreamEvent {
	return [][]StreamEvent{
		MockToolCallResponse(toolID, toolName, input),
		MockTextResponse(textTokens...),
	}
}
