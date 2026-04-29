package index

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

func TestMemoryVectorIndex_CosineRanking(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()

	// Three orthogonal-ish vectors
	_ = idx.IndexVector(ctx, "exact.md", []float32{1, 0, 0}, &VectorMeta{Type: schema.PageTypeMemory})
	_ = idx.IndexVector(ctx, "perpendicular.md", []float32{0, 1, 0}, &VectorMeta{Type: schema.PageTypeMemory})
	_ = idx.IndexVector(ctx, "near.md", []float32{0.9, 0.1, 0}, &VectorMeta{Type: schema.PageTypeMemory})

	res, err := idx.SearchVector(ctx, []float32{1, 0, 0}, &VectorSearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("SearchVector failed: %v", err)
	}

	if len(res.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res.Results))
	}
	if res.Results[0].Path != "exact.md" {
		t.Errorf("rank 1 = %s, want exact.md", res.Results[0].Path)
	}
	if res.Results[1].Path != "near.md" {
		t.Errorf("rank 2 = %s, want near.md", res.Results[1].Path)
	}
	if res.Results[2].Path != "perpendicular.md" {
		t.Errorf("rank 3 = %s, want perpendicular.md", res.Results[2].Path)
	}

	// exact match should score 1.0
	if math.Abs(res.Results[0].Score-1.0) > 1e-6 {
		t.Errorf("exact match score = %f, want 1.0", res.Results[0].Score)
	}
	// perpendicular should score 0
	if math.Abs(res.Results[2].Score) > 1e-6 {
		t.Errorf("perpendicular score = %f, want 0", res.Results[2].Score)
	}
}

func TestMemoryVectorIndex_TypeAndTenantFilter(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()

	q := []float32{1, 0}
	_ = idx.IndexVector(ctx, "pref-A.md", q, &VectorMeta{Type: schema.PageTypePreference, TenantID: "A"})
	_ = idx.IndexVector(ctx, "pref-B.md", q, &VectorMeta{Type: schema.PageTypePreference, TenantID: "B"})
	_ = idx.IndexVector(ctx, "mem-A.md", q, &VectorMeta{Type: schema.PageTypeMemory, TenantID: "A"})

	// Tenant filter
	res, _ := idx.SearchVector(ctx, q, &VectorSearchOptions{TenantID: "A", Limit: 10})
	if len(res.Results) != 2 {
		t.Errorf("tenant=A should return 2, got %d", len(res.Results))
	}
	for _, r := range res.Results {
		if r.Path == "pref-B.md" {
			t.Error("tenant filter leaked B page")
		}
	}

	// Type filter
	res, _ = idx.SearchVector(ctx, q, &VectorSearchOptions{
		Types: []schema.PageType{schema.PageTypePreference}, Limit: 10,
	})
	if len(res.Results) != 2 {
		t.Errorf("type=preference should return 2, got %d", len(res.Results))
	}

	// Combined
	res, _ = idx.SearchVector(ctx, q, &VectorSearchOptions{
		TenantID: "A",
		Types:    []schema.PageType{schema.PageTypePreference},
		Limit:    10,
	})
	if len(res.Results) != 1 || res.Results[0].Path != "pref-A.md" {
		t.Errorf("combined filter failed: %v", res.Results)
	}
}

func TestMemoryVectorIndex_ExcludeExpired(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	q := []float32{1, 0}
	_ = idx.IndexVector(ctx, "expired.md", q, &VectorMeta{
		Type: schema.PageTypeMemory, ExpiresAt: &past,
	})
	_ = idx.IndexVector(ctx, "fresh.md", q, &VectorMeta{Type: schema.PageTypeMemory})

	res, _ := idx.SearchVector(ctx, q, &VectorSearchOptions{Limit: 10, ExcludeExpired: true})
	if len(res.Results) != 1 || res.Results[0].Path != "fresh.md" {
		t.Errorf("ExcludeExpired filter failed: %v", res.Results)
	}
}

func TestMemoryVectorIndex_DimensionMismatch(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()

	// Indexed at dim=3
	_ = idx.IndexVector(ctx, "p.md", []float32{1, 0, 0}, nil)

	// Query at dim=4 → should silently skip
	res, err := idx.SearchVector(ctx, []float32{1, 0, 0, 0}, &VectorSearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("SearchVector errored on dim mismatch: %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("dim mismatch should yield 0 results, got %d", len(res.Results))
	}
}

func TestMemoryVectorIndex_PersistLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vec.idx")

	a := NewMemoryVectorIndex()
	ctx := context.Background()
	_ = a.IndexVector(ctx, "p1.md", []float32{1, 0, 0}, &VectorMeta{Type: schema.PageTypeMemory, TenantID: "x"})
	_ = a.IndexVector(ctx, "p2.md", []float32{0, 1, 0}, &VectorMeta{Type: schema.PageTypePreference, TenantID: "y"})

	if err := a.Persist(file); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	stat, err := os.Stat(file)
	if err != nil || stat.Size() == 0 {
		t.Fatalf("persist file missing or empty: err=%v size=%d", err, stat.Size())
	}

	b := NewMemoryVectorIndex()
	if err := b.Load(file); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Round-trip should preserve search behavior
	res, _ := b.SearchVector(ctx, []float32{1, 0, 0}, &VectorSearchOptions{Limit: 10})
	if len(res.Results) != 2 {
		t.Fatalf("post-load: expected 2 results, got %d", len(res.Results))
	}
	if res.Results[0].Path != "p1.md" {
		t.Errorf("post-load top = %s, want p1.md", res.Results[0].Path)
	}
	if res.Results[0].Type != schema.PageTypeMemory {
		t.Errorf("post-load type = %s, want memory", res.Results[0].Type)
	}
}

func TestMemoryVectorIndex_Remove(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()
	_ = idx.IndexVector(ctx, "p.md", []float32{1, 0}, nil)

	res, _ := idx.SearchVector(ctx, []float32{1, 0}, &VectorSearchOptions{Limit: 10})
	if len(res.Results) != 1 {
		t.Fatalf("pre-remove: 1 expected, got %d", len(res.Results))
	}

	if err := idx.Remove(ctx, "p.md"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	res, _ = idx.SearchVector(ctx, []float32{1, 0}, &VectorSearchOptions{Limit: 10})
	if len(res.Results) != 0 {
		t.Errorf("post-remove: 0 expected, got %d", len(res.Results))
	}

	// Removing nonexistent path is a no-op
	if err := idx.Remove(ctx, "nonexistent.md"); err != nil {
		t.Errorf("Remove nonexistent should not error, got %v", err)
	}
}

func TestMemoryVectorIndex_ZeroNormQuery(t *testing.T) {
	idx := NewMemoryVectorIndex()
	ctx := context.Background()
	_ = idx.IndexVector(ctx, "p.md", []float32{1, 0}, nil)

	// Zero vector query should yield no results (norm=0 short-circuit)
	res, err := idx.SearchVector(ctx, []float32{0, 0}, &VectorSearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("zero query errored: %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("zero-norm query should yield 0, got %d", len(res.Results))
	}
}
