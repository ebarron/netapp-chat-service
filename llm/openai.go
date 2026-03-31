package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// openaiProvider wraps the official OpenAI Go SDK. It also supports any
// OpenAI-compatible endpoint (vLLM, Ollama, Azure, etc.) via the "custom"
// provider type.
//
// Design ref: docs/chatbot-design-spec.md §4.3
type openaiProvider struct {
	client *openai.Client
	cfg    ProviderConfig
}

func newOpenAIProvider(cfg ProviderConfig) (*openaiProvider, error) {
	if cfg.Endpoint == "" {
		if cfg.Provider == "openai" {
			cfg.Endpoint = "https://api.openai.com/v1"
		} else if cfg.Provider == "llm-proxy" {
			return nil, fmt.Errorf("llm-proxy: endpoint is required")
		} else {
			return nil, fmt.Errorf("openai: endpoint is required")
		}
	}

	opts := []option.RequestOption{
		option.WithBaseURL(cfg.Endpoint),
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}

	client := openai.NewClient(opts...)
	return &openaiProvider{client: &client, cfg: cfg}, nil
}

func (p *openaiProvider) Name() string {
	switch p.cfg.Provider {
	case "custom":
		return "Custom (OpenAI-compatible)"
	case "llm-proxy":
		return "LLM Proxy"
	default:
		return "OpenAI"
	}
}

func (p *openaiProvider) ValidateConfig(ctx context.Context) error {
	if p.cfg.Endpoint == "" {
		return fmt.Errorf("openai: endpoint is required (set to https://api.openai.com/v1 for OpenAI)")
	}
	if p.cfg.Model == "" {
		return fmt.Errorf("openai: model is required")
	}
	// Attempt a minimal non-streaming call to validate credentials.
	params := openai.ChatCompletionNewParams{
		Model:     p.cfg.Model,
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("ping")},
		MaxTokens: openai.Int(1),
	}
	if p.cfg.User != "" {
		params.User = openai.String(p.cfg.User)
	}
	_, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return fmt.Errorf("openai: connection test failed: %w", err)
	}
	return nil
}

func (p *openaiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	page, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai: list models: %w", err)
	}

	var models []ModelInfo
	for _, m := range page.Data {
		models = append(models, ModelInfo{
			ID:      m.ID,
			OwnedBy: m.OwnedBy,
		})
	}
	return models, nil
}

func (p *openaiProvider) ChatStream(ctx context.Context, req ChatRequest) iter.Seq2[StreamEvent, error] {
	params := p.buildParams(req)

	slog.Debug("openai: starting stream",
		"model", req.Model,
		"provider", p.cfg.Provider,
		"tools", len(req.Tools),
		"messages", len(req.Messages),
	)

	return func(yield func(StreamEvent, error) bool) {
		stream := p.client.Chat.Completions.NewStreaming(ctx, params)
		defer func() {
			_ = stream.Close()
		}()

		acc := openai.ChatCompletionAccumulator{}

		// Track tool calls yielded via JustFinishedToolCall to detect
		// any that the accumulator's state machine missed (can happen
		// with certain proxy implementations).
		yieldedToolCalls := make(map[int]bool)
		chunkCount := 0
		var lastFinishReason string

		for stream.Next() {
			chunk := stream.Current()
			chunkCount++
			acc.AddChunk(chunk)

			for _, choice := range chunk.Choices {
				if choice.FinishReason != "" {
					lastFinishReason = string(choice.FinishReason)
					slog.Debug("openai: chunk finish_reason",
						"chunk", chunkCount,
						"finish_reason", lastFinishReason,
					)
				}

				// Log tool call deltas as they arrive.
				for _, tc := range choice.Delta.ToolCalls {
					slog.Debug("openai: tool call delta",
						"chunk", chunkCount,
						"index", tc.Index,
						"id", tc.ID,
						"name", tc.Function.Name,
						"args_len", len(tc.Function.Arguments),
					)
				}

				if choice.Delta.Content != "" {
					if !yield(StreamEvent{Type: EventText, Delta: choice.Delta.Content}, nil) {
						return
					}
				}
			}

			// Check for completed tool calls (SDK state machine).
			if tc, ok := acc.JustFinishedToolCall(); ok {
				slog.Debug("openai: JustFinishedToolCall fired",
					"index", tc.Index,
					"name", tc.Name,
					"id", tc.ID,
				)
				yieldedToolCalls[tc.Index] = true
				toolCall := &ToolCall{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Arguments),
				}
				if !yield(StreamEvent{Type: EventToolCall, ToolCall: toolCall}, nil) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			yield(StreamEvent{}, fmt.Errorf("openai stream: %w", err))
			return
		}

		// Log stream summary.
		accToolCalls := 0
		if len(acc.Choices) > 0 {
			accToolCalls = len(acc.Choices[0].Message.ToolCalls)
		}
		slog.Debug("openai: stream finished",
			"chunks", chunkCount,
			"finish_reason", lastFinishReason,
			"yielded_tool_calls", len(yieldedToolCalls),
			"accumulated_tool_calls", accToolCalls,
			"has_content", len(acc.Choices) > 0 && acc.Choices[0].Message.Content != "",
		)

		// Fallback: check for tool calls that were accumulated but not
		// detected by JustFinishedToolCall. This handles OpenAI-compatible
		// proxies (e.g. llm-proxy with Claude models) that may chunk
		// tool call streaming differently than the SDK state machine expects.
		if len(acc.Choices) > 0 {
			for i, tc := range acc.Choices[0].Message.ToolCalls {
				if yieldedToolCalls[i] || tc.Function.Name == "" {
					continue
				}
				slog.Info("openai: recovered tool call from accumulator fallback",
					"index", i,
					"name", tc.Function.Name,
					"id", tc.ID,
				)
				toolCall := &ToolCall{
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				}
				if !yield(StreamEvent{Type: EventToolCall, ToolCall: toolCall}, nil) {
					return
				}
			}
		}

		yield(StreamEvent{Type: EventDone}, nil)
	}
}

// buildParams converts our ChatRequest into OpenAI SDK params.
func (p *openaiProvider) buildParams(req ChatRequest) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model: req.Model,
	}

	if p.cfg.User != "" {
		params.User = openai.String(p.cfg.User)
	}

	// Convert messages
	for _, msg := range req.Messages {
		params.Messages = append(params.Messages, convertOpenAIMessage(msg))
	}

	// Add system message if provided
	if req.System != "" {
		sys := openai.SystemMessage(req.System)
		params.Messages = append([]openai.ChatCompletionMessageParamUnion{sys}, params.Messages...)
	}

	// Convert tools
	for _, tool := range req.Tools {
		params.Tools = append(params.Tools, convertOpenAITool(tool))
	}

	return params
}

func convertOpenAIMessage(msg Message) openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case RoleSystem:
		return openai.SystemMessage(msg.Content)
	case RoleUser:
		return openai.UserMessage(msg.Content)
	case RoleAssistant:
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				toolCalls[i] = openai.ChatCompletionMessageToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: string(tc.Input),
					},
				}
			}
			return openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(msg.Content)},
					ToolCalls: toolCalls,
				},
			}
		}
		return openai.AssistantMessage(msg.Content)
	case RoleTool:
		return openai.ToolMessage(msg.Content, msg.ToolCallID)
	default:
		return openai.UserMessage(msg.Content)
	}
}

func convertOpenAITool(tool ToolDef) openai.ChatCompletionToolParam {
	var schema shared.FunctionParameters
	if len(tool.Schema) > 0 {
		_ = json.Unmarshal(tool.Schema, &schema)
	}

	// OpenAI requires "properties" on object schemas — MCP servers may omit it
	// for tools that accept no parameters.
	if schema != nil {
		if t, _ := schema["type"].(string); t == "object" {
			if _, ok := schema["properties"]; !ok {
				schema["properties"] = map[string]any{}
			}
		}
	}

	return openai.ChatCompletionToolParam{
		Function: shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
			Parameters:  schema,
		},
	}
}
