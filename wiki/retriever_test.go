package wiki

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// retrieverFixture wires storage + indexes + retriever for tests.
type retrieverFixture struct {
	storage  *storage.MemoryStorage
	fulltext *index.FullTextIndex
	vector   *index.MemoryVectorIndex
	llm      *llm.MockLLM
	r        *HybridRetriever
}

func newFixture(withVector bool) *retrieverFixture {
	st := storage.NewMemoryStorage()
	ft := index.NewFullTextIndex()
	mock := &llm.MockLLM{}
	scorer := DefaultImportanceScorer()
	var vec index.VectorIndex
	var memVec *index.MemoryVectorIndex
	if withVector {
		memVec = index.NewMemoryVectorIndex()
		vec = memVec
	}
	r := NewHybridRetriever(ft, vec, scorer, mock)
	return &retrieverFixture{
		storage:  st,
		fulltext: ft,
		vector:   memVec,
		llm:      mock,
		r:        r,
	}
}

func (f *retrieverFixture) addPage(t *testing.T, page *schema.Page, vec []float32) {
	t.Helper()
	ctx := context.Background()
	if err := f.storage.WritePage(ctx, page); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	if err := f.fulltext.Index(ctx, page); err != nil {
		t.Fatalf("fulltext.Index: %v", err)
	}
	if f.vector != nil && vec != nil {
		if err := f.vector.IndexVector(ctx, page.Path, vec, &index.VectorMeta{
			Type:     page.Type,
			TenantID: page.TenantID,
		}); err != nil {
			t.Fatalf("vector.IndexVector: %v", err)
		}
	}
}

func TestRetriever_DegradesToKeywordWithoutVector(t *testing.T) {
	fx := newFixture(false) // no vector index
	ctx := context.Background()

	fx.addPage(t, &schema.Page{
		Path: "memories/note.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypeMemory,
			Title:      "meeting note",
			Importance: 0.5,
		},
		Content: "Discussed roadmap with the team.",
	}, nil)

	scored, err := fx.r.Recall(ctx, "roadmap", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 1 {
		t.Fatalf("expected 1 hit (keyword-only mode), got %d", len(scored))
	}
	if scored[0].Page.Path != "memories/note.md" {
		t.Errorf("got %s, want memories/note.md", scored[0].Page.Path)
	}
}

func TestRetriever_VectorBoostForPreferenceLayer(t *testing.T) {
	fx := newFixture(true)
	ctx := context.Background()

	// Embedder returns embedding aligned with the query vector for the matching page
	matchVec := []float32{1, 0, 0}
	missVec := []float32{0, 1, 0}

	// Two pages with NO common keywords with the query
	fx.addPage(t, &schema.Page{
		Path: "preferences/p1.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypePreference,
			Title:      "alpha",
			Importance: 0.5,
		},
		Content: "alpha content",
	}, matchVec)
	fx.addPage(t, &schema.Page{
		Path: "preferences/p2.md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypePreference,
			Title:      "beta",
			Importance: 0.5,
		},
		Content: "beta content",
	}, missVec)

	// Mock LLM returns the matching vector as the query's embedding
	fx.llm.EmbedFunc = func(_ context.Context, text string) ([]float32, error) {
		return matchVec, nil
	}

	scored, err := fx.r.Recall(ctx, "totally-unrelated-keyword", &RecallOptions{
		Types: []schema.PageType{schema.PageTypePreference},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) == 0 {
		t.Fatal("expected vector match despite zero keyword overlap")
	}
	if scored[0].Page.Path != "preferences/p1.md" {
		t.Errorf("top hit = %s, want preferences/p1.md", scored[0].Page.Path)
	}
}

func TestRetriever_RecencyDecayAffectsScore(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	now := time.Now()
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	t60 := now.Add(-60 * 24 * time.Hour)

	// Two pages with same keyword score but different recency
	fx.addPage(t, &schema.Page{
		Path: "memories/recent.md",
		Frontmatter: schema.Frontmatter{
			Type:           schema.PageTypeMemory,
			Title:          "recent",
			Importance:     0.5,
			LastAccessedAt: &tenDaysAgo,
		},
		Content: "shared keyword content",
	}, nil)
	fx.addPage(t, &schema.Page{
		Path: "memories/old.md",
		Frontmatter: schema.Frontmatter{
			Type:           schema.PageTypeMemory,
			Title:          "old",
			Importance:     0.5,
			LastAccessedAt: &t60,
		},
		Content: "shared keyword content",
	}, nil)

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) < 2 {
		t.Fatalf("expected 2 hits, got %d", len(scored))
	}
	// Memory layer strategy weights recency at 0.3 → recent should win
	if scored[0].Page.Path != "memories/recent.md" {
		t.Errorf("rank 1 = %s, want memories/recent.md (recency boost)", scored[0].Page.Path)
	}
}

func TestRetriever_StrategyOverride(t *testing.T) {
	fx := newFixture(false)

	// Verify default exists
	def := fx.r.strategyFor(schema.PageTypePreference)
	if def.VectorWeight != 1.0 {
		t.Errorf("default preference VectorWeight = %f, want 1.0", def.VectorWeight)
	}

	// Override
	fx.r.strategies[schema.PageTypePreference] = LayerStrategy{KeywordWeight: 1.0}
	override := fx.r.strategyFor(schema.PageTypePreference)
	if override.KeywordWeight != 1.0 {
		t.Errorf("override KeywordWeight = %f, want 1.0", override.KeywordWeight)
	}

	// Unknown type falls back to keyword-heavy default
	unknown := fx.r.strategyFor(schema.PageType("unknown"))
	if unknown.KeywordWeight == 0 {
		t.Errorf("unknown type should have non-zero KeywordWeight, got %+v", unknown)
	}
}

func TestRetriever_LimitAndOverFetch(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		fx.addPage(t, &schema.Page{
			Path: "memories/" + string(rune('a'+i)) + ".md",
			Frontmatter: schema.Frontmatter{
				Type:       schema.PageTypeMemory,
				Title:      "page" + string(rune('a'+i)),
				Importance: 0.5,
			},
			Content: "shared content",
		}, nil)
	}

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Limit: 5,
		Types: []schema.PageType{schema.PageTypeMemory},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 5 {
		t.Errorf("expected exactly 5 results (Limit), got %d", len(scored))
	}
}

func TestRetriever_TenantIsolation(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	a := &schema.Page{
		Path: "preferences/a.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypePreference, Title: "a", TenantID: "user-A", Importance: 0.5,
		},
		Content: "secret data",
	}
	b := &schema.Page{
		Path: "preferences/b.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypePreference, Title: "b", TenantID: "user-B", Importance: 0.5,
		},
		Content: "secret data",
	}
	fx.addPage(t, a, nil)
	fx.addPage(t, b, nil)

	scored, _ := fx.r.Recall(ctx, "secret", &RecallOptions{
		Types:    []schema.PageType{schema.PageTypePreference},
		TenantID: "user-A",
	}, fx.storage)
	if len(scored) != 1 || scored[0].Page.TenantID != "user-A" {
		t.Errorf("tenant isolation failed: %+v", scored)
	}
}
