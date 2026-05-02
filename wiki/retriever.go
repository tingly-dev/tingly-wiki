package wiki

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// LayerStrategy defines per-layer retrieval weight coefficients.
// All four weights should sum to 1.0 for a given layer.
type LayerStrategy struct {
	VectorWeight     float64 // cosine similarity weight
	KeywordWeight    float64 // full-text keyword weight
	RecencyWeight    float64 // time-decay weight (Generative Agents formula)
	ImportanceWeight float64 // stored importance score weight
}

// defaultLayerStrategies maps PageType to its retrieval strategy.
// Strategies are designed around each layer's retrieval semantics:
//
//   - preference: semantic identity matching → pure vector
//   - memory:     episodic → vector + time decay + importance (Generative Agents)
//   - entity:     named-entity lookup → keyword-dominant + light vector
//   - concept:    broad semantic ideas → balanced keyword + vector
//   - synthesis:  summaries → vector + importance
var defaultLayerStrategies = map[schema.PageType]LayerStrategy{
	schema.PageTypePreference: {VectorWeight: 1.0},
	schema.PageTypeMemory:     {VectorWeight: 0.4, KeywordWeight: 0.3, RecencyWeight: 0.2, ImportanceWeight: 0.1},
	schema.PageTypeEntity:     {KeywordWeight: 0.7, ImportanceWeight: 0.2, VectorWeight: 0.1},
	schema.PageTypeConcept:    {VectorWeight: 0.4, KeywordWeight: 0.4, ImportanceWeight: 0.2},
	schema.PageTypeSynthesis:  {VectorWeight: 0.5, KeywordWeight: 0.3, ImportanceWeight: 0.2},
	// Procedure pages are retrieved by keyword (precise skill names) with
	// a vector component for situation-matching and importance for prioritization.
	schema.PageTypeProcedure: {KeywordWeight: 0.5, VectorWeight: 0.3, ImportanceWeight: 0.2},
}

// ScoredPage pairs a page with its composite recall score.
type ScoredPage struct {
	Page  *schema.Page
	Score float64
}

// HybridRetriever performs layer-aware hybrid retrieval over keyword and vector indexes.
// When vector is nil or embedding fails, it degrades gracefully to keyword-only mode.
type HybridRetriever struct {
	fulltext   index.Index
	vector     index.VectorIndex // may be nil
	scorer     *ImportanceScorer
	llm        llm.LLM
	strategies map[schema.PageType]LayerStrategy
}

// NewHybridRetriever creates a HybridRetriever.
// vec may be nil; in that case vector weight is redistributed to keyword weight.
func NewHybridRetriever(ft index.Index, vec index.VectorIndex, scorer *ImportanceScorer, l llm.LLM) *HybridRetriever {
	strats := make(map[schema.PageType]LayerStrategy, len(defaultLayerStrategies))
	for k, v := range defaultLayerStrategies {
		strats[k] = v
	}
	return &HybridRetriever{
		fulltext:   ft,
		vector:     vec,
		scorer:     scorer,
		llm:        l,
		strategies: strats,
	}
}

// Recall performs layer-aware hybrid retrieval and returns pages ranked by composite score.
func (h *HybridRetriever) Recall(ctx context.Context, query string, opts *RecallOptions, st storage.Storage) ([]*ScoredPage, error) {
	if opts == nil {
		opts = &RecallOptions{}
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 10
	}
	overFetch := limit * 3

	// ── Keyword search ──────────────────────────────────────────────────────
	searchOpts := &index.SearchOptions{
		Limit:          overFetch,
		TenantID:       opts.TenantID,
		MinImportance:  opts.MinImportance,
		MemoryTier:     opts.MemoryTier,
		ExcludeExpired: true,
	}
	if len(opts.Types) == 1 {
		searchOpts.Type = &opts.Types[0]
	}

	kwResult, err := h.fulltext.Search(ctx, query, searchOpts)
	if err != nil {
		return nil, err
	}
	kwScores := make(map[string]float64, len(kwResult.Results))
	for _, r := range kwResult.Results {
		kwScores[r.Page.Path] = r.Score
	}

	// ── Vector search (optional) ─────────────────────────────────────────────
	vecScores := make(map[string]float64)
	if h.vector != nil {
		qVec, embedErr := h.llm.Embed(ctx, query)
		if embedErr == nil && len(qVec) > 0 {
			vecOpts := &index.VectorSearchOptions{
				Limit:          overFetch,
				TenantID:       opts.TenantID,
				ExcludeExpired: true,
			}
			if len(opts.Types) > 0 {
				vecOpts.Types = opts.Types
			}
			vecResult, searchErr := h.vector.SearchVector(ctx, qVec, vecOpts)
			if searchErr == nil {
				for _, r := range vecResult.Results {
					vecScores[r.Path] = r.Score
				}
			}
		}
	}

	// ── Union candidate paths ────────────────────────────────────────────────
	seen := make(map[string]bool, len(kwScores)+len(vecScores))
	var paths []string
	for p := range kwScores {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	for p := range vecScores {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	// ── Load pages + compute composite score ─────────────────────────────────
	now := time.Now()
	var scored []*ScoredPage

	for _, path := range paths {
		page, readErr := st.ReadPage(ctx, path)
		if readErr != nil {
			continue
		}

		// Type filter when multiple types requested
		if len(opts.Types) > 0 && !containsType(opts.Types, page.Type) {
			continue
		}

		strat := h.strategyFor(page.Type)

		// If vector index unavailable or produced no score for this page,
		// redistribute vector weight to keyword weight.
		vScore := vecScores[path]
		kScore := kwScores[path]
		if h.vector == nil || vScore == 0 {
			strat.KeywordWeight += strat.VectorWeight
			strat.VectorWeight = 0
		}

		// Recency decay (Generative Agents: exp(-λ·days), λ=0.02)
		var recency float64
		if page.LastAccessedAt != nil {
			days := now.Sub(*page.LastAccessedAt).Hours() / 24
			recency = math.Exp(-0.02 * days)
		} else {
			recency = 1.0
		}

		composite := strat.VectorWeight*vScore +
			strat.KeywordWeight*kScore +
			strat.RecencyWeight*recency +
			strat.ImportanceWeight*page.Importance

		scored = append(scored, &ScoredPage{Page: page, Score: composite})
	}

	// ── Sort + truncate ───────────────────────────────────────────────────────
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// strategyFor returns the strategy for pt, falling back to a keyword-heavy default.
func (h *HybridRetriever) strategyFor(pt schema.PageType) LayerStrategy {
	if s, ok := h.strategies[pt]; ok {
		return s
	}
	return LayerStrategy{KeywordWeight: 0.7, ImportanceWeight: 0.3}
}
