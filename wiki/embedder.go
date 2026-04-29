package wiki

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/llm"
)

// Embedder converts text to a dense vector for semantic retrieval.
// Decoupling this from llm.LLM lets callers substitute a local model,
// a caching layer, or a mock without touching the LLM adapter.
type Embedder interface {
	// Embed returns the vector representation of text.
	// Returns a nil slice (not an error) when embedding is intentionally
	// skipped (e.g. NoopEmbedder); callers must guard len(vec) > 0.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dim returns the expected vector dimension (0 means unknown/noop).
	Dim() int
}

// LLMEmbedder delegates to an llm.LLM adapter's Embed method.
type LLMEmbedder struct {
	LLM llm.LLM
	dim int // cached after first call; 0 until first successful Embed
}

// NewLLMEmbedder creates an Embedder backed by the given LLM adapter.
func NewLLMEmbedder(l llm.LLM) *LLMEmbedder { return &LLMEmbedder{LLM: l} }

func (e *LLMEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec, err := e.LLM.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	if e.dim == 0 {
		e.dim = len(vec)
	}
	return vec, nil
}

func (e *LLMEmbedder) Dim() int { return e.dim }

// NoopEmbedder always returns a nil vector.
// Use it to disable semantic retrieval without removing vector index wiring.
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (NoopEmbedder) Dim() int                                              { return 0 }
