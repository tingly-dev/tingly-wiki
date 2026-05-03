package wiki

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// asOfTestFixture builds a MemoryWikiImpl wired to in-memory backends.
func asOfTestFixture(t *testing.T) *MemoryWikiImpl {
	t.Helper()
	cfg := &config.Config{
		Storage:     storage.NewMemoryStorage(),
		LLM:         llm.NewMockLLM(),
		Layout:      config.DefaultLayout(),
		VectorIndex: index.NewMemoryVectorIndex(),
	}
	mw, err := NewMemoryWiki(cfg)
	if err != nil {
		t.Fatalf("NewMemoryWiki: %v", err)
	}
	return mw
}

// TestRecallAsOf_BiTemporalSlice verifies that opts.AsOf returns the fact set
// that was valid at the given instant: a fact invalidated *after* asOf is
// still visible, and a fact whose EventTime is *after* asOf is hidden.
func TestRecallAsOf_BiTemporalSlice(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	// Manually craft a page with three facts so the test does not depend on
	// LLM extraction ordering.
	tInv := t2 // fact #1 invalidated at t2
	page := &schema.Page{
		Path: "memory/timeline.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypeMemory,
			Title:      "timeline",
			Importance: 0.9,
			MemoryTier: schema.MemoryTierHot,
			Facts: []schema.MemoryFact{
				{Subject: "user", Predicate: "prefers", Object: "tabs",
					EventTime: &t1, InvalidatedAt: &tInv},
				{Subject: "user", Predicate: "prefers", Object: "spaces",
					EventTime: &t2},
				{Subject: "user", Predicate: "lives_in", Object: "Mars",
					EventTime: &t3},
			},
		},
		Content:   "user formatting & location facts",
		CreatedAt: t1,
		UpdatedAt: t3,
	}
	if err := mw.storage.WritePage(ctx, page); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if err := mw.index.Index(ctx, page); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Query "user" — full-text matches all three subjects.
	cases := []struct {
		name      string
		asOf      *time.Time
		wantObjs  []string // sorted facts.Object values expected
	}{
		{
			name:     "now (default)",
			asOf:     nil,
			wantObjs: []string{"Mars", "spaces"}, // 'tabs' invalidated, default hides it
		},
		{
			name:     "asOf at t1",
			asOf:     ptr(t1),
			wantObjs: []string{"tabs"}, // only the original preference; 'spaces' & 'Mars' not yet known
		},
		{
			name:     "asOf at t2",
			asOf:     ptr(t2),
			wantObjs: []string{"spaces"}, // tabs just invalidated, Mars not yet
		},
		{
			name: "asOf in future",
			asOf: ptr(t3.Add(24 * time.Hour)),
			// All non-invalidated facts known by then are 'spaces' (still valid) and 'Mars'.
			// 'tabs' was invalidated at t2 < future, so it is excluded.
			wantObjs: []string{"Mars", "spaces"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := mw.RecallMemory(ctx, "user", &RecallOptions{
				Types: []schema.PageType{schema.PageTypeMemory},
				AsOf:  tc.asOf,
				Limit: 10,
			})
			if err != nil {
				t.Fatalf("RecallMemory: %v", err)
			}
			if len(res.Pages) == 0 {
				t.Fatalf("expected the timeline page in the recall result")
			}
			got := factObjects(res.Pages[0])
			if !equalStringSets(got, tc.wantObjs) {
				t.Errorf("AsOf=%v: got %v, want %v", tc.asOf, got, tc.wantObjs)
			}
		})
	}
}

// TestRecallAsOf_DefaultMatchesValidFacts ensures the AsOf=nil path is
// behaviorally identical to the old withValidFacts() path.
func TestRecallAsOf_DefaultMatchesValidFacts(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	now := time.Now()
	old := now.Add(-24 * time.Hour)
	page := &schema.Page{
		Path: "memory/d.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypeMemory,
			Title:      "default",
			Importance: 0.5,
			MemoryTier: schema.MemoryTierHot,
			Facts: []schema.MemoryFact{
				{Subject: "user", Predicate: "p", Object: "current"},
				{Subject: "user", Predicate: "p", Object: "stale", InvalidatedAt: &old},
			},
		},
		Content:   "default content",
		CreatedAt: old,
		UpdatedAt: now,
	}
	if err := mw.storage.WritePage(ctx, page); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if err := mw.index.Index(ctx, page); err != nil {
		t.Fatalf("index: %v", err)
	}

	res, err := mw.RecallMemory(ctx, "default content", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	})
	if err != nil {
		t.Fatalf("RecallMemory: %v", err)
	}
	if len(res.Pages) == 0 {
		t.Fatalf("expected the page in recall result")
	}
	objs := factObjects(res.Pages[0])
	if !equalStringSets(objs, []string{"current"}) {
		t.Errorf("default recall should hide invalidated facts, got %v", objs)
	}
}

// TestAssembleAsOf_ForwardsToFormatter checks that AssembleContext renders the
// fact slice that was valid at opts.AsOf.
func TestAssembleAsOf_ForwardsToFormatter(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	page := &schema.Page{
		Path: "memory/asm.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypeMemory,
			Title:      "asm",
			Importance: 0.9,
			MemoryTier: schema.MemoryTierHot,
			Facts: []schema.MemoryFact{
				{Subject: "user", Predicate: "prefers", Object: "v1",
					EventTime: &t1, InvalidatedAt: &t2},
				{Subject: "user", Predicate: "prefers", Object: "v2",
					EventTime: &t2},
			},
		},
		CreatedAt: t1,
		UpdatedAt: t2,
	}
	if err := mw.storage.WritePage(ctx, page); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	res, err := mw.AssembleContext(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypeMemory},
		AsOf:   ptr(t1),
	})
	if err != nil {
		t.Fatalf("AssembleContext: %v", err)
	}
	if !strings.Contains(res.Text, "v1") {
		t.Errorf("expected 'v1' in AsOf=t1 output, got: %s", res.Text)
	}
	if strings.Contains(res.Text, "v2") {
		t.Errorf("'v2' should not appear before its EventTime (t2): %s", res.Text)
	}
}

// ---- helpers ----

func ptr(t time.Time) *time.Time { return &t }

func factObjects(p *schema.Page) []string {
	out := make([]string, 0, len(p.Facts))
	for _, f := range p.Facts {
		out = append(out, f.Object)
	}
	return out
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		if m[s] == 0 {
			return false
		}
		m[s]--
	}
	return true
}
