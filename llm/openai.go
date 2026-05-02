package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// OpenAIAdapter wraps the OpenAI SDK
type OpenAIAdapter struct {
	client         openai.Client
	model          string
	embeddingModel string
	promptBuilder  *PromptBuilder
}

// OpenAIConfig configures the OpenAI adapter
type OpenAIConfig struct {
	// APIKey is the OpenAI API key (or set OPENAI_API_KEY env var)
	APIKey string

	// Model is the model to use (default: gpt-4o-mini)
	Model string

	// EmbeddingModel is the model for Embed calls (default: text-embedding-3-small)
	EmbeddingModel string

	// BaseURL is the base URL (optional, for custom endpoints)
	BaseURL string
}

// NewOpenAIAdapter creates a new OpenAI adapter
func NewOpenAIAdapter(cfg *OpenAIConfig) (*OpenAIAdapter, error) {
	if cfg == nil {
		cfg = &OpenAIConfig{}
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini" // Good balance of cost and performance
	}

	opts := []option.RequestOption{}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	embModel := cfg.EmbeddingModel

	return &OpenAIAdapter{
		client:         client,
		model:          model,
		embeddingModel: embModel,
		promptBuilder:  NewPromptBuilder(),
	}, nil
}

// Extract extracts structured information from content
func (o *OpenAIAdapter) Extract(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
	prompt := o.promptBuilder.BuildExtractPrompt(content)

	messages := []openai.ChatCompletionMessageParamUnion{
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			},
		},
	}

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(o.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse JSON response
	var extracted schema.ExtractedInfo
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse extraction response: %w", err)
	}

	return &extracted, nil
}

// Summarize creates a summary of content
func (o *OpenAIAdapter) Summarize(ctx context.Context, content string) (string, error) {
	prompt := o.promptBuilder.BuildSummarizePrompt(content)

	messages := []openai.ChatCompletionMessageParamUnion{
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			},
		},
	}

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(o.model),
	})
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return resp.Choices[0].Message.Content, nil
}

// Query answers a question with context
func (o *OpenAIAdapter) Query(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
	prompt := o.promptBuilder.BuildQueryPrompt(question, contextPages)

	messages := []openai.ChatCompletionMessageParamUnion{
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			},
		},
	}

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(o.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// TODO: Parse citations from response
	// For now, return empty citations
	return &schema.QueryAnswer{
		Answer:    resp.Choices[0].Message.Content,
		Citations: []schema.Citation{},
	}, nil
}

// Lint performs health analysis on pages
func (o *OpenAIAdapter) Lint(ctx context.Context, pages []*schema.Page) (*LintReport, error) {
	// Build page content for linting
	var content string
	for _, page := range pages {
		content += fmt.Sprintf("--- %s ---\n# %s\n%s\n\n", page.Path, page.Title, page.Content)
	}

	prompt := o.promptBuilder.BuildLintPrompt(content)

	messages := []openai.ChatCompletionMessageParamUnion{
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			},
		},
	}

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(o.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse JSON response
	var report LintReport
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &report); err != nil {
		return nil, fmt.Errorf("failed to parse lint response: %w", err)
	}

	return &report, nil
}

// Consolidate merges related pages into a single coherent page using LLM
func (o *OpenAIAdapter) Consolidate(ctx context.Context, pages []*schema.Page) (*ConsolidateResult, error) {
	content := buildPagesText(pages)
	prompt := PromptConsolidate + "\n\nPages to consolidate:\n\n" + content

	messages := []openai.ChatCompletionMessageParamUnion{
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			},
		},
	}

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(o.model),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI for consolidation: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	var result ConsolidateResult
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse consolidation response: %w", err)
	}

	return &result, nil
}

// Embed generates a dense vector for semantic retrieval using the OpenAI Embeddings API.
func (o *OpenAIAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := o.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
		Model: openai.EmbeddingModel(o.embeddingModel),
	})
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embed: empty response")
	}
	f64 := resp.Data[0].Embedding
	out := make([]float32, len(f64))
	for i, v := range f64 {
		out[i] = float32(v)
	}
	return out, nil
}

// ExtractMemoryFacts extracts atomic (subject, predicate, object) facts from content.
func (o *OpenAIAdapter) ExtractMemoryFacts(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error) {
	prompt := PromptExtractMemoryFacts + "\n\nContent (type: " + string(pageType) + "):\n\n" + content

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			}},
		},
		Model: openai.ChatModel(o.model),
	})
	if err != nil {
		return nil, fmt.Errorf("openai extract facts: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai extract facts: empty response")
	}

	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	// Strip markdown code fences if present
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var facts []schema.MemoryFact
	if err := json.Unmarshal([]byte(raw), &facts); err != nil {
		return nil, fmt.Errorf("openai extract facts: parse error: %w", err)
	}
	return facts, nil
}

// RateImportance asks the LLM how important content is for future recall (0.0–1.0).
func (o *OpenAIAdapter) RateImportance(ctx context.Context, content string) (float64, error) {
	prompt := PromptRateImportance + "\n\nContent:\n\n" + content

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(prompt),
				},
			}},
		},
		Model: openai.ChatModel(o.model),
	})
	if err != nil {
		return 0, fmt.Errorf("openai rate importance: %w", err)
	}
	if len(resp.Choices) == 0 {
		return 0, fmt.Errorf("openai rate importance: empty response")
	}

	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var result struct {
		Importance float64 `json:"importance"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return 0, fmt.Errorf("openai rate importance: parse error: %w", err)
	}
	return result.Importance, nil
}

// buildPagesText formats pages for LLM prompts
func buildPagesText(pages []*schema.Page) string {
	var out string
	for _, p := range pages {
		out += fmt.Sprintf("## %s\n%s\n\n", p.Title, p.Content)
	}
	return out
}
