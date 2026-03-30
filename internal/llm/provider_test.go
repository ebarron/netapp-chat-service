package llm

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestMockProviderName(t *testing.T) {
	tests := []struct {
		name     string
		provider MockProvider
		want     string
	}{
		{"default name", MockProvider{}, "mock"},
		{"custom name", MockProvider{ProviderName: "test-llm"}, "test-llm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMockProviderValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"valid", nil, false},
		{"invalid", errors.New("bad key"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MockProvider{ValidateErr: tt.err}
			err := m.ValidateConfig(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMockProviderChatStreamText(t *testing.T) {
	m := &MockProvider{
		Responses: [][]StreamEvent{
			MockTextResponse("Hello", " ", "world"),
		},
	}

	req := ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Model:    "test",
	}

	var events []StreamEvent
	for e, err := range m.ChatStream(context.Background(), req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, e)
	}

	// Should get 3 text events + 1 done
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
	if events[0].Type != EventText || events[0].Delta != "Hello" {
		t.Errorf("event[0] = %+v, want text 'Hello'", events[0])
	}
	if events[1].Type != EventText || events[1].Delta != " " {
		t.Errorf("event[1] = %+v, want text ' '", events[1])
	}
	if events[2].Type != EventText || events[2].Delta != "world" {
		t.Errorf("event[2] = %+v, want text 'world'", events[2])
	}
	if events[3].Type != EventDone {
		t.Errorf("event[3] = %+v, want done", events[3])
	}

	// Verify request was recorded
	if len(m.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(m.Calls))
	}
	if m.Calls[0].Model != "test" {
		t.Errorf("recorded model = %q, want %q", m.Calls[0].Model, "test")
	}
}

func TestMockProviderChatStreamToolCall(t *testing.T) {
	input := map[string]string{"query": "up"}
	m := &MockProvider{
		Responses: [][]StreamEvent{
			MockToolCallResponse("call-1", "metrics_query", input),
		},
	}

	req := ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "check metrics"}},
		Model:    "test",
	}

	var events []StreamEvent
	for e, err := range m.ChatStream(context.Background(), req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Type != EventText {
		t.Errorf("event[0].Type = %d, want EventText", events[0].Type)
	}
	if events[1].Type != EventToolCall {
		t.Fatalf("event[1].Type = %d, want EventToolCall", events[1].Type)
	}

	tc := events[1].ToolCall
	if tc.ID != "call-1" || tc.Name != "metrics_query" {
		t.Errorf("tool call = %+v, want id=call-1 name=metrics_query", tc)
	}

	var gotInput map[string]string
	if err := json.Unmarshal(tc.Input, &gotInput); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if !reflect.DeepEqual(gotInput, input) {
		t.Errorf("input = %v, want %v", gotInput, input)
	}
}

func TestMockProviderEmptyResponses(t *testing.T) {
	m := &MockProvider{}

	req := ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Model:    "test",
	}

	var events []StreamEvent
	for e, err := range m.ChatStream(context.Background(), req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, e)
	}

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (done)", len(events))
	}
	if events[0].Type != EventDone {
		t.Errorf("event[0].Type = %d, want EventDone", events[0].Type)
	}
}

func TestMockProviderMultipleRoundTrips(t *testing.T) {
	// Simulate a tool-then-text flow: first call returns tool call,
	// second call returns text.
	m := &MockProvider{
		Responses: MockToolThenText(
			"tc-1", "get_volumes", map[string]string{},
			"Here", " are", " volumes",
		),
	}

	// First round: tool call
	req1 := ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "show volumes"}},
		Model:    "test",
	}
	var events1 []StreamEvent
	for e, err := range m.ChatStream(context.Background(), req1) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events1 = append(events1, e)
	}
	if events1[len(events1)-1].Type != EventToolCall {
		t.Fatalf("expected last event to be tool call, got %+v", events1[len(events1)-1])
	}

	// Second round: text response after feeding tool result
	req2 := ChatRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "show volumes"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "tc-1", Name: "get_volumes"}}},
			{Role: RoleTool, Content: `[{"name":"vol1"}]`, ToolCallID: "tc-1"},
		},
		Model: "test",
	}
	var events2 []StreamEvent
	for e, err := range m.ChatStream(context.Background(), req2) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events2 = append(events2, e)
	}
	if events2[len(events2)-1].Type != EventDone {
		t.Fatalf("expected last event to be done, got %+v", events2[len(events2)-1])
	}

	if len(m.Calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(m.Calls))
	}
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestStreamEventTypes(t *testing.T) {
	// Verify constants are distinct
	types := []StreamEventType{EventText, EventToolCall, EventDone}
	seen := make(map[StreamEventType]bool)
	for _, ty := range types {
		if seen[ty] {
			t.Errorf("duplicate StreamEventType: %d", ty)
		}
		seen[ty] = true
	}
}

func TestRoleConstants(t *testing.T) {
	roles := []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool}
	seen := make(map[Role]bool)
	for _, r := range roles {
		if r == "" {
			t.Error("empty role constant")
		}
		if seen[r] {
			t.Errorf("duplicate role: %q", r)
		}
		seen[r] = true
	}
}

func TestNewProviderOpenAI(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Provider: "openai",
		Endpoint: "https://api.openai.com/v1/",
		APIKey:   "sk-test",
		Model:    "gpt-4.1",
	})
	if err != nil {
		t.Fatalf("NewProvider(openai) error: %v", err)
	}
	if p.Name() != "OpenAI" {
		t.Errorf("Name() = %q, want %q", p.Name(), "OpenAI")
	}
}

func TestNewProviderCustom(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Provider: "custom",
		Endpoint: "http://localhost:11434/v1/",
		Model:    "llama3",
	})
	if err != nil {
		t.Fatalf("NewProvider(custom) error: %v", err)
	}
	if p.Name() != "Custom (OpenAI-compatible)" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Custom (OpenAI-compatible)")
	}
}

func TestNewProviderOpenAIDefaultEndpoint(t *testing.T) {
	// When endpoint is omitted for "openai", the default should be applied.
	p, err := NewProvider(ProviderConfig{
		Provider: "openai",
		APIKey:   "sk-test",
		Model:    "gpt-4.1",
	})
	if err != nil {
		t.Fatalf("NewProvider(openai) error: %v", err)
	}
	if p.Name() != "OpenAI" {
		t.Errorf("Name() = %q, want %q", p.Name(), "OpenAI")
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Provider: "anthropic",
		APIKey:   "sk-ant-test",
		Model:    "claude-sonnet-4-20250514",
	})
	if err != nil {
		t.Fatalf("NewProvider(anthropic) error: %v", err)
	}
	if p.Name() != "Anthropic" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Anthropic")
	}
}

func TestNewProviderAnthropicMissingKey(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewProviderBedrock(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Provider:  "bedrock",
		AWSRegion: "us-east-1",
		Model:     "anthropic.claude-sonnet-4-20250514-v1:0",
	})
	if err != nil {
		t.Fatalf("NewProvider(bedrock) error: %v", err)
	}
	if p.Name() != "Bedrock" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Bedrock")
	}
}

func TestNewProviderBedrockMissingRegion(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Provider: "bedrock",
		Model:    "anthropic.claude-sonnet-4-20250514-v1:0",
	})
	if err == nil {
		t.Fatal("expected error for missing AWS region")
	}
}

func TestNewProviderBedrockMissingModel(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Provider:  "bedrock",
		AWSRegion: "us-east-1",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewProviderLLMProxy(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		Provider: "llm-proxy",
		Endpoint: "https://llm-proxy-api.example.com",
		APIKey:   "sk-proxy-test",
		Model:    "gpt-4.1",
		User:     "testuser",
	})
	if err != nil {
		t.Fatalf("NewProvider(llm-proxy) error: %v", err)
	}
	if p.Name() != "LLM Proxy" {
		t.Errorf("Name() = %q, want %q", p.Name(), "LLM Proxy")
	}
}

func TestNewProviderLLMProxyMissingEndpoint(t *testing.T) {
	_, err := NewProvider(ProviderConfig{
		Provider: "llm-proxy",
		APIKey:   "sk-proxy-test",
		Model:    "gpt-4.1",
	})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}
