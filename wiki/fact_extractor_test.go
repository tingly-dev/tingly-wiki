package wiki

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
)

func TestNoopFactExtractor(t *testing.T) {
	fe := NoopFactExtractor{}
	facts, err := fe.Extract(context.Background(), "any content", schema.PageTypePreference)
	if err != nil {
		t.Errorf("NoopFactExtractor.Extract should not error, got: %v", err)
	}
	if facts != nil {
		t.Errorf("NoopFactExtractor.Extract should return nil, got %v", facts)
	}
}

func TestLLMFactExtractor_Delegates(t *testing.T) {
	gotPageType := schema.PageType("")
	gotContent := ""
	mock := &llm.MockLLM{
		ExtractMemoryFactsFunc: func(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error) {
			gotPageType = pageType
			gotContent = content
			return []schema.MemoryFact{
				{Subject: "user", Predicate: "prefers", Object: "dark mode", Confidence: 0.9},
			}, nil
		},
	}
	fe := NewLLMFactExtractor(mock)
	facts, err := fe.Extract(context.Background(), "user wants dark theme", schema.PageTypePreference)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if gotPageType != schema.PageTypePreference {
		t.Errorf("page type = %s, want %s", gotPageType, schema.PageTypePreference)
	}
	if gotContent != "user wants dark theme" {
		t.Errorf("content = %q, not forwarded", gotContent)
	}
	if len(facts) != 1 || facts[0].Object != "dark mode" {
		t.Errorf("facts = %v, want one fact with Object=dark mode", facts)
	}
}

func TestLLMFactExtractor_ErrorPassthrough(t *testing.T) {
	wantErr := errors.New("llm-down")
	mock := &llm.MockLLM{
		ExtractMemoryFactsFunc: func(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error) {
			return nil, wantErr
		},
	}
	fe := NewLLMFactExtractor(mock)
	_, err := fe.Extract(context.Background(), "anything", schema.PageTypeMemory)
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}
