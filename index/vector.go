package index

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// VectorMeta holds the filterable metadata stored alongside each embedding.
type VectorMeta struct {
	Type      schema.PageType
	TenantID  string
	ExpiresAt *time.Time
}

// VectorSearchOptions controls vector search behavior.
type VectorSearchOptions struct {
	// Limit is the maximum number of results (0 = 10)
	Limit int

	// MinScore filters out results below this cosine similarity (0 = no filter)
	MinScore float64

	// TenantID restricts results to a namespace (empty = all)
	TenantID string

	// Types restricts results to specific page types (nil = all)
	Types []schema.PageType

	// ExcludeExpired skips pages whose ExpiresAt is in the past
	ExcludeExpired bool
}

// VectorSearchItem is a single result from a vector search.
type VectorSearchItem struct {
	// Path is the page path
	Path string

	// Score is the cosine similarity [0, 1]
	Score float64

	// Type is the page type of this result
	Type schema.PageType
}

// VectorSearchResult is the response from SearchVector.
type VectorSearchResult struct {
	Results []VectorSearchItem
}

// VectorIndex stores dense embeddings and retrieves by cosine similarity.
// It is optional: MemoryWikiImpl degrades gracefully to keyword-only if nil.
type VectorIndex interface {
	// IndexVector stores or updates the embedding for a page path.
	IndexVector(ctx context.Context, path string, vec []float32, meta *VectorMeta) error

	// SearchVector returns pages ordered by cosine similarity to the query vector.
	SearchVector(ctx context.Context, query []float32, opts *VectorSearchOptions) (*VectorSearchResult, error)

	// Remove deletes a path from the vector index.
	Remove(ctx context.Context, path string) error

	// Persist serialises the index to a file (gob-encoded map).
	Persist(filePath string) error

	// Load deserialises the index from a file written by Persist.
	Load(filePath string) error

	// Close releases any resources held by the index.
	Close() error
}
