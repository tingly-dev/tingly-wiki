package wiki

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// LayerStrategy defines per-layer retrieval weight coefficients.
// Weights are intended to sum to ~1.0 per layer; values do not need to be
// renormalised when EntityWeight is zero.
type LayerStrategy struct {
	VectorWeight     float64 // cosine similarity weight
	KeywordWeight    float64 // full-text keyword weight
	RecencyWeight    float64 // time-decay weight (Generative Agents formula)
	ImportanceWeight float64 // stored importance score weight
	EntityWeight     float64 // entity-linking boost (mem0 style)
}

// defaultLayerStrategies maps PageType to its retrieval strategy.
// Strategies are designed around each layer's retrieval semantics:
//
//   - preference: semantic identity matching → pure vector
//   - memory:     episodic → vector + time decay + importance + light entity boost
//   - entity:     named-entity lookup → keyword-dominant + entity boost
//   - concept:    broad semantic ideas → balanced keyword + vector + entity boost
//   - synthesis:  summaries → vector + importance
//
// EntityWeight is dormant unless a non-noop LinkExtractor is wired into the
// retriever (see SetLinkExtractor). Without an extractor, entity scores stay
// at zero and the weighted term contributes nothing — preserving the prior
// behaviour for callers that have not opted in.
var defaultLayerStrategies = map[schema.PageType]LayerStrategy{
	schema.PageTypePreference: {VectorWeight: 1.0},
	schema.PageTypeMemory:     {VectorWeight: 0.35, KeywordWeight: 0.25, RecencyWeight: 0.2, ImportanceWeight: 0.1, EntityWeight: 0.1},
	schema.PageTypeEntity:     {KeywordWeight: 0.6, ImportanceWeight: 0.2, VectorWeight: 0.1, EntityWeight: 0.1},
	schema.PageTypeConcept:    {VectorWeight: 0.35, KeywordWeight: 0.35, ImportanceWeight: 0.2, EntityWeight: 0.1},
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
//
// An optional LinkExtractor (set via SetLinkExtractor) enables an entity-linking
// boost: entities mentioned in the query seed an additional keyword search whose
// hits contribute via LayerStrategy.EntityWeight. With no extractor wired, the
// entity term is identically zero and the retriever behaves as before.
type HybridRetriever struct {
	fulltext      index.Index
	vector        index.VectorIndex // may be nil
	scorer        *ImportanceScorer
	llm           llm.LLM
	strategies    map[schema.PageType]LayerStrategy
	linkExtractor LinkExtractor // optional; nil → entity boost dormant
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

// SetLinkExtractor wires a LinkExtractor into the retriever, activating the
// entity-linking boost path. Pass nil (or a NoopLinkExtractor) to disable.
func (h *HybridRetriever) SetLinkExtractor(le LinkExtractor) {
	if _, isNoop := le.(NoopLinkExtractor); isNoop {
		h.linkExtractor = nil
		return
	}
	h.linkExtractor = le
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
	kwList := newRankList(len(kwResult.Results))
	for _, r := range kwResult.Results {
		kwList.add(r.Page.Path, r.Score)
	}
	kwList.sortDesc()

	// ── Vector search (optional) ─────────────────────────────────────────────
	vecList := newRankList(0)
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
					vecList.add(r.Path, r.Score)
				}
				vecList.sortDesc()
			}
		}
	}

	// ── Entity-linking boost (optional) ──────────────────────────────────────
	// When a LinkExtractor is wired, mentions of entities in the query seed
	// additional keyword searches; the resulting paths form a third rank list
	// that participates in the RRF fusion via LayerStrategy.EntityWeight.
	entityList := newRankList(0)
	for path, score := range h.entityBoost(ctx, query, opts, overFetch) {
		entityList.add(path, score)
	}
	entityList.sortDesc()

	// ── Reciprocal Rank Fusion ───────────────────────────────────────────────
	// Each signal contributes 1/(rrfK + rank) per path. Per-layer weights
	// (Vector/Keyword/Entity) modulate the per-signal contribution so callers
	// can still bias toward keyword or vector retrieval. Ranks are stable
	// regardless of raw score magnitudes — fixing the linear-fusion scale
	// problem (BM25 ~ 1-10 vs. cosine ~ 0-1).
	rrfPerSignal := func(list *rankList) map[string]float64 {
		out := make(map[string]float64, len(list.entries))
		for rank, e := range list.entries {
			out[e.path] = 1.0 / float64(rrfK+rank+1)
		}
		return out
	}
	rrfKW := rrfPerSignal(kwList)
	rrfVec := rrfPerSignal(vecList)
	rrfEnt := rrfPerSignal(entityList)

	// ── Union candidate paths ────────────────────────────────────────────────
	seen := make(map[string]bool, len(rrfKW)+len(rrfVec)+len(rrfEnt))
	var paths []string
	for _, m := range []map[string]float64{rrfKW, rrfVec, rrfEnt} {
		for p := range m {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
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

		// Recency decay (Generative Agents: exp(-λ·days), λ=0.02)
		var recency float64
		if page.LastAccessedAt != nil {
			days := now.Sub(*page.LastAccessedAt).Hours() / 24
			recency = math.Exp(-0.02 * days)
		} else {
			recency = 1.0
		}

		// Search-signal contribution via RRF, modulated by layer weights.
		// Pages absent from a signal contribute 0 from that signal — there is
		// no need to redistribute weights, since RRF naturally absorbs missing
		// signals (rrf=0 for absent paths).
		searchTerm := strat.VectorWeight*rrfVec[path] +
			strat.KeywordWeight*rrfKW[path] +
			strat.EntityWeight*rrfEnt[path]

		composite := searchTerm +
			strat.RecencyWeight*recency +
			strat.ImportanceWeight*page.Importance

		scored = append(scored, &ScoredPage{Page: page, Score: composite})
	}

	// ── Sort ─────────────────────────────────────────────────────────────────
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// ── Optional cross-encoder rerank ────────────────────────────────────────
	// When opts.Rerank is set and the LLM satisfies llm.Reranker, rescore the
	// top-N candidates with the reranker. Rerank scores replace the composite
	// for that prefix; non-reranked entries keep their composites and are
	// pushed below by adding the maximum non-reranked composite as a floor to
	// the reranked scores. Rerank failures are silently ignored — the RRF
	// order is already a sensible fallback.
	scored = h.maybeRerank(ctx, query, scored, opts)

	// ── Truncate to limit ────────────────────────────────────────────────────
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// rrfK is the standard Reciprocal Rank Fusion constant. Smaller values give
// more weight to top ranks; 60 is the value commonly cited in IR literature.
const rrfK = 60

// defaultRerankCandidates is the number of top-RRF candidates rescored when
// opts.Rerank is set without a custom RerankCandidates count.
const defaultRerankCandidates = 20

type rankEntry struct {
	path  string
	score float64
}

// rankList is a small helper that keeps rank construction readable.
type rankList struct{ entries []rankEntry }

func newRankList(capHint int) *rankList { return &rankList{entries: make([]rankEntry, 0, capHint)} }

func (l *rankList) add(path string, score float64) {
	l.entries = append(l.entries, rankEntry{path: path, score: score})
}

// sortDesc orders entries by score descending; the resulting index becomes the
// rank consumed by RRF.
func (l *rankList) sortDesc() {
	sort.SliceStable(l.entries, func(i, j int) bool { return l.entries[i].score > l.entries[j].score })
}

// maybeRerank rescuses the top-N candidates with an LLM reranker when
// opts.Rerank is true and the configured LLM implements llm.Reranker. Returns
// scored unchanged on any failure.
func (h *HybridRetriever) maybeRerank(ctx context.Context, query string, scored []*ScoredPage, opts *RecallOptions) []*ScoredPage {
	if !opts.Rerank || len(scored) == 0 {
		return scored
	}
	rr, ok := h.llm.(llm.Reranker)
	if !ok {
		return scored
	}
	n := opts.RerankCandidates
	if n <= 0 {
		n = defaultRerankCandidates
	}
	if n > len(scored) {
		n = len(scored)
	}
	docs := make([]string, n)
	for i := 0; i < n; i++ {
		docs[i] = canonicalText(scored[i].Page)
	}
	rrScores, err := rr.Rerank(ctx, query, docs)
	if err != nil || len(rrScores) != n {
		return scored
	}

	// Compute floor so reranked entries sit above untouched ones.
	floor := 0.0
	if n < len(scored) {
		floor = scored[n].Score
	}
	for i := 0; i < n; i++ {
		scored[i].Score = floor + 1.0 + rrScores[i]
	}
	sort.Slice(scored[:n], func(i, j int) bool { return scored[i].Score > scored[j].Score })
	return scored
}

// strategyFor returns the strategy for pt, falling back to a keyword-heavy default.
func (h *HybridRetriever) strategyFor(pt schema.PageType) LayerStrategy {
	if s, ok := h.strategies[pt]; ok {
		return s
	}
	return LayerStrategy{KeywordWeight: 0.7, ImportanceWeight: 0.3}
}

// entityBoost returns per-path scores derived from entity mentions in the
// query. Each unique entity name (subject or object) found by the configured
// LinkExtractor seeds a keyword search; the best match-score per path is kept.
//
// Returns an empty map when no LinkExtractor is wired or when no entities are
// detected, preserving the prior retrieval semantics.
func (h *HybridRetriever) entityBoost(ctx context.Context, query string, opts *RecallOptions, limit int) map[string]float64 {
	scores := make(map[string]float64)
	if h.linkExtractor == nil {
		return scores
	}
	links, err := h.linkExtractor.Extract(ctx, query)
	if err != nil || len(links) == 0 {
		return scores
	}

	seen := make(map[string]bool, 2*len(links))
	names := make([]string, 0, 2*len(links))
	for _, l := range links {
		for _, n := range []string{l.Subject, l.Object} {
			if n == "" {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(n))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return scores
	}

	searchOpts := &index.SearchOptions{
		Limit:          limit,
		TenantID:       opts.TenantID,
		MinImportance:  opts.MinImportance,
		MemoryTier:     opts.MemoryTier,
		ExcludeExpired: true,
	}
	for _, name := range names {
		res, errS := h.fulltext.Search(ctx, name, searchOpts)
		if errS != nil || res == nil {
			continue
		}
		for _, r := range res.Results {
			if r.Score > scores[r.Page.Path] {
				scores[r.Page.Path] = r.Score
			}
		}
	}
	return scores
}
