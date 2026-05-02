package index

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

func mkPage(path, title, content string, t schema.PageType) *schema.Page {
	return &schema.Page{
		Path: path,
		Frontmatter: schema.Frontmatter{
			Type:  t,
			Title: title,
		},
		Content: content,
	}
}

func TestFullTextIndex_BasicSearch(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	pages := []*schema.Page{
		mkPage("p1.md", "Go Programming", "Go is a fast language for backend services", schema.PageTypeConcept),
		mkPage("p2.md", "Python", "Python is a popular language for data science", schema.PageTypeConcept),
		mkPage("p3.md", "Rust", "Rust is a memory-safe systems language", schema.PageTypeConcept),
	}
	for _, p := range pages {
		if err := idx.Index(ctx, p); err != nil {
			t.Fatalf("index failed: %v", err)
		}
	}

	res, err := idx.Search(ctx, "language", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Results) != 3 {
		t.Errorf("expected 3 hits for 'language', got %d", len(res.Results))
	}

	// All three pages contain "language" exactly once.
	// BM25 scores differ slightly due to document length normalization (shorter docs score higher).
	if len(res.Results) != 3 {
		t.Errorf("expected 3 hits for 'language', got %d", len(res.Results))
	}
	for _, r := range res.Results {
		if r.Score <= 0 {
			t.Errorf("expected positive BM25 score for %s, got %f", r.Page.Path, r.Score)
		}
	}
}

func TestFullTextIndex_TypeFilter(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	_ = idx.Index(ctx, mkPage("e1.md", "OpenAI", "AI org", schema.PageTypeEntity))
	_ = idx.Index(ctx, mkPage("c1.md", "Concept", "AI is intelligence", schema.PageTypeConcept))

	want := schema.PageTypeEntity
	res, _ := idx.Search(ctx, "ai", &SearchOptions{Limit: 10, Type: &want})

	if len(res.Results) != 1 {
		t.Fatalf("expected 1 entity hit, got %d", len(res.Results))
	}
	if res.Results[0].Page.Path != "e1.md" {
		t.Errorf("expected e1.md, got %s", res.Results[0].Page.Path)
	}
}

func TestFullTextIndex_TenantFilter(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	pA := mkPage("a.md", "data", "tenant a data", schema.PageTypeMemory)
	pA.TenantID = "tenant-a"
	pB := mkPage("b.md", "data", "tenant b data", schema.PageTypeMemory)
	pB.TenantID = "tenant-b"

	_ = idx.Index(ctx, pA)
	_ = idx.Index(ctx, pB)

	res, _ := idx.Search(ctx, "data", &SearchOptions{Limit: 10, TenantID: "tenant-a"})
	if len(res.Results) != 1 || res.Results[0].Page.Path != "a.md" {
		t.Errorf("expected only tenant-a result, got %v", res.Results)
	}
}

func TestFullTextIndex_ExcludeExpired(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	expired := mkPage("expired.md", "stale", "old data", schema.PageTypeMemory)
	expired.ExpiresAt = &past

	future := time.Now().Add(1 * time.Hour)
	fresh := mkPage("fresh.md", "alive", "new data", schema.PageTypeMemory)
	fresh.ExpiresAt = &future

	_ = idx.Index(ctx, expired)
	_ = idx.Index(ctx, fresh)

	res, _ := idx.Search(ctx, "data", &SearchOptions{Limit: 10, ExcludeExpired: true})
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 non-expired hit, got %d", len(res.Results))
	}
	if res.Results[0].Page.Path != "fresh.md" {
		t.Errorf("expected fresh.md, got %s", res.Results[0].Page.Path)
	}
}

func TestFullTextIndex_MinImportance(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	low := mkPage("low.md", "x", "test content", schema.PageTypeMemory)
	low.Importance = 0.2
	high := mkPage("high.md", "y", "test content", schema.PageTypeMemory)
	high.Importance = 0.8

	_ = idx.Index(ctx, low)
	_ = idx.Index(ctx, high)

	res, _ := idx.Search(ctx, "test", &SearchOptions{Limit: 10, MinImportance: 0.5})
	if len(res.Results) != 1 || res.Results[0].Page.Path != "high.md" {
		t.Errorf("MinImportance filter failed: %v", res.Results)
	}
}

func TestFullTextIndex_RemoveAndReindex(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	p := mkPage("p.md", "doc", "hello world", schema.PageTypeMemory)
	_ = idx.Index(ctx, p)

	res, _ := idx.Search(ctx, "world", &SearchOptions{Limit: 10})
	if len(res.Results) != 1 {
		t.Fatalf("pre-remove: expected 1 hit, got %d", len(res.Results))
	}

	if err := idx.Remove(ctx, "p.md"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	res, _ = idx.Search(ctx, "world", &SearchOptions{Limit: 10})
	if len(res.Results) != 0 {
		t.Errorf("post-remove: expected 0 hits, got %d", len(res.Results))
	}

	// Re-index should resurrect the page
	_ = idx.Index(ctx, p)
	res, _ = idx.Search(ctx, "world", &SearchOptions{Limit: 10})
	if len(res.Results) != 1 {
		t.Errorf("post-reindex: expected 1 hit, got %d", len(res.Results))
	}
}

func TestFullTextIndex_TitleAndTagsTokenized(t *testing.T) {
	idx := NewFullTextIndex()
	ctx := context.Background()

	// Content has no "rust" but title does
	p := mkPage("rust.md", "Rust Programming", "Memory-safe systems language", schema.PageTypeConcept)
	p.Tags = []string{"systems", "performance"}
	_ = idx.Index(ctx, p)

	// Title token
	res, _ := idx.Search(ctx, "rust", &SearchOptions{Limit: 10})
	if len(res.Results) != 1 {
		t.Errorf("expected title token to be searchable, got %d hits", len(res.Results))
	}

	// Tag token
	res, _ = idx.Search(ctx, "performance", &SearchOptions{Limit: 10})
	if len(res.Results) != 1 {
		t.Errorf("expected tag token to be searchable, got %d hits", len(res.Results))
	}
}
