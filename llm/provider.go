// Package llm provides a multi-provider LLM client abstraction with streaming
// and tool calling support. It wraps the official OpenAI and Anthropic Go SDKs
// behind a thin Provider interface.
//
// Design ref: docs/chatbot-design-spec.md §4.3
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"gopkg.in/yaml.v3"
)

// ModelInfo describes a single model available from a provider.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
}

// Provider is the core abstraction for LLM backends. Each provider (OpenAI,
// Anthropic, Bedrock) implements this interface. The chatbot's agent loop
// calls ChatStream to send messages + tool definitions and receives a stream
// of events back.
type Provider interface {
	// ChatStream sends a conversation with tool definitions to the LLM and
	// returns an iterator of streaming events. The caller must consume the
	// iterator; cancelling ctx stops the stream.
	ChatStream(ctx context.Context, req ChatRequest) iter.Seq2[StreamEvent, error]

	// Name returns a human-readable provider name (e.g. "OpenAI", "Anthropic").
	Name() string

	// ValidateConfig checks that the provider configuration is usable
	// (e.g. API key set, endpoint reachable). Returns nil if valid.
	ValidateConfig(ctx context.Context) error

	// ListModels returns the models available from this provider.
	// Returns an empty slice (not an error) if the provider does not support
	// model discovery.
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// ChatRequest holds everything the LLM needs for a single turn.
type ChatRequest struct {
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
	System   string    `json:"system,omitempty"`
	Model    string    `json:"model"`
	MaxTurns int       `json:"max_turns,omitempty"` // max tool-call iterations (safety)
}

// Role enumerates valid message roles.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single conversation turn.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role=tool only
}

// ToolDef describes a tool available to the LLM. The schema is the JSON Schema
// for the tool's input parameters, sourced from MCP tools/list.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	// ReadOnlyHint indicates the tool does not modify its environment.
	// Sourced from MCP ToolAnnotations.ReadOnlyHint or per-server allowlist.
	// Default false — assume tools may write unless explicitly marked safe.
	ReadOnlyHint bool `json:"read_only_hint,omitempty"`
	// DestructiveHint indicates the tool may perform destructive updates.
	// Sourced from MCP ToolAnnotations.DestructiveHint when present.
	DestructiveHint bool `json:"destructive_hint,omitempty"`
}

// ToolCall represents the LLM requesting a tool invocation.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// StreamEventType enumerates the kinds of streaming events.
type StreamEventType int

const (
	// EventText means Delta contains a text token.
	EventText StreamEventType = iota
	// EventToolCall means ToolCall is populated with a complete tool call request.
	EventToolCall
	// EventDone signals the end of the stream.
	EventDone
)

// StreamEvent is a single event from the LLM streaming response.
type StreamEvent struct {
	Type     StreamEventType `json:"type"`
	Delta    string          `json:"delta,omitempty"`     // text token (EventText)
	ToolCall *ToolCall       `json:"tool_call,omitempty"` // tool request (EventToolCall)
}

// ProviderConfig holds the configuration for a single LLM provider.
// Stored in /etc/nabox/ai.yaml.
type ProviderConfig struct {
	Provider     string `yaml:"provider" json:"provider"`                             // "openai", "anthropic", "bedrock", "custom", "llm-proxy"
	Endpoint     string `yaml:"endpoint" json:"endpoint"`                             // API endpoint URL
	APIKey       string `yaml:"api_key" json:"api_key,omitempty"`                     // masked in GET responses
	Model        string `yaml:"model" json:"model"`                                   // e.g. "gpt-4.1", "claude-sonnet-4"
	User         string `yaml:"user,omitempty" json:"user,omitempty"`                 // LLM Proxy: identifies the calling user
	AWSRegion    string `yaml:"aws_region,omitempty" json:"aws_region,omitempty"`      // Bedrock only
	AWSAccessKey string `yaml:"aws_access_key,omitempty" json:"aws_access_key,omitempty"`
	AWSSecretKey string `yaml:"aws_secret_key,omitempty" json:"aws_secret_key,omitempty"`
}

// AIFileConfig is the full ai.yaml file structure. It embeds the LLM provider
// configuration inline and adds capability state persistence.
//
// Design ref: docs/chatbot-design-spec.md §7.4, Open Question #8
type AIFileConfig struct {
	ProviderConfig `yaml:",inline"`
	Capabilities   map[string]string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

// MarshalProviderConfig serializes a ProviderConfig to YAML bytes.
func MarshalProviderConfig(cfg *ProviderConfig) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// UnmarshalProviderConfig deserializes YAML bytes into a ProviderConfig.
func UnmarshalProviderConfig(data []byte, cfg *ProviderConfig) error {
	return yaml.Unmarshal(data, cfg)
}

// MarshalAIFileConfig serializes the full AI config (provider + capabilities).
func MarshalAIFileConfig(cfg *AIFileConfig) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// UnmarshalAIFileConfig deserializes the full AI config from YAML.
func UnmarshalAIFileConfig(data []byte, cfg *AIFileConfig) error {
	return yaml.Unmarshal(data, cfg)
}

// NewProvider creates a Provider from the given configuration.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.Provider {
	case "openai", "custom", "llm-proxy":
		return newOpenAIProvider(cfg)
	case "anthropic":
		return newAnthropicProvider(cfg)
	case "bedrock":
		return newBedrockProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", cfg.Provider)
	}
}
