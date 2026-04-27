package wiki

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-wiki/llm"
)

// Lint performs health checks on the wiki
func (w *WikiImpl) Lint(ctx context.Context, opts *LintOptions) (*llm.LintReport, error) {
	if opts == nil {
		opts = &LintOptions{
			CheckContradictions: true,
			CheckOrphans:        true,
			CheckStale:          false, // Expensive
			CheckMissingRefs:    true,
		}
	}

	// Load all pages
	pages, err := w.storage.ListPages(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}

	// Run LLM lint
	report, err := w.llm.Lint(ctx, pages)
	if err != nil {
		return nil, fmt.Errorf("failed to run LLM lint: %w", err)
	}

	return report, nil
}
