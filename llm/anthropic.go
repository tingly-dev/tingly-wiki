package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// AnthropicAdapter wraps the Anthropic SDK
type AnthropicAdapter struct {
	client        anthropic.Client
	model         string
	promptBuilder *PromptBuilder
}

// AnthropicConfig configures the Anthropic adapter
type AnthropicConfig struct {
	// APIKey is the Anthropic API key (or set ANTHROPIC_API_KEY env var)
	APIKey string

	// Model is the model to use (default: claude-3-5-sonnet-20241022)
	Model string

	// BaseURL is the base URL (optional, for custom endpoints)
	BaseURL string
}

// NewAnthropicAdapter creates a new Anthropic adapter
func NewAnthropicAdapter(cfg *AnthropicConfig) (*AnthropicAdapter, error) {
	if cfg == nil {
		cfg = &AnthropicConfig{}
	}

	model := cfg.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022" // Good balance of cost and performance
	}

	opts := []option.RequestOption{}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	return &AnthropicAdapter{
		client:        client,
		model:         model,
		promptBuilder: NewPromptBuilder(),
	}, nil
}

// Extract extracts structured information from content
func (a *AnthropicAdapter) Extract(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
	prompt := a.promptBuilder.BuildExtractPrompt(content)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: prompt,
						},
					},
				},
			},
		},
		Model: anthropic.Model(a.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no response from Anthropic")
	}

	// Get text content
	contentText := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = block.Text
			break
		}
	}

	// Parse JSON response
	var extracted schema.ExtractedInfo
	if err := json.Unmarshal([]byte(contentText), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse extraction response: %w", err)
	}

	return &extracted, nil
}

// Summarize creates a summary of content
func (a *AnthropicAdapter) Summarize(ctx context.Context, content string) (string, error) {
	prompt := a.promptBuilder.BuildSummarizePrompt(content)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: prompt,
						},
					},
				},
			},
		},
		Model: anthropic.Model(a.model),
	})
	if err != nil {
		return "", fmt.Errorf("failed to call Anthropic: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}

	// Get text content
	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

// Query answers a question with context
func (a *AnthropicAdapter) Query(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
	prompt := a.promptBuilder.BuildQueryPrompt(question, contextPages)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: prompt,
						},
					},
				},
			},
		},
		Model: anthropic.Model(a.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no response from Anthropic")
	}

	// Get text content
	var answer string
	for _, block := range resp.Content {
		if block.Type == "text" {
			answer = block.Text
			break
		}
	}

	// TODO: Parse citations from response
	// For now, return empty citations
	return &schema.QueryAnswer{
		Answer:    answer,
		Citations: []schema.Citation{},
	}, nil
}

// Lint performs health analysis on pages
func (a *AnthropicAdapter) Lint(ctx context.Context, pages []*schema.Page) (*LintReport, error) {
	// Build page content for linting
	var content string
	for _, page := range pages {
		content += fmt.Sprintf("--- %s ---\n# %s\n%s\n\n", page.Path, page.Title, page.Content)
	}

	prompt := a.promptBuilder.BuildLintPrompt(content)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: prompt,
						},
					},
				},
			},
		},
		Model: anthropic.Model(a.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no response from Anthropic")
	}

	// Get text content
	contentText := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = block.Text
			break
		}
	}

	// Parse JSON response
	var report LintReport
	if err := json.Unmarshal([]byte(contentText), &report); err != nil {
		return nil, fmt.Errorf("failed to parse lint response: %w", err)
	}

	return &report, nil
}

// Consolidate merges related pages into a single coherent page using LLM
func (a *AnthropicAdapter) Consolidate(ctx context.Context, pages []*schema.Page) (*ConsolidateResult, error) {
	content := buildPagesText(pages)
	prompt := PromptConsolidate + "\n\nPages to consolidate:\n\n" + content

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: prompt,
						},
					},
				},
			},
		},
		Model: anthropic.Model(a.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic for consolidation: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("no response from Anthropic")
	}

	contentText := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			contentText = block.Text
			break
		}
	}

	var result ConsolidateResult
	if err := json.Unmarshal([]byte(contentText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse consolidation response: %w", err)
	}

	return &result, nil
}
