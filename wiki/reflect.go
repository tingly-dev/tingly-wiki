package wiki

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// ReflectOptions controls a Reflect run.
//
// Reflect is a "sleep-cycle" operation: take a corpus of stored memories and
// ask the LLM to surface new insights as PageTypeSynthesis pages. It is the
// counterpart to ConsolidateMemories, which only merges similar pages — Reflect
// produces *new* derived knowledge across pages that may share no surface
// vocabulary.
type ReflectOptions struct {
	// TenantID restricts both inputs and the resulting synthesis pages to a
	// single namespace. Empty means "all tenants" — usually only sensible for
	// single-tenant deployments.
	TenantID string

	// Query, when non-empty, focuses the reflection on memories that match.
	// When empty, MaxInputs highest-importance memories are selected.
	Query string

	// Types limits the candidate pool. Defaults to PageTypeMemory and
	// PageTypePreference — the layers most likely to contain raw observations
	// worth synthesising. PageTypeSynthesis is intentionally excluded to keep
	// reflections grounded in raw memories.
	Types []schema.PageType

	// MaxInputs caps how many memories are fed to the reflector. 0 → 20.
	MaxInputs int

	// AgentID is recorded on each generated synthesis page.
	AgentID string

	// DryRun reports the planned synthesis titles without writing pages.
	DryRun bool
}

// ReflectResult summarises a Reflect run.
type ReflectResult struct {
	// SynthesisCreated lists paths of newly-written synthesis pages. In a
	// dry run, this list is empty even though Planned has entries.
	SynthesisCreated []string

	// Planned lists synthesis titles the reflector intended to create. This
	// is populated for both dry-run and real runs and is the primary signal
	// callers use to verify a run.
	Planned []string

	// InputsConsidered is how many source memories were sent to the LLM.
	InputsConsidered int

	// DryRun mirrors ReflectOptions.DryRun.
	DryRun bool
}

// Reflect surveys stored memories and asks the configured LLM (when it
// implements llm.Reflector) to derive new PageTypeSynthesis pages whose
// Sources field links back to the contributing inputs.
//
// Reflect is a no-op (no error) when the configured LLM does not implement
// llm.Reflector — adapters opt in by adding a Reflect method.
func (m *MemoryWikiImpl) Reflect(ctx context.Context, opts *ReflectOptions) (*ReflectResult, error) {
	if opts == nil {
		opts = &ReflectOptions{}
	}
	maxInputs := opts.MaxInputs
	if maxInputs <= 0 {
		maxInputs = 20
	}

	reflector, ok := m.llm.(llm.Reflector)
	if !ok {
		// Quietly succeed: callers can detect "did anything happen?" via
		// SynthesisCreated/Planned being empty.
		return &ReflectResult{DryRun: opts.DryRun}, nil
	}

	candidates, err := m.collectReflectInputs(ctx, opts, maxInputs)
	if err != nil {
		return nil, fmt.Errorf("Reflect: collect inputs: %w", err)
	}
	if len(candidates) == 0 {
		return &ReflectResult{DryRun: opts.DryRun}, nil
	}

	inputs := make([]llm.ReflectInput, len(candidates))
	for i, p := range candidates {
		inputs[i] = llm.ReflectInput{Path: p.Path, Title: p.Title, Content: p.Content}
	}

	syntheses, err := reflector.Reflect(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("Reflect: LLM: %w", err)
	}

	res := &ReflectResult{
		InputsConsidered: len(inputs),
		DryRun:           opts.DryRun,
	}
	for _, s := range syntheses {
		if s.Title == "" {
			continue
		}
		res.Planned = append(res.Planned, s.Title)
		if opts.DryRun {
			continue
		}
		path := m.layout.GetSynthesisPath(s.Title)
		if opts.TenantID != "" {
			path = "tenants/" + opts.TenantID + "/" + path
		}
		now := time.Now()
		page := &schema.Page{
			Path: path,
			Frontmatter: schema.Frontmatter{
				Type:       schema.PageTypeSynthesis,
				Title:      s.Title,
				Tags:       []string{"reflect"},
				Sources:    s.Sources,
				Importance: 0.6, // synthesis defaults to moderately important
				MemoryTier: schema.MemoryTierHot,
				TenantID:   opts.TenantID,
				AgentID:    opts.AgentID,
			},
			Content:   s.Content,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := m.storage.WritePage(ctx, page); err != nil {
			return nil, fmt.Errorf("Reflect: write %s: %w", path, err)
		}
		_ = m.index.Index(ctx, page)
		// Best-effort vector indexing
		if m.vector != nil {
			if vec, errE := m.embedder.Embed(ctx, canonicalText(page)); errE == nil && len(vec) > 0 {
				_ = m.vector.IndexVector(ctx, path, vec, nil)
			}
		}
		res.SynthesisCreated = append(res.SynthesisCreated, path)
	}
	return res, nil
}

// collectReflectInputs picks the source pages fed to the reflector, honoring
// opts.Query (relevance-ranked) when set, otherwise the most-important pages.
func (m *MemoryWikiImpl) collectReflectInputs(ctx context.Context, opts *ReflectOptions, maxInputs int) ([]*schema.Page, error) {
	types := opts.Types
	if len(types) == 0 {
		types = []schema.PageType{schema.PageTypeMemory, schema.PageTypePreference}
	}

	if opts.Query != "" {
		scored, err := m.retriever.Recall(ctx, opts.Query, &RecallOptions{
			Types:    types,
			TenantID: opts.TenantID,
			Limit:    maxInputs,
		}, m.storage)
		if err != nil {
			return nil, err
		}
		out := make([]*schema.Page, 0, len(scored))
		for _, s := range scored {
			out = append(out, s.Page)
		}
		return out, nil
	}

	// Fallback: scan candidate pages and pick the top-N by Importance.
	var all []*schema.Page
	for _, pt := range types {
		ptCopy := pt
		pages, err := m.storage.ListPages(ctx, &storage.ListOptions{Type: &ptCopy})
		if err != nil {
			return nil, err
		}
		for _, p := range pages {
			if opts.TenantID != "" && p.TenantID != opts.TenantID {
				continue
			}
			all = append(all, p)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Importance != all[j].Importance {
			return all[i].Importance > all[j].Importance
		}
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	if len(all) > maxInputs {
		all = all[:maxInputs]
	}
	return all, nil
}
