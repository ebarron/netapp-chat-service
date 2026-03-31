package llm

import (
	"context"
	"fmt"
	"iter"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// bedrockProvider wraps the Anthropic Go SDK's Bedrock sub-package. It
// creates an Anthropic client configured with AWS SigV4 signing so requests
// go through Amazon Bedrock instead of the Anthropic API directly.
//
// Design ref: docs/chatbot-design-spec.md §4.3
type bedrockProvider struct {
	// Embed anthropicProvider to reuse message/tool conversion and streaming logic.
	anthropicProvider
}

func newBedrockProvider(cfg ProviderConfig) (*bedrockProvider, error) {
	if cfg.AWSRegion == "" {
		return nil, fmt.Errorf("bedrock: aws_region is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("bedrock: model is required")
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.AWSRegion),
	}

	// If explicit credentials are provided, set them as static credentials.
	if cfg.AWSAccessKey != "" && cfg.AWSSecretKey != "" {
		loadOpts = append(loadOpts,
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AWSAccessKey, cfg.AWSSecretKey, ""),
			),
		)
	}

	opts := []option.RequestOption{
		bedrock.WithLoadDefaultConfig(context.Background(), loadOpts...),
	}

	client := anthropic.NewClient(opts...)
	return &bedrockProvider{
		anthropicProvider: anthropicProvider{
			client: &client,
			cfg:    cfg,
		},
	}, nil
}

func (p *bedrockProvider) Name() string {
	return "Bedrock"
}

func (p *bedrockProvider) ValidateConfig(ctx context.Context) error {
	if p.cfg.AWSRegion == "" {
		return fmt.Errorf("bedrock: aws_region is required")
	}
	if p.cfg.Model == "" {
		return fmt.Errorf("bedrock: model is required")
	}
	// Attempt a minimal call to validate credentials + model access.
	_, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.cfg.Model),
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("ping")),
		},
	})
	if err != nil {
		return fmt.Errorf("bedrock: connection test failed: %w", err)
	}
	return nil
}

// ListModels returns an empty list for Bedrock; model discovery requires the
// AWS Bedrock ListFoundationModels API which is not available through the
// Anthropic SDK.
func (p *bedrockProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return nil, nil
}

// ChatStream is inherited from anthropicProvider — the Bedrock middleware
// transparently handles the AWS signing and endpoint rewriting.
func (p *bedrockProvider) ChatStream(ctx context.Context, req ChatRequest) iter.Seq2[StreamEvent, error] {
	return p.anthropicProvider.ChatStream(ctx, req)
}
