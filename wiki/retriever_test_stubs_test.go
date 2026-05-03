package wiki

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// nonRerankerLLM is a minimal llm.LLM that intentionally does NOT implement
// llm.Reranker, used to exercise the retriever's "skip rerank when LLM cannot
// rerank" branch.
type nonRerankerLLM struct{}

func (nonRerankerLLM) Extract(_ context.Context, _ string, _ *schema.Schema) (*schema.ExtractedInfo, error) {
	return &schema.ExtractedInfo{}, nil
}
func (nonRerankerLLM) Summarize(_ context.Context, _ string) (string, error) { return "", nil }
func (nonRerankerLLM) Query(_ context.Context, _ string, _ []string) (*schema.QueryAnswer, error) {
	return &schema.QueryAnswer{}, nil
}
func (nonRerankerLLM) Lint(_ context.Context, _ []*schema.Page) (*llm.LintReport, error) {
	return &llm.LintReport{}, nil
}
func (nonRerankerLLM) Consolidate(_ context.Context, _ []*schema.Page) (*llm.ConsolidateResult, error) {
	return &llm.ConsolidateResult{}, nil
}
func (nonRerankerLLM) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, llm.ErrEmbeddingNotSupported
}
func (nonRerankerLLM) ExtractMemoryFacts(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
	return nil, nil
}
func (nonRerankerLLM) RateImportance(_ context.Context, _ string) (float64, error) {
	return 0.5, nil
}
