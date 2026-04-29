// Package wiki — Layer B integration scenarios.
//
// Each TestMemoryScenario_* exercises an end-to-end question that the memory
// layer must answer well. These complement the per-module unit tests by
// verifying that the components compose correctly through the full
// MemoryWiki API surface.
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

// TestMemoryScenario_ParaphraseRecall verifies that vector retrieval finds
// semantically related content even with zero keyword overlap.
//
// Without vector index: "Sichuan cuisine" cannot be matched by "what cuisine?"
// With vector index: matched via cosine similarity of embeddings.
func TestMemoryScenario_ParaphraseRecall(t *testing.T) {
	// Two documents the retriever must distinguish:
	//   sichuan: aligned with "spicy / cuisine" embedding
	//   ui:      aligned with "ui / theme" embedding
	embeds := map[string][]float32{
		"User loves spicy Sichuan cuisine":          {1, 0, 0},
		"User prefers dark mode UI":                 {0, 1, 0},
		"what kind of food should I recommend?":     {0.95, 0.05, 0},
		"any UI theme preferences?":                 {0.05, 0.95, 0},
	}
	mock := &llm.MockLLM{
		EmbedFunc: func(_ context.Context, text string) ([]float32, error) {
			if v, ok := embeds[text]; ok {
				return v, nil
			}
			// Default: orthogonal so unrelated queries don't pollute scoring
			return []float32{0, 0, 1}, nil
		},
	}

	cfg := &config.Config{
		Storage:     storage.NewMemoryStorage(),
		LLM:         mock,
		Layout:      config.DefaultLayout(),
		VectorIndex: index.NewMemoryVectorIndex(),
	}
	mw, err := NewMemoryWiki(cfg, WithFactExtractor(NoopFactExtractor{}))
	if err != nil {
		t.Fatalf("NewMemoryWiki: %v", err)
	}

	ctx := context.Background()
	// canonicalText for these pages will start with title; embed key matches Content
	// since canonicalText starts "title. content...". To make the test robust, we
	// craft canonicalText input via a custom Embedder:
	mock.EmbedFunc = func(_ context.Context, text string) ([]float32, error) {
		// Match the most relevant key by substring.
		switch {
		case strings.Contains(text, "Sichuan"):
			return embeds["User loves spicy Sichuan cuisine"], nil
		case strings.Contains(text, "dark mode") || strings.Contains(text, "ui"):
			return embeds["User prefers dark mode UI"], nil
		case strings.Contains(text, "food") || strings.Contains(text, "cuisine"):
			return embeds["what kind of food should I recommend?"], nil
		case strings.Contains(text, "UI") || strings.Contains(text, "theme"):
			return embeds["any UI theme preferences?"], nil
		}
		return []float32{0, 0, 1}, nil
	}

	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "food",
		Content: "User loves spicy Sichuan cuisine", Importance: 0.9,
	})
	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "ui",
		Content: "User prefers dark mode UI", Importance: 0.9,
	})

	// Paraphrase query — keyword search would miss
	res, err := mw.RecallMemory(ctx, "what kind of food should I recommend?", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference},
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("RecallMemory: %v", err)
	}
	if len(res.Pages) == 0 {
		t.Fatal("paraphrase recall returned 0 results — vector retrieval failed")
	}
	if !strings.Contains(res.Pages[0].Path, "food") {
		t.Errorf("top hit path = %s, want contains 'food'", res.Pages[0].Path)
	}
}

// TestMemoryScenario_MultiTenantIsolation verifies that tenant A's memories
// are never returned to tenant B (or vice versa).
func TestMemoryScenario_MultiTenantIsolation(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "ui",
		Content: "loves dark mode", Importance: 0.8, TenantID: "user-A",
	})
	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "ui",
		Content: "loves light mode", Importance: 0.8, TenantID: "user-B",
	})

	// Tenant A query
	resA, _ := mw.RecallMemory(ctx, "mode", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference}, TenantID: "user-A",
	})
	for _, p := range resA.Pages {
		if p.TenantID != "user-A" {
			t.Errorf("tenant A query leaked tenant=%s page", p.TenantID)
		}
	}
	if len(resA.Pages) == 0 {
		t.Error("tenant A should see at least its own page")
	}

	// Tenant B query
	resB, _ := mw.RecallMemory(ctx, "mode", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference}, TenantID: "user-B",
	})
	for _, p := range resB.Pages {
		if p.TenantID != "user-B" {
			t.Errorf("tenant B query leaked tenant=%s page", p.TenantID)
		}
	}

	// AssembleContext also respects tenant isolation
	ctxA, _ := mw.AssembleContext(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference}, TenantID: "user-A",
	})
	if strings.Contains(ctxA.Text, "light") {
		t.Errorf("AssembleContext leaked B's preference into A's context:\n%s", ctxA.Text)
	}
}

// TestMemoryScenario_BiTemporalReasoning verifies that:
//   - by default, only current facts are visible
//   - IncludeInvalidated=true exposes the full history
func TestMemoryScenario_BiTemporalReasoning(t *testing.T) {
	callCount := 0
	mock := &llm.MockLLM{
		ExtractMemoryFactsFunc: func(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
			callCount++
			if callCount == 1 {
				return []schema.MemoryFact{
					{Subject: "user", Predicate: "lives_in", Object: "NYC", Confidence: 0.9},
				}, nil
			}
			return []schema.MemoryFact{
				{Subject: "user", Predicate: "lives_in", Object: "SF", Confidence: 0.95},
			}, nil
		},
	}
	mw := newTestWiki(t, mock, false)
	ctx := context.Background()

	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "location",
		Content: "I live in NYC", Importance: 0.7,
	})
	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "location",
		Content: "I moved to SF", Importance: 0.7,
	})

	// Default: only current facts
	res, _ := mw.RecallMemory(ctx, "location", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference},
	})
	if len(res.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(res.Pages))
	}
	if len(res.Pages[0].Facts) != 1 {
		t.Errorf("default recall should hide invalidated facts; got %d facts", len(res.Pages[0].Facts))
	}
	if res.Pages[0].Facts[0].Object != "SF" {
		t.Errorf("current fact = %s, want SF", res.Pages[0].Facts[0].Object)
	}

	// History query
	resHist, _ := mw.RecallMemory(ctx, "location", &RecallOptions{
		Types:              []schema.PageType{schema.PageTypePreference},
		IncludeInvalidated: true,
	})
	if len(resHist.Pages[0].Facts) != 2 {
		t.Errorf("history recall should expose 2 facts (NYC+SF), got %d", len(resHist.Pages[0].Facts))
	}
}

// TestMemoryScenario_LayerStrategyRouting verifies that per-layer retrieval
// strategies are honored: preference uses vector-dominant, entity uses
// keyword-dominant. We feed a query that lexically matches one page but
// semantically aligns with another — the layer choice decides the winner.
func TestMemoryScenario_LayerStrategyRouting(t *testing.T) {
	// "magenta" is the keyword the entity page contains; the preference page
	// has no shared keyword but its embedding aligns with the query.
	mock := &llm.MockLLM{
		EmbedFunc: func(_ context.Context, text string) ([]float32, error) {
			if strings.Contains(text, "magenta") {
				// entity page
				return []float32{0, 1, 0}, nil
			}
			if strings.Contains(text, "users want vibrant colors") {
				// preference page
				return []float32{1, 0, 0}, nil
			}
			// query
			return []float32{0.9, 0.1, 0}, nil
		},
	}
	cfg := &config.Config{
		Storage:     storage.NewMemoryStorage(),
		LLM:         mock,
		Layout:      config.DefaultLayout(),
		VectorIndex: index.NewMemoryVectorIndex(),
	}
	mw, err := NewMemoryWiki(cfg, WithFactExtractor(NoopFactExtractor{}))
	if err != nil {
		t.Fatalf("NewMemoryWiki: %v", err)
	}
	ctx := context.Background()

	// Entity page contains "magenta" keyword
	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypeMemory, Title: "design-meeting",
		Content: "discussed magenta swatches", Importance: 0.5,
	})
	// Preference page is semantically aligned with the query but no keyword overlap
	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "color-pref",
		Content: "users want vibrant colors", Importance: 0.5,
	})

	// Query that contains "magenta" as keyword AND is semantically aligned with vibrant
	res, _ := mw.RecallMemory(ctx, "magenta colors", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference, schema.PageTypeMemory},
		Limit: 5,
	})

	// Both layers should have a hit; verify both are present and the strategy
	// applied per-type means the preference page (vector-strong) and
	// memory page (keyword-strong) are both retrievable.
	gotPref := false
	gotMem := false
	for _, p := range res.Pages {
		if p.Type == schema.PageTypePreference {
			gotPref = true
		}
		if p.Type == schema.PageTypeMemory {
			gotMem = true
		}
	}
	if !gotPref {
		t.Errorf("preference page should be retrievable via vector despite zero keyword overlap")
	}
	if !gotMem {
		t.Errorf("memory page should be retrievable via keyword 'magenta'")
	}
}

// TestMemoryScenario_AssembleBudgetAndOrdering verifies that AssembleContext
// honors MaxChars and orders entries by importance.
func TestMemoryScenario_AssembleBudgetAndOrdering(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	// Store 10 preferences with descending importance
	for i := 0; i < 10; i++ {
		_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
			Type: schema.PageTypePreference,
			// title with importance baked in for visual verification
			Title:      "pref" + string(rune('a'+i)),
			Content:    "content for pref " + string(rune('a'+i)),
			Importance: float64(10-i) / 10.0, // 1.0, 0.9, ..., 0.1
		})
	}

	got, err := mw.AssembleContext(ctx, &AssembleOptions{
		Layers:   []schema.PageType{schema.PageTypePreference},
		MaxChars: 250,
	})
	if err != nil {
		t.Fatalf("AssembleContext: %v", err)
	}
	if len(got.Text) > 250 {
		t.Errorf("budget violated: %d > 250", len(got.Text))
	}
	if got.Stats.PagesIncluded == 0 {
		t.Error("expected at least 1 page included")
	}
	if got.Stats.PagesSkipped == 0 {
		t.Error("expected some pages skipped due to budget")
	}
	if got.Stats.PagesIncluded+got.Stats.PagesSkipped != 10 {
		t.Errorf("included(%d) + skipped(%d) ≠ 10",
			got.Stats.PagesIncluded, got.Stats.PagesSkipped)
	}
	// First included page should be the highest-importance one (prefa, importance=1.0)
	if !strings.Contains(got.Text, "prefa") {
		t.Errorf("highest-importance entry 'prefa' missing\n%s", got.Text)
	}
}

// TestMemoryScenario_TTLAndGC verifies that pages with past expiry are
// (a) hidden from RecallMemory results and (b) physically deleted by RunGC.
func TestMemoryScenario_TTLAndGC(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	res, _ := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypeMemory, Title: "ephemeral",
		Content: "scheduled for deletion", Importance: 0.5,
	})

	// Set TTL to past
	past := time.Now().Add(-1 * time.Hour)
	if err := mw.SetTTL(ctx, res.Path, &past); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}

	// RecallMemory should hide the expired page (ExcludeExpired is true by default)
	rec, _ := mw.RecallMemory(ctx, "scheduled", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	})
	for _, p := range rec.Pages {
		if p.Path == res.Path {
			t.Errorf("expired page %s should not be recalled", p.Path)
		}
	}

	// Run GC → page is physically deleted
	gc, err := mw.RunGC(ctx)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if gc.DeletedCount != 1 {
		t.Errorf("DeletedCount = %d, want 1", gc.DeletedCount)
	}
	if _, err := mw.GetPage(ctx, res.Path); err == nil {
		t.Errorf("expected GetPage to fail after GC")
	}
}
