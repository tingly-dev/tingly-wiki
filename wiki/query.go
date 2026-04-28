package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// Query answers questions using wiki content
func (w *WikiImpl) Query(ctx context.Context, query string, opts *QueryOptions) (*QueryResult, error) {
	if opts == nil {
		opts = &QueryOptions{}
	}

	// Ensure index is loaded (lazy initialization)
	if err := w.ensureIndexLoaded(ctx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	// Search index for relevant pages
	searchOpts := &index.SearchOptions{
		Limit: opts.Limit,
	}

	if opts.Limit == 0 {
		searchOpts.Limit = 10 // Default limit
	}

	searchResult, err := w.index.Search(ctx, query, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	// Read relevant pages
	var contextPages []string
	pagesRead := []string{}

	for _, item := range searchResult.Results {
		// Load full page (index only has path, need to load full content)
		page, err := w.storage.ReadPage(ctx, item.Page.Path)
		if err != nil {
			continue
		}
		item.Page = page

		// Build context string
		contextStr := fmt.Sprintf("# %s\n\n%s", item.Page.Title, item.Page.Content)
		contextPages = append(contextPages, contextStr)
		pagesRead = append(pagesRead, item.Page.Path)
	}

	// Query LLM
	answer, err := w.llm.Query(ctx, query, contextPages)
	if err != nil {
		return nil, fmt.Errorf("failed to query LLM: %w", err)
	}

	result := &QueryResult{
		Answer:    answer.Answer,
		Citations: answer.Citations,
		PagesRead: pagesRead,
	}

	// Archive result if requested
	if opts.ArchiveResult && opts.ArchivePath != "" {
		archivePage := &schema.Page{
			Path: opts.ArchivePath,
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeSynthesis,
				Title: strings.TrimSuffix(opts.ArchivePath, ".md"),
				Tags:  []string{"query", "synthesis"},
			},
			Content: w.formatQueryResult(query, answer),
		}
		archivePage.Frontmatter.Extra = map[string]interface{}{}

		if err := w.storage.WritePage(ctx, archivePage); err != nil {
			return nil, fmt.Errorf("failed to archive result: %w", err)
		}

		result.ArchivedPath = opts.ArchivePath
	}

	return result, nil
}

// formatQueryResult formats a query result as a page
func (w *WikiImpl) formatQueryResult(query string, answer *schema.QueryAnswer) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(query)
	sb.WriteString("\n\n")

	sb.WriteString("## Question\n\n")
	sb.WriteString(query)
	sb.WriteString("\n\n")

	sb.WriteString("## Answer\n\n")
	sb.WriteString(answer.Answer)
	sb.WriteString("\n\n")

	if len(answer.Citations) > 0 {
		sb.WriteString("## Sources\n\n")
		for _, citation := range answer.Citations {
			sb.WriteString("- [")
			sb.WriteString(citation.Title)
			sb.WriteString("](")
			sb.WriteString(citation.Path)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
