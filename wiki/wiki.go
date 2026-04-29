package wiki

import (
	"context"
	"time"

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

	// RebuildIndex rebuilds the search index from existing pages
	RebuildIndex(ctx context.Context) error

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

// ---- Memory system types ----

// StoreMemoryRequest is the input for StoreMemory
type StoreMemoryRequest struct {
	// Type must be PageTypeMemory, PageTypePreference, or PageTypeAuditLog
	Type schema.PageType

	// Title is a stable key for the memory (used for dedup: same title = update)
	Title string

	// Content is the markdown body
	Content string

	// Tags are optional classification tags
	Tags []string

	// Importance is the initial importance score (0 uses default 0.5)
	Importance float64

	// TTL is the optional time-to-live; nil means never expires
	TTL *time.Duration

	// TenantID scopes the memory to a specific agent/user namespace
	TenantID string

	// AgentID identifies the writing agent
	AgentID string
}

// StoreMemoryResult is returned by StoreMemory
type StoreMemoryResult struct {
	// Path is the storage path of the page
	Path string

	// Created is true when a new page was created, false when an existing one was updated
	Created bool
}

// RecallOptions controls RecallMemory behavior
type RecallOptions struct {
	// Types restricts to specific memory page types (empty = all)
	Types []schema.PageType

	// TenantID restricts to a specific namespace (empty = all)
	TenantID string

	// MinImportance filters out pages below this threshold (0 = no filter)
	MinImportance float64

	// MemoryTier restricts to hot/warm/cold (empty = all)
	MemoryTier schema.MemoryTier

	// Limit is max results (0 = default 10)
	Limit int

	// IncludeInvalidated, when true, returns facts whose InvalidatedAt is set.
	// Useful for temporal queries ("what did the user think before?").
	// Default false = only current (valid) facts are visible.
	IncludeInvalidated bool

	// Strategies overrides the per-layer retrieval weight coefficients.
	// Keys are PageType constants; absent types fall back to defaults.
	Strategies map[schema.PageType]LayerStrategy
}

// RecallResult is returned by RecallMemory
type RecallResult struct {
	// Pages are the matching memory pages, ordered by relevance
	Pages []*schema.Page

	// Total is the total number of matches before the limit was applied
	Total int
}

// AuditEntry is a single agent operation log entry
type AuditEntry struct {
	// AgentID identifies the agent
	AgentID string

	// TenantID scopes the entry
	TenantID string

	// Action is a short verb (e.g., "ingest", "query", "memory_store")
	Action string

	// TargetPath is the wiki path affected (may be empty)
	TargetPath string

	// Metadata holds arbitrary key-value context
	Metadata map[string]string

	// Timestamp defaults to now if zero
	Timestamp time.Time
}

// GCResult summarises a garbage-collection run
type GCResult struct {
	// DeletedCount is how many expired pages were physically removed
	DeletedCount int

	// DeletedPaths are the paths of removed pages
	DeletedPaths []string

	// DemotedCount is how many pages had their MemoryTier lowered
	DemotedCount int
}

// ConsolidateOptions controls ConsolidateMemories behavior
type ConsolidateOptions struct {
	// TenantID restricts consolidation to a single namespace (empty = all)
	TenantID string

	// Types restricts which PageTypes are candidates (empty = memory + preference)
	Types []schema.PageType

	// DryRun reports what would be merged without making changes
	DryRun bool
}

// ConsolidateStats summarises a consolidation run
type ConsolidateStats struct {
	// MergedGroups is the number of merge groups processed
	MergedGroups int

	// PagesAbsorbed is how many pages were folded into their merge target
	PagesAbsorbed int

	// DryRun mirrors ConsolidateOptions.DryRun
	DryRun bool
}

// MemoryWiki extends Wiki with memory-system capabilities.
// All existing Wiki users are unaffected — this is a pure superset.
type MemoryWiki interface {
	Wiki

	// StoreMemory writes a memory page, creating or updating by title
	StoreMemory(ctx context.Context, req *StoreMemoryRequest) (*StoreMemoryResult, error)

	// RecallMemory retrieves memory pages matching the query, with access tracking
	RecallMemory(ctx context.Context, query string, opts *RecallOptions) (*RecallResult, error)

	// AppendAuditLog appends an entry to the date-scoped audit log (never overwrites)
	AppendAuditLog(ctx context.Context, entry *AuditEntry) error

	// SetImportance updates the importance score of an existing page
	SetImportance(ctx context.Context, path string, score float64) error

	// SetTTL sets or clears the expiry time of an existing page
	SetTTL(ctx context.Context, path string, expiresAt *time.Time) error

	// RunGC deletes expired pages and recalculates MemoryTier for all pages
	RunGC(ctx context.Context) (*GCResult, error)

	// ConsolidateMemories uses LLM to merge semantically similar memories
	ConsolidateMemories(ctx context.Context, opts *ConsolidateOptions) (*ConsolidateStats, error)
}
