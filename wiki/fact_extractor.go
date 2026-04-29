package wiki

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// FactExtractor extracts atomic (subject, predicate, object) facts from content.
// Implementations can be LLM-based, rule-based, or a no-op.
// Callers inject the implementation that suits their cost/quality trade-off.
type FactExtractor interface {
	// Extract returns a (best-effort) list of MemoryFacts parsed from content.
	// pageType is a hint that allows implementations to calibrate extraction style
	// (e.g., preference pages warrant different predicates than entity pages).
	// Errors are non-fatal; callers should treat an empty slice as acceptable.
	Extract(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error)
}

// NoopFactExtractor always returns an empty fact list.
// Default when the caller does not supply a FactExtractor; preserves
// the original page-level granularity without any LLM call.
type NoopFactExtractor struct{}

func (NoopFactExtractor) Extract(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
	return nil, nil
}

// LLMFactExtractor delegates to llm.LLM.ExtractMemoryFacts.
type LLMFactExtractor struct {
	LLM llm.LLM
}

// NewLLMFactExtractor creates a FactExtractor backed by the given LLM adapter.
func NewLLMFactExtractor(l llm.LLM) *LLMFactExtractor { return &LLMFactExtractor{LLM: l} }

func (e *LLMFactExtractor) Extract(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error) {
	return e.LLM.ExtractMemoryFacts(ctx, content, pageType)
}
