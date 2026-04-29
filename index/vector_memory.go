package index

import (
	"bytes"
	"context"
	"encoding/gob"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// vectorEntry is the unit stored per page in the in-memory index.
type vectorEntry struct {
	Vec       []float32
	Type      schema.PageType
	TenantID  string
	ExpiresAt *time.Time
}

// MemoryVectorIndex is an in-memory VectorIndex backed by a gob-encoded file.
// Cosine similarity is computed with brute-force scanning (adequate for <50k pages).
type MemoryVectorIndex struct {
	mu      sync.RWMutex
	entries map[string]*vectorEntry // path → entry
}

// NewMemoryVectorIndex creates a new empty in-memory vector index.
func NewMemoryVectorIndex() *MemoryVectorIndex {
	return &MemoryVectorIndex{
		entries: make(map[string]*vectorEntry),
	}
}

// IndexVector stores or replaces the embedding for path.
func (v *MemoryVectorIndex) IndexVector(_ context.Context, path string, vec []float32, meta *VectorMeta) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry := &vectorEntry{Vec: vec}
	if meta != nil {
		entry.Type = meta.Type
		entry.TenantID = meta.TenantID
		entry.ExpiresAt = meta.ExpiresAt
	}
	v.entries[path] = entry
	return nil
}

// SearchVector returns pages sorted by cosine similarity to query.
func (v *MemoryVectorIndex) SearchVector(_ context.Context, query []float32, opts *VectorSearchOptions) (*VectorSearchResult, error) {
	if opts == nil {
		opts = &VectorSearchOptions{}
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 10
	}

	now := time.Now()
	typeSet := make(map[schema.PageType]bool, len(opts.Types))
	for _, t := range opts.Types {
		typeSet[t] = true
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	type scored struct {
		path  string
		score float64
		ptype schema.PageType
	}
	var candidates []scored

	qNorm := norm(query)
	if qNorm == 0 {
		return &VectorSearchResult{}, nil
	}

	for path, e := range v.entries {
		// Tenant filter
		if opts.TenantID != "" && e.TenantID != opts.TenantID {
			continue
		}
		// Type filter
		if len(typeSet) > 0 && !typeSet[e.Type] {
			continue
		}
		// Expiry filter
		if opts.ExcludeExpired && e.ExpiresAt != nil && e.ExpiresAt.Before(now) {
			continue
		}
		if len(e.Vec) == 0 || len(e.Vec) != len(query) {
			continue
		}

		score := dot(query, e.Vec) / (qNorm * norm(e.Vec))
		if score < opts.MinScore {
			continue
		}
		candidates = append(candidates, scored{path: path, score: score, ptype: e.Type})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	items := make([]VectorSearchItem, len(candidates))
	for i, c := range candidates {
		items[i] = VectorSearchItem{Path: c.path, Score: c.score, Type: c.ptype}
	}
	return &VectorSearchResult{Results: items}, nil
}

// Remove deletes a path from the index.
func (v *MemoryVectorIndex) Remove(_ context.Context, path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.entries, path)
	return nil
}

// gobData is the serialization container for Persist/Load.
type gobData struct {
	Entries map[string]*vectorEntry
}

// Persist saves the index to filePath using gob encoding.
func (v *MemoryVectorIndex) Persist(filePath string) error {
	v.mu.RLock()
	data := gobData{Entries: v.entries}
	v.mu.RUnlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0o600)
}

// Load replaces the index contents from a file written by Persist.
func (v *MemoryVectorIndex) Load(filePath string) error {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var data gobData
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&data); err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = data.Entries
	return nil
}

// Close is a no-op for the in-memory implementation.
func (v *MemoryVectorIndex) Close() error { return nil }

// ---- math helpers ----

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func norm(a []float32) float64 {
	return math.Sqrt(dot(a, a))
}
