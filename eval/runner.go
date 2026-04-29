package eval

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
	"github.com/tingly-dev/tingly-wiki/wiki"
)

// Runner executes scenarios against a fresh MemoryWiki and produces metrics.
//
// The same Runner can be used to run multiple scenarios sequentially.
// Each scenario gets its own isolated MemoryWiki instance; state never
// leaks across scenarios.
type Runner struct {
	// LLM is the language model adapter. If nil, DeterministicMockLLM is used,
	// which produces stable hash-based embeddings (suitable for CI).
	LLM llm.LLM
}

// NewRunner returns a Runner that uses a deterministic mock LLM by default.
// For production-quality evaluation, supply a real LLM via the LLM field.
func NewRunner() *Runner { return &Runner{LLM: NewDeterministicMockLLM()} }

// Run executes a single scenario end-to-end and returns its aggregated metrics.
func (r *Runner) Run(ctx context.Context, sc *Scenario) (*ScenarioMetric, error) {
	mw, err := r.buildWiki(sc)
	if err != nil {
		return nil, fmt.Errorf("scenario %s: build wiki: %w", sc.Name, err)
	}
	defer mw.Close()

	if err := r.runSetup(ctx, mw, sc.Setup); err != nil {
		return nil, fmt.Errorf("scenario %s: setup: %w", sc.Name, err)
	}

	qms := make([]QueryMetric, 0, len(sc.Queries))
	for _, q := range sc.Queries {
		qm, err := r.runQuery(ctx, mw, q)
		if err != nil {
			return nil, fmt.Errorf("scenario %s: query %q: %w", sc.Name, q.Query, err)
		}
		qms = append(qms, qm)
	}

	out := aggregateScenarioMetric(sc.Name, qms)
	return &out, nil
}

// RunAll executes scenarios sequentially.
func (r *Runner) RunAll(ctx context.Context, scs []*Scenario) ([]*ScenarioMetric, error) {
	out := make([]*ScenarioMetric, 0, len(scs))
	for _, sc := range scs {
		m, err := r.Run(ctx, sc)
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Runner) buildWiki(sc *Scenario) (*wiki.MemoryWikiImpl, error) {
	llmAdapter := r.LLM
	if llmAdapter == nil {
		llmAdapter = NewDeterministicMockLLM()
	}

	cfg := &config.Config{
		Storage: storage.NewMemoryStorage(),
		LLM:     llmAdapter,
		Layout:  config.DefaultLayout(),
	}
	if sc.Config.UseVector {
		cfg.VectorIndex = index.NewMemoryVectorIndex()
	}

	opts := []wiki.MemoryWikiOption{
		// Avoid writing fact rows — scenarios assert on path-level retrieval,
		// not fact-level. Callers that want fact extraction enabled should
		// supply their own runner with WithFactExtractor in a custom build.
		wiki.WithFactExtractor(wiki.NoopFactExtractor{}),
	}
	if sc.Config.UseLinkExtractor {
		opts = append(opts, wiki.WithLinkExtractor(
			wiki.NewRegexLinkExtractor(wiki.DefaultLinkPatterns()),
		))
	}
	return wiki.NewMemoryWiki(cfg, opts...)
}

func (r *Runner) runSetup(ctx context.Context, mw *wiki.MemoryWikiImpl, ops []SetupOp) error {
	for _, op := range ops {
		opName := op.Op
		if opName == "" {
			opName = "store"
		}
		switch opName {
		case "store":
			ttl, err := ParseDuration(op.TTL)
			if err != nil {
				return fmt.Errorf("invalid TTL %q: %w", op.TTL, err)
			}
			req := &wiki.StoreMemoryRequest{
				Type:       schema.PageType(op.Type),
				Title:      op.Title,
				Content:    op.Content,
				Tags:       op.Tags,
				Importance: op.Importance,
				TTL:        ttl,
				TenantID:   op.Tenant,
				AgentID:    op.Agent,
			}
			if _, err := mw.StoreMemory(ctx, req); err != nil {
				return err
			}
		case "set_ttl":
			ttl, err := ParseDuration(op.TTL)
			if err != nil {
				return err
			}
			var deadline *time.Time
			if ttl != nil {
				t := time.Now().Add(*ttl)
				deadline = &t
			}
			if err := mw.SetTTL(ctx, op.Path, deadline); err != nil {
				return err
			}
		case "set_importance":
			if err := mw.SetImportance(ctx, op.Path, op.Score); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) runQuery(ctx context.Context, mw *wiki.MemoryWikiImpl, q Query) (QueryMetric, error) {
	types := make([]schema.PageType, len(q.Types))
	for i, t := range q.Types {
		types[i] = schema.PageType(t)
	}
	k := q.K
	if k == 0 {
		k = 5
	}
	opts := &wiki.RecallOptions{
		Types:              types,
		TenantID:           q.Tenant,
		MinImportance:      q.MinImportance,
		Limit:              k,
		IncludeInvalidated: q.IncludeInvalidated,
	}

	t0 := time.Now()
	res, err := mw.RecallMemory(ctx, q.Query, opts)
	latency := time.Since(t0)
	if err != nil {
		return QueryMetric{}, err
	}

	returned := make([]string, len(res.Pages))
	for i, p := range res.Pages {
		returned[i] = p.Path
	}
	return computeQueryMetric(q.Query, returned, q.RelevantPaths, k, latency), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// DeterministicMockLLM: stable mock for reproducible scenario evaluation.
//
// It hashes input text into a fixed-dimension embedding, so semantically
// related strings (sharing tokens) produce vectors that are similar under
// cosine. Not as good as a real embedder, but sufficient for verifying the
// retrieval pipeline integrates correctly and produces non-trivial signals.
// ─────────────────────────────────────────────────────────────────────────────

const detEmbedDim = 64

// DeterministicMockLLM is an llm.LLM implementation that produces stable,
// content-based embeddings without external API calls.
type DeterministicMockLLM struct {
	llm.MockLLM // delegate non-Embed methods to the standard mock
}

// NewDeterministicMockLLM returns a DeterministicMockLLM.
func NewDeterministicMockLLM() *DeterministicMockLLM {
	return &DeterministicMockLLM{}
}

// Embed produces a 64-dim vector by hashing each token (whitespace-split,
// lowercased) and accumulating into the corresponding bucket. Cosine similarity
// of these vectors approximates token-overlap similarity.
func (m *DeterministicMockLLM) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, detEmbedDim)
	for _, tok := range strings.Fields(strings.ToLower(text)) {
		// Strip simple punctuation.
		tok = strings.Trim(tok, `.,!?;:"'()[]{}`)
		if tok == "" {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		idx := int(h.Sum32()) % detEmbedDim
		if idx < 0 {
			idx += detEmbedDim
		}
		vec[idx] += 1.0
	}
	// L2 normalise so cosine = dot
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec, nil
	}
	scale := float32(1.0 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= scale
	}
	return vec, nil
}
