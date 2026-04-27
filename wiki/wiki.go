package wiki

import (
	"context"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// QueryOptions controls query behavior
type QueryOptions struct {
	// Limit is max results
	Limit int

	// ArchiveResult whether to save the answer as a new page
	ArchiveResult bool

	// ArchivePath is the path to save the answer (if ArchiveResult is true)
	ArchivePath string
}

// LintOptions controls lint behavior
type LintOptions struct {
	// CheckContradictions whether to check for contradictions
	CheckContradictions bool

	// CheckOrphans whether to check for orphan pages
	CheckOrphans bool

	// CheckStale whether to check for stale information
	CheckStale bool

	// CheckMissingRefs whether to check for missing cross-references
	CheckMissingRefs bool
}

// ListOptions controls listing behavior
type ListOptions struct {
	// Type filters by page type
	Type *schema.PageType

	// Prefix filters by path prefix
	Prefix string

	// Limit is max results
	Limit int

	// Tags filters by tags
	Tags []string
}

// IngestResult is the result of an ingest operation
type IngestResult struct {
	// PagesCreated is the number of pages created
	PagesCreated int `json:"pages_created"`

	// PagesUpdated is the number of pages updated
	PagesUpdated int `json:"pages_updated"`

	// CreatedPaths are the paths of created pages
	CreatedPaths []string `json:"created_paths"`

	// UpdatedPaths are the paths of updated pages
	UpdatedPaths []string `json:"updated_paths"`

	// ExtractedInfo is the information extracted from the source
	ExtractedInfo *schema.ExtractedInfo `json:"extracted_info"`
}

// QueryResult is the result of a query operation
type QueryResult struct {
	// Answer is the synthesized answer
	Answer string `json:"answer"`

	// Citations are references to source pages
	Citations []schema.Citation `json:"citations"`

	// PagesRead are the pages that were read
	PagesRead []string `json:"pages_read"`

	// ArchivedPath is the path where the answer was archived (if any)
	ArchivedPath string `json:"archived_path,omitempty"`
}

// Wiki is the main interface for knowledge management
type Wiki interface {
	// Ingest processes a new source document
	Ingest(ctx context.Context, source *schema.Source) (*IngestResult, error)

	// Query answers questions using wiki content
	Query(ctx context.Context, query string, opts *QueryOptions) (*QueryResult, error)

	// Lint performs health checks on the wiki
	Lint(ctx context.Context, opts *LintOptions) (*llm.LintReport, error)

	// GetPage retrieves a single page
	GetPage(ctx context.Context, path string) (*schema.Page, error)

	// UpdatePage updates an existing page
	UpdatePage(ctx context.Context, page *schema.Page) error

	// ListPages lists pages matching criteria
	ListPages(ctx context.Context, opts *ListOptions) ([]*schema.Page, error)

	// Close releases resources
	Close() error
}
