package index

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// SearchOptions controls search behavior
type SearchOptions struct {
	// Limit is max results
	Limit int

	// Type filters by page type
	Type *schema.PageType

	// Tags filters by tags
	Tags []string

	// MinScore is minimum relevance score
	MinScore float64

	// Memory-scoping filters

	// TenantID restricts results to a specific tenant namespace (empty = all)
	TenantID string

	// MinImportance filters out pages below this importance threshold (0 = no filter)
	MinImportance float64

	// MemoryTier restricts results to a specific tier (empty = all)
	MemoryTier schema.MemoryTier

	// ExcludeExpired skips pages whose ExpiresAt is in the past
	ExcludeExpired bool
}

// SearchResult is the result of a search
type SearchResult struct {
	// Results are the matching pages
	Results []SearchResultItem `json:"results"`

	// Total is the total number of matches
	Total int `json:"total"`
}

// SearchResultItem is a single search result
type SearchResultItem struct {
	// Page is the matching page
	Page *schema.Page `json:"page"`

	// Score is the relevance score
	Score float64 `json:"score"`

	// Excerpt is a relevant excerpt
	Excerpt string `json:"excerpt,omitempty"`
}

// Index defines search and indexing operations
type Index interface {
	// Index adds a page to the index
	Index(ctx context.Context, page *schema.Page) error

	// Search finds relevant pages
	Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error)

	// Remove removes a page from the index
	Remove(ctx context.Context, path string) error

	// Close closes the index
	Close() error
}
