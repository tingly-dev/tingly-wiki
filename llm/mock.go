package llm

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// MockLLM is a mock implementation for testing
type MockLLM struct {
	// ExtractFunc is the mock extract function
	ExtractFunc func(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error)

	// SummarizeFunc is the mock summarize function
	SummarizeFunc func(ctx context.Context, content string) (string, error)

	// QueryFunc is the mock query function
	QueryFunc func(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error)

	// LintFunc is the mock lint function
	LintFunc func(ctx context.Context, pages []*schema.Page) (*LintReport, error)

	// ConsolidateFunc is the mock consolidate function
	ConsolidateFunc func(ctx context.Context, pages []*schema.Page) (*ConsolidateResult, error)

	// EmbedFunc is the mock embed function
	EmbedFunc func(ctx context.Context, text string) ([]float32, error)

	// ExtractMemoryFactsFunc is the mock fact extraction function
	ExtractMemoryFactsFunc func(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error)

	// RateImportanceFunc is the mock importance rating function
	RateImportanceFunc func(ctx context.Context, content string) (float64, error)
}

// NewMockLLM creates a new mock LLM with default behavior
func NewMockLLM() *MockLLM {
	return &MockLLM{}
}

// Extract calls the mock extract function
func (m *MockLLM) Extract(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
	if m.ExtractFunc != nil {
		return m.ExtractFunc(ctx, content, schemaDef)
	}

	// Default mock implementation
	return &schema.ExtractedInfo{
		Summary: "Mock summary of content",
		Entities: []schema.Entity{
			{Name: "MockEntity", Type: "organization", Description: "A mock entity"},
		},
		Concepts: []schema.Concept{
			{Name: "MockConcept", Description: "A mock concept"},
		},
		KeyPoints: []string{"Mock key point 1", "Mock key point 2"},
	}, nil
}

// Summarize calls the mock summarize function
func (m *MockLLM) Summarize(ctx context.Context, content string) (string, error) {
	if m.SummarizeFunc != nil {
		return m.SummarizeFunc(ctx, content)
	}

	// Default mock implementation
	return "Mock summary", nil
}

// Query calls the mock query function
func (m *MockLLM) Query(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, question, contextPages)
	}

	// Default mock implementation
	return &schema.QueryAnswer{
		Answer: "Mock answer to: " + question,
		Citations: []schema.Citation{
			{
				Path:      "mock/page.md",
				Title:     "Mock Page",
				Relevance: 1.0,
			},
		},
	}, nil
}

// Lint calls the mock lint function
func (m *MockLLM) Lint(ctx context.Context, pages []*schema.Page) (*LintReport, error) {
	if m.LintFunc != nil {
		return m.LintFunc(ctx, pages)
	}

	return &LintReport{
		Issues: []LintIssue{
			{
				Type:     LintIssueTypeOrphan,
				Severity: LintSeverityInfo,
				Message:  "No issues found (mock)",
			},
		},
	}, nil
}

// Consolidate calls the mock consolidate function
func (m *MockLLM) Consolidate(ctx context.Context, pages []*schema.Page) (*ConsolidateResult, error) {
	if m.ConsolidateFunc != nil {
		return m.ConsolidateFunc(ctx, pages)
	}

	title := "Consolidated"
	if len(pages) > 0 {
		title = pages[0].Title
	}
	return &ConsolidateResult{
		MergedContent:   "Mock consolidated content",
		SuggestedTitle:  title,
		ImportanceScore: 0.6,
	}, nil
}

// Embed calls the mock embed function; defaults to a deterministic 4-dim stub.
func (m *MockLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, text)
	}
	// Stable stub: use first 4 chars of text as a tiny "embedding"
	vec := make([]float32, 4)
	for i, r := range []rune(text) {
		if i >= 4 {
			break
		}
		vec[i] = float32(r) / 65536.0
	}
	return vec, nil
}

// ExtractMemoryFacts calls the mock fact extraction function.
func (m *MockLLM) ExtractMemoryFacts(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error) {
	if m.ExtractMemoryFactsFunc != nil {
		return m.ExtractMemoryFactsFunc(ctx, content, pageType)
	}
	return []schema.MemoryFact{
		{Subject: "user", Predicate: "noted", Object: content, Confidence: 0.7},
	}, nil
}

// RateImportance calls the mock importance rating function.
func (m *MockLLM) RateImportance(ctx context.Context, content string) (float64, error) {
	if m.RateImportanceFunc != nil {
		return m.RateImportanceFunc(ctx, content)
	}
	return 0.5, nil
}
