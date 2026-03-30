package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicProvider wraps the official Anthropic Go SDK for direct Anthropic
// API access (not Bedrock — see bedrockProvider).
//
// Design ref: docs/chatbot-design-spec.md §4.3
type anthropicProvider struct {
	client *anthropic.Client
	cfg    ProviderConfig
}

func newAnthropicProvider(cfg ProviderConfig) (*anthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: api_key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}

	client := anthropic.NewClient(opts...)
	return &anthropicProvider{client: &client, cfg: cfg}, nil
}

func (p *anthropicProvider) Name() string {
	return "Anthropic"
}

func (p *anthropicProvider) ValidateConfig(ctx context.Context) error {
	if p.cfg.APIKey == "" {
		return fmt.Errorf("anthropic: api_key is required")
	}
	if p.cfg.Model == "" {
		return fmt.Errorf("anthropic: model is required")
	}
	// Attempt a minimal call to validate credentials.
	_, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.cfg.Model),
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("ping")),
		},
	})
	if err != nil {
		return fmt.Errorf("anthropic: connection test failed: %w", err)
	}
	return nil
}

func (p *anthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	page, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("anthropic: list models: %w", err)
	}

	var models []ModelInfo
	for _, m := range page.Data {
		models = append(models, ModelInfo{
			ID:          m.ID,
			DisplayName: m.DisplayName,
		})
	}
	return models, nil
}

func (p *anthropicProvider) ChatStream(ctx context.Context, req ChatRequest) iter.Seq2[StreamEvent, error] {
	params := p.buildParams(req)

	return func(yield func(StreamEvent, error) bool) {
		stream := p.client.Messages.NewStreaming(ctx, params)
		defer func() {
			_ = stream.Close()
		}()

		// Track in-progress tool calls by content block index.
		type toolState struct {
			id          string
			name        string
			inputChunks []string
		}
		activeTools := make(map[int64]*toolState)

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "content_block_start":
				ev := event.AsContentBlockStart()
				cb := ev.ContentBlock
				if cb.Type == "tool_use" {
					activeTools[ev.Index] = &toolState{
						id:   cb.ID,
						name: cb.Name,
					}
				}

			case "content_block_delta":
				ev := event.AsContentBlockDelta()
				switch ev.Delta.Type {
				case "text_delta":
					td := ev.Delta.AsTextDelta()
					if td.Text != "" {
						if !yield(StreamEvent{Type: EventText, Delta: td.Text}, nil) {
							return
						}
					}
				case "input_json_delta":
					jd := ev.Delta.AsInputJSONDelta()
					if ts, ok := activeTools[ev.Index]; ok {
						ts.inputChunks = append(ts.inputChunks, jd.PartialJSON)
					}
				}

			case "content_block_stop":
				ev := event.AsContentBlockStop()
				if ts, ok := activeTools[ev.Index]; ok {
					input := json.RawMessage(strings.Join(ts.inputChunks, ""))
					tc := &ToolCall{
						ID:    ts.id,
						Name:  ts.name,
						Input: input,
					}
					delete(activeTools, ev.Index)
					if !yield(StreamEvent{Type: EventToolCall, ToolCall: tc}, nil) {
						return
					}
				}

			case "message_stop":
				// Terminal event — handled after loop
			}
		}

		if err := stream.Err(); err != nil {
			yield(StreamEvent{}, fmt.Errorf("anthropic stream: %w", err))
			return
		}

		yield(StreamEvent{Type: EventDone}, nil)
	}
}

// buildParams converts our ChatRequest into Anthropic SDK params.
func (p *anthropicProvider) buildParams(req ChatRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: 4096,
	}

	// System message
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		params.Messages = append(params.Messages, convertAnthropicMessage(msg))
	}

	// Convert tools
	for _, tool := range req.Tools {
		params.Tools = append(params.Tools, convertAnthropicTool(tool))
	}

	return params
}

func convertAnthropicMessage(msg Message) anthropic.MessageParam {
	switch msg.Role {
	case RoleUser:
		return anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content))

	case RoleAssistant:
		blocks := []anthropic.ContentBlockParamUnion{}
		if msg.Content != "" {
			blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
		}
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Input),
				},
			})
		}
		return anthropic.NewAssistantMessage(blocks...)

	case RoleTool:
		return anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
		)

	default:
		return anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content))
	}
}

func convertAnthropicTool(tool ToolDef) anthropic.ToolUnionParam {
	var props map[string]any
	var required []string
	if len(tool.Schema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(tool.Schema, &schema); err == nil {
			if p, ok := schema["properties"]; ok {
				if pm, ok := p.(map[string]any); ok {
					props = pm
				}
			}
			if r, ok := schema["required"]; ok {
				if ra, ok := r.([]any); ok {
					for _, v := range ra {
						if s, ok := v.(string); ok {
							required = append(required, s)
						}
					}
				}
			}
		}
	}

	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: props,
				Required:   required,
			},
		},
	}
}
