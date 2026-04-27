package llm

import (
	"context"

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
