package wiki

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// newTestWiki builds a MemoryWikiImpl with mock LLM, in-memory storage,
// and optional vector index. Caller-supplied options override defaults.
func newTestWiki(t *testing.T, mock *llm.MockLLM, withVector bool, opts ...MemoryWikiOption) *MemoryWikiImpl {
	t.Helper()
	if mock == nil {
		mock = &llm.MockLLM{}
	}
	cfg := &config.Config{
		Storage: storage.NewMemoryStorage(),
		LLM:     mock,
		Layout:  config.DefaultLayout(),
	}
	if withVector {
		cfg.VectorIndex = index.NewMemoryVectorIndex()
	}
	mw, err := NewMemoryWiki(cfg, opts...)
	if err != nil {
		t.Fatalf("NewMemoryWiki: %v", err)
	}
	return mw
}

func TestMemoryImpl_StoreMemory_BasicCreate(t *testing.T) {
	mw := newTestWiki(t, &llm.MockLLM{
		ExtractMemoryFactsFunc: func(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
			return nil, nil
		},
	}, false, WithFactExtractor(NoopFactExtractor{}))

	ctx := context.Background()
	res, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "first-note",
		Content:    "anything",
		Importance: 0.7,
	})
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}
	if !res.Created {
		t.Errorf("expected Created=true on first store")
	}
	if res.Path != "memories/first-note.md" {
		t.Errorf("path = %s, want memories/first-note.md", res.Path)
	}

	page, err := mw.GetPage(ctx, res.Path)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page.Importance != 0.7 {
		t.Errorf("Importance = %f, want 0.7", page.Importance)
	}
	if page.MemoryTier != schema.MemoryTierHot {
		t.Errorf("MemoryTier = %s, want hot", page.MemoryTier)
	}
}

func TestMemoryImpl_StoreMemory_DefaultImportanceFromLLM(t *testing.T) {
	rated := false
	mock := &llm.MockLLM{
		RateImportanceFunc: func(_ context.Context, _ string) (float64, error) {
			rated = true
			return 0.83, nil
		},
	}
	mw := newTestWiki(t, mock, false, WithFactExtractor(NoopFactExtractor{}))

	res, err := mw.StoreMemory(context.Background(), &StoreMemoryRequest{
		Type:    schema.PageTypePreference,
		Title:   "x",
		Content: "y",
		// Importance left at 0 → should call LLM.RateImportance
	})
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}
	if !rated {
		t.Error("expected LLM.RateImportance to be invoked when Importance=0")
	}
	page, _ := mw.GetPage(context.Background(), res.Path)
	if page.Importance != 0.83 {
		t.Errorf("Importance = %f, want 0.83 (from LLM)", page.Importance)
	}
}

func TestMemoryImpl_StoreMemory_BiTemporalConflict(t *testing.T) {
	// First store: user lives_in NYC. Second store: user lives_in SF.
	// Expect: NYC fact gets InvalidatedAt set; SF is current.
	callCount := 0
	mock := &llm.MockLLM{
		ExtractMemoryFactsFunc: func(_ context.Context, content string, _ schema.PageType) ([]schema.MemoryFact, error) {
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
	_, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:    schema.PageTypePreference,
		Title:   "location",
		Content: "I live in NYC",
	})
	if err != nil {
		t.Fatalf("first StoreMemory: %v", err)
	}

	res, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:    schema.PageTypePreference,
		Title:   "location", // same title → update
		Content: "I moved to SF",
	})
	if err != nil {
		t.Fatalf("second StoreMemory: %v", err)
	}
	if res.Created {
		t.Errorf("expected Created=false on update")
	}

	page, _ := mw.GetPage(ctx, res.Path)

	if len(page.Facts) < 2 {
		t.Fatalf("expected ≥ 2 facts (old + new), got %d", len(page.Facts))
	}

	var nycFact, sfFact *schema.MemoryFact
	for i, f := range page.Facts {
		switch f.Object {
		case "NYC":
			nycFact = &page.Facts[i]
		case "SF":
			sfFact = &page.Facts[i]
		}
	}
	if nycFact == nil {
		t.Fatal("NYC fact missing from history")
	}
	if nycFact.InvalidatedAt == nil {
		t.Errorf("NYC fact should be invalidated, got InvalidatedAt=nil")
	}
	if sfFact == nil {
		t.Fatal("SF fact missing")
	}
	if sfFact.InvalidatedAt != nil {
		t.Errorf("SF fact should be valid, got InvalidatedAt=%v", sfFact.InvalidatedAt)
	}
}

func TestMemoryImpl_RecallMemory_AccessTracking(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "hello",
		Content:    "hello world content",
		Importance: 0.6,
	})

	// First recall
	res, err := mw.RecallMemory(ctx, "hello", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	})
	if err != nil {
		t.Fatalf("RecallMemory: %v", err)
	}
	if len(res.Pages) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(res.Pages))
	}

	// Read back; AccessCount should be 1
	page, _ := mw.GetPage(ctx, res.Pages[0].Path)
	if page.AccessCount != 1 {
		t.Errorf("AccessCount after first recall = %d, want 1", page.AccessCount)
	}
	if page.LastAccessedAt == nil {
		t.Error("LastAccessedAt should be set")
	}

	// Second recall
	_, _ = mw.RecallMemory(ctx, "hello", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	})
	page, _ = mw.GetPage(ctx, res.Pages[0].Path)
	if page.AccessCount != 2 {
		t.Errorf("AccessCount after second recall = %d, want 2", page.AccessCount)
	}
}

func TestMemoryImpl_RecallMemory_FiltersInvalidatedFacts(t *testing.T) {
	mw := newTestWiki(t, &llm.MockLLM{
		ExtractMemoryFactsFunc: func(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
			return nil, nil
		},
	}, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	// Manually craft a page with mixed facts
	now := time.Now()
	old := now.Add(-1 * time.Hour)
	st := mw.WikiImpl.storage
	page := &schema.Page{
		Path: "preferences/lang.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypePreference, Title: "lang",
			Importance: 0.6,
			Facts: []schema.MemoryFact{
				{Subject: "user", Predicate: "speaks", Object: "English"},
				{Subject: "user", Predicate: "speaks", Object: "Spanish", InvalidatedAt: &old},
			},
		},
		Content:   "language preferences",
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = st.WritePage(ctx, page)
	_ = mw.WikiImpl.index.Index(ctx, page)

	// Recall with default IncludeInvalidated=false
	res, _ := mw.RecallMemory(ctx, "language", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference},
	})
	if len(res.Pages) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(res.Pages))
	}
	if len(res.Pages[0].Facts) != 1 {
		t.Errorf("expected 1 active fact, got %d", len(res.Pages[0].Facts))
	}
	if res.Pages[0].Facts[0].Object != "English" {
		t.Errorf("got Object=%s, want English", res.Pages[0].Facts[0].Object)
	}

	// IncludeInvalidated=true → returns both
	res, _ = mw.RecallMemory(ctx, "language", &RecallOptions{
		Types:              []schema.PageType{schema.PageTypePreference},
		IncludeInvalidated: true,
	})
	if len(res.Pages[0].Facts) != 2 {
		t.Errorf("with IncludeInvalidated=true, expected 2 facts, got %d", len(res.Pages[0].Facts))
	}
}

func TestMemoryImpl_AppendAuditLog(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		err := mw.AppendAuditLog(ctx, &AuditEntry{
			AgentID:    "agent-x",
			TenantID:   "user-A",
			Action:     "memory_store",
			TargetPath: "memories/x.md",
		})
		if err != nil {
			t.Fatalf("AppendAuditLog: %v", err)
		}
	}

	// Read back the audit log file (tenant-namespaced)
	yearMonth := time.Now().UTC().Format("2006-01")
	page, err := mw.GetPage(ctx, "tenants/user-A/audit/"+yearMonth+".md")
	if err != nil {
		t.Fatalf("GetPage(audit): %v", err)
	}

	// Should have 3 lines (no overwrites)
	lines := 0
	for _, c := range page.Content {
		if c == '\n' {
			lines++
		}
	}
	// 3 lines + final newline-or-not
	if lines < 2 {
		t.Errorf("expected at least 2 newlines (3 entries), got %d:\n%s", lines, page.Content)
	}
}

func TestMemoryImpl_SetImportance(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()
	res, _ := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "x",
		Content:    "x content",
		Importance: 0.3,
	})

	if err := mw.SetImportance(ctx, res.Path, 0.95); err != nil {
		t.Fatalf("SetImportance: %v", err)
	}
	page, _ := mw.GetPage(ctx, res.Path)
	if page.Importance != 0.95 {
		t.Errorf("Importance = %f, want 0.95", page.Importance)
	}

	// Out-of-range should fail
	if err := mw.SetImportance(ctx, res.Path, 1.5); err == nil {
		t.Error("expected error for score > 1")
	}
	if err := mw.SetImportance(ctx, res.Path, -0.1); err == nil {
		t.Error("expected error for score < 0")
	}
}

func TestMemoryImpl_SetTTLAndRunGC(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()
	res, _ := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "ephemeral",
		Content:    "soon to be gone",
		Importance: 0.5,
	})

	// Set TTL to 1 ms in the past
	past := time.Now().Add(-1 * time.Hour)
	if err := mw.SetTTL(ctx, res.Path, &past); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}

	gc, err := mw.RunGC(ctx)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if gc.DeletedCount != 1 {
		t.Errorf("DeletedCount = %d, want 1", gc.DeletedCount)
	}
	// Page should now be gone
	if _, err := mw.GetPage(ctx, res.Path); err == nil {
		t.Errorf("expected GetPage to fail after GC")
	}
}

func TestMemoryImpl_VectorIndexUpdatedOnStore(t *testing.T) {
	mw := newTestWiki(t, &llm.MockLLM{
		EmbedFunc: func(_ context.Context, text string) ([]float32, error) {
			return []float32{1, 0, 0}, nil
		},
	}, true, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	_, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypePreference,
		Title:      "vec-test",
		Content:    "any content",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	// The vector index should have one entry at the page path
	results, err := mw.vector.SearchVector(ctx, []float32{1, 0, 0}, &index.VectorSearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	if len(results.Results) != 1 || results.Results[0].Path != "preferences/vec-test.md" {
		t.Errorf("vector index missing or mispathed: %+v", results.Results)
	}
}

func TestMemoryImpl_LinkExtractorPopulatesRelated(t *testing.T) {
	mw := newTestWiki(t, &llm.MockLLM{
		ExtractMemoryFactsFunc: func(_ context.Context, _ string, _ schema.PageType) ([]schema.MemoryFact, error) {
			return nil, nil
		},
	}, false,
		WithFactExtractor(NoopFactExtractor{}),
		WithLinkExtractor(NewRegexLinkExtractor(DefaultLinkPatterns())),
	)
	ctx := context.Background()

	res, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "meeting",
		Content:    "Pedro works at Brex.",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	page, _ := mw.GetPage(ctx, res.Path)
	found := false
	for _, r := range page.Related {
		if r == "Brex" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Brex' in Related, got %v", page.Related)
	}
}

func TestMemoryImpl_AssembleContext(t *testing.T) {
	mw := newTestWiki(t, nil, false, WithFactExtractor(NoopFactExtractor{}))
	ctx := context.Background()

	_, _ = mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type: schema.PageTypePreference, Title: "lang",
		Content: "User prefers English.", Importance: 0.9,
	})

	got, err := mw.AssembleContext(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
	})
	if err != nil {
		t.Fatalf("AssembleContext: %v", err)
	}
	if got.Stats.PagesIncluded != 1 {
		t.Errorf("PagesIncluded = %d, want 1", got.Stats.PagesIncluded)
	}
	if len(got.Citations) != 1 {
		t.Errorf("Citations = %v", got.Citations)
	}
}
