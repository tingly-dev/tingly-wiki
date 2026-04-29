package llm

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// LLM defines the AI operations
type LLM interface {
	// Extract extracts structured information from content
	Extract(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error)

	// Summarize creates a summary of content
	Summarize(ctx context.Context, content string) (string, error)

	// Query answers a question with context
	Query(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error)

	// Lint performs health analysis on pages
	Lint(ctx context.Context, pages []*schema.Page) (*LintReport, error)

	// Consolidate merges a group of related pages into a single coherent page.
	// It returns the merged content, a suggested title, and which pages should be absorbed.
	Consolidate(ctx context.Context, pages []*schema.Page) (*ConsolidateResult, error)

	// Embed generates a dense vector representation of text for semantic retrieval.
	// Returns ErrEmbeddingNotSupported if the adapter does not support embeddings.
	Embed(ctx context.Context, text string) ([]float32, error)

	// ExtractMemoryFacts extracts atomic (subject, predicate, object) facts from content.
	// pageType is a hint that lets the LLM calibrate extraction style.
	ExtractMemoryFacts(ctx context.Context, content string, pageType schema.PageType) ([]schema.MemoryFact, error)

	// RateImportance asks the LLM how important content is for future recall (0.0–1.0).
	RateImportance(ctx context.Context, content string) (float64, error)
}

// ErrEmbeddingNotSupported is returned by LLM adapters that do not support embeddings.
var ErrEmbeddingNotSupported = fmt.Errorf("embedding not supported by this LLM adapter")

// ConsolidateResult is returned by LLM.Consolidate
type ConsolidateResult struct {
	// MergedContent is the unified markdown body
	MergedContent string `json:"merged_content"`

	// SuggestedTitle is the LLM-proposed title for the merged page
	SuggestedTitle string `json:"suggested_title"`

	// AbsorbedPaths are the paths that should be deleted after merging
	AbsorbedPaths []string `json:"absorbed_paths"`

	// ImportanceScore is the LLM-suggested importance for the merged page (0–1)
	ImportanceScore float64 `json:"importance_score"`
}

// LintReport is the result of a health check
type LintReport struct {
	// Issues found during linting
	Issues []LintIssue `json:"issues"`

	// Suggestions for improvement
	Suggestions []string `json:"suggestions"`

	// Pages that should be created
	PagesToCreate []PageSuggestion `json:"pages_to_create"`

	// Pages that should be updated
	PagesToUpdate []PageUpdate `json:"pages_to_update"`
}

// LintIssue represents a problem found
type LintIssue struct {
	// Type is the type of issue
	Type LintIssueType `json:"type"`

	// Severity is how bad the issue is
	Severity LintSeverity `json:"severity"`

	// Message describes the issue
	Message string `json:"message"`

	// Pages involved in the issue
	Pages []string `json:"pages,omitempty"`
}

// LintIssueType is the type of lint issue
type LintIssueType string

const (
	LintIssueTypeContradiction LintIssueType = "contradiction"
	LintIssueTypeOrphan        LintIssueType = "orphan"
	LintIssueTypeStale         LintIssueType = "stale"
	LintIssueTypeMissingRef    LintIssueType = "missing_ref"
)

// LintSeverity is the severity of a lint issue
type LintSeverity string

const (
	LintSeverityError   LintSeverity = "error"
	LintSeverityWarning LintSeverity = "warning"
	LintSeverityInfo    LintSeverity = "info"
)

// PageSuggestion is a suggestion for a new page
type PageSuggestion struct {
	// Type is the type of page
	Type schema.PageType `json:"type"`

	// Title is the suggested title
	Title string `json:"title"`

	// Reason why this page should be created
	Reason string `json:"reason"`

	// Sources to reference
	Sources []string `json:"sources,omitempty"`
}

// PageUpdate is a suggestion for updating a page
type PageUpdate struct {
	// Path is the page to update
	Path string `json:"path"`

	// Reason for the update
	Reason string `json:"reason"`

	// Specific changes needed
	Changes []string `json:"changes"`
}
