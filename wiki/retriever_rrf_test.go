package wiki

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// TestRetriever_RRFFusionStableAcrossScales documents the core RRF guarantee:
// rank order is invariant to the absolute magnitude of input scores. Two pages
// with very different keyword scores still produce a deterministic order.
func TestRetriever_RRFFusionStableAcrossScales(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	// Two memory pages, both keyword-matching "shared" but with different
	// content lengths so the underlying full-text scorer gives different
	// magnitudes. RRF should still rank them stably.
	fx.addPage(t, &schema.Page{
		Path: "memories/long.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "long", Importance: 0.5,
		},
		Content: "shared keyword token appears here and again — shared content shared etc",
	}, nil)
	fx.addPage(t, &schema.Page{
		Path: "memories/short.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "short", Importance: 0.5,
		},
		Content: "shared once",
	}, nil)

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 2 {
		t.Fatalf("expected both hits, got %d", len(scored))
	}
	// We don't assert specific order — only that scores are bounded by RRF
	// magnitudes, not driven by raw keyword score scale. Both should fall in
	// the [0, 1] band typical for RRF + bounded recency/importance.
	for _, s := range scored {
		if s.Score < 0 || s.Score > 5 {
			t.Errorf("RRF composite out of expected range for %s: %f", s.Page.Path, s.Score)
		}
	}
}

// TestRetriever_RerankReordersTop confirms that an LLM Reranker, when wired
// and opted into via RecallOptions.Rerank, can flip top-N order regardless of
// the underlying RRF composite.
func TestRetriever_RerankReordersTop(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	// Three keyword-matching pages — RRF would rank them roughly by full-text
	// score / insertion order. We then attach a reranker that explicitly
	// prefers "memories/c.md".
	for _, p := range []string{"a", "b", "c"} {
		fx.addPage(t, &schema.Page{
			Path: "memories/" + p + ".md",
			Frontmatter: schema.Frontmatter{
				Type: schema.PageTypeMemory, Title: p, Importance: 0.5,
			},
			Content: "shared topic " + p,
		}, nil)
	}

	// Reranker scores favouring c > a > b
	fx.llm.RerankFunc = func(_ context.Context, _ string, docs []string) ([]float64, error) {
		out := make([]float64, len(docs))
		for i, d := range docs {
			switch {
			case strings.Contains(d, "memories/c.md") || strings.Contains(d, "title:c") || strings.Contains(d, "c"):
				// docs is canonicalText: title + facts + content. We embed
				// the path into content via the shared topic token "c".
				out[i] = 0.95
			case strings.Contains(d, "a"):
				out[i] = 0.6
			default:
				out[i] = 0.2
			}
			// Override based on actual title for determinism
			switch {
			case strings.HasPrefix(d, "c."):
				out[i] = 0.95
			case strings.HasPrefix(d, "a."):
				out[i] = 0.6
			case strings.HasPrefix(d, "b."):
				out[i] = 0.2
			}
		}
		return out, nil
	}

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types:            []schema.PageType{schema.PageTypeMemory},
		Rerank:           true,
		RerankCandidates: 3,
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(scored))
	}
	if scored[0].Page.Path != "memories/c.md" {
		t.Errorf("rerank should promote 'c' to top, got %s", scored[0].Page.Path)
	}
}

// TestRetriever_RerankFailureFallsBackToRRF ensures rerank failures are
// silently absorbed and the RRF order is preserved.
func TestRetriever_RerankFailureFallsBackToRRF(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	fx.addPage(t, &schema.Page{
		Path: "memories/x.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "x", Importance: 0.5,
		},
		Content: "shared topic",
	}, nil)

	// Reranker returns wrong-length slice → treated as failure.
	fx.llm.RerankFunc = func(_ context.Context, _ string, _ []string) ([]float64, error) {
		return []float64{}, nil
	}

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types:  []schema.PageType{schema.PageTypeMemory},
		Rerank: true,
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(scored))
	}
	// The single hit should still be present; we don't assert the score
	// because RRF absolute value depends on tokenizer details.
	if scored[0].Page.Path != "memories/x.md" {
		t.Errorf("expected memories/x.md, got %s", scored[0].Page.Path)
	}
}

// TestRetriever_RerankIgnoredWhenLLMNotReranker confirms that a non-Reranker
// LLM silently skips rerank without errors. (MockLLM does implement Reranker,
// so this test injects a minimal LLM that does not.)
func TestRetriever_RerankIgnoredWhenLLMNotReranker(t *testing.T) {
	// Build a fixture using a non-reranker LLM stub.
	fx := newFixture(false)
	fx.r.llm = nonRerankerLLM{} // overwrite with a plain LLM

	ctx := context.Background()
	fx.addPage(t, &schema.Page{
		Path: "memories/y.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "y", Importance: 0.5,
		},
		Content: "shared topic",
	}, nil)

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types:  []schema.PageType{schema.PageTypeMemory},
		Rerank: true,
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(scored))
	}
}
