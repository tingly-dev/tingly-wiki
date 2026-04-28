package wiki

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// WikiImpl is the concrete implementation of Wiki
type WikiImpl struct {
	config      *config.Config
	storage     storage.Storage
	llm         llm.LLM
	index       index.Index
	layout      *config.LayoutConfig
	indexLoaded bool
	indexMu     sync.Mutex
}

// New creates a new Wiki instance
func New(cfg *config.Config) (*WikiImpl, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if cfg.Layout == nil {
		cfg.Layout = config.DefaultLayout()
	}

	// Type assert interfaces
	var idx index.Index
	if cfg.Index != nil {
		if i, ok := cfg.Index.(index.Index); ok {
			idx = i
		} else {
			idx = index.NewFullTextIndex()
		}
	} else {
		idx = index.NewFullTextIndex()
	}

	return &WikiImpl{
		config:  cfg,
		storage: cfg.Storage.(storage.Storage),
		llm:     cfg.LLM.(llm.LLM),
		index:   idx,
		layout:  cfg.Layout,
	}, nil
}

// Ingest processes a new source document
func (w *WikiImpl) Ingest(ctx context.Context, source *schema.Source) (*IngestResult, error) {
	result := &IngestResult{
		CreatedPaths: []string{},
		UpdatedPaths: []string{},
	}

	// Assign ID if not set
	if source.ID == "" {
		source.ID = uuid.New().String()
	}

	// Store raw source
	if err := w.storage.WriteSource(ctx, source); err != nil {
		return nil, fmt.Errorf("failed to store source: %w", err)
	}

	// Extract information using LLM
	extracted, err := w.llm.Extract(ctx, source.Content, schema.DefaultSchema())
	if err != nil {
		return nil, fmt.Errorf("failed to extract: %w", err)
	}
	result.ExtractedInfo = extracted

	// Create source summary page
	sourcePath := w.layout.GetSourcePath(source.ID)
	sourcePage := &schema.Page{
		Path: sourcePath,
		Frontmatter: schema.Frontmatter{
			Type:  schema.PageTypeSource,
			Title: source.ID,
			Tags:  []string{"source"},
			Extra: map[string]interface{}{},
		},
		Content:   w.formatSourceSummary(source, extracted),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := w.storage.WritePage(ctx, sourcePage); err != nil {
		return nil, fmt.Errorf("failed to write source page: %w", err)
	}
	result.CreatedPaths = append(result.CreatedPaths, sourcePath)
	result.PagesCreated++

	// Update index
	if err := w.index.Index(ctx, sourcePage); err != nil {
		return nil, fmt.Errorf("failed to index source page: %w", err)
	}

	// Create/update entity pages
	for _, entity := range extracted.Entities {
		entityPath := w.layout.GetEntityPath(entity.Name)
		entityPage := &schema.Page{
			Path: entityPath,
			Frontmatter: schema.Frontmatter{
				Type:    schema.PageTypeEntity,
				Title:   entity.Name,
				Tags:    []string{"entity", entity.Type},
				Sources: []string{source.ID},
			},
			Content:   w.formatEntityPage(entity, extracted),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		entityPage.Frontmatter.Extra = map[string]interface{}{}

		// Check if page exists
		existing, err := w.storage.ReadPage(ctx, entityPath)
		if err == nil {
			// Update existing page
			existing.Content = w.mergeEntityContent(existing.Content, entity, extracted)
			existing.Sources = append(existing.Sources, source.ID)
			existing.UpdatedAt = time.Now()
			if err := w.storage.WritePage(ctx, existing); err != nil {
				return nil, fmt.Errorf("failed to update entity page: %w", err)
			}
			result.UpdatedPaths = append(result.UpdatedPaths, entityPath)
			result.PagesUpdated++
		} else {
			// Create new page
			if err := w.storage.WritePage(ctx, entityPage); err != nil {
				return nil, fmt.Errorf("failed to write entity page: %w", err)
			}
			result.CreatedPaths = append(result.CreatedPaths, entityPath)
			result.PagesCreated++
		}
	}

	// Create/update concept pages
	for _, concept := range extracted.Concepts {
		conceptPath := w.layout.GetConceptPath(concept.Name)
		conceptPage := &schema.Page{
			Path: conceptPath,
			Frontmatter: schema.Frontmatter{
				Type:    schema.PageTypeConcept,
				Title:   concept.Name,
				Tags:    []string{"concept"},
				Sources: []string{source.ID},
			},
			Content:   w.formatConceptPage(concept, extracted),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		conceptPage.Frontmatter.Extra = map[string]interface{}{}

		// Check if page exists
		existing, err := w.storage.ReadPage(ctx, conceptPath)
		if err == nil {
			// Update existing page
			existing.Content = w.mergeConceptContent(existing.Content, concept, extracted)
			existing.Sources = append(existing.Sources, source.ID)
			existing.UpdatedAt = time.Now()
			if err := w.storage.WritePage(ctx, existing); err != nil {
				return nil, fmt.Errorf("failed to update concept page: %w", err)
			}
			result.UpdatedPaths = append(result.UpdatedPaths, conceptPath)
			result.PagesUpdated++
		} else {
			// Create new page
			if err := w.storage.WritePage(ctx, conceptPage); err != nil {
				return nil, fmt.Errorf("failed to write concept page: %w", err)
			}
			result.CreatedPaths = append(result.CreatedPaths, conceptPath)
			result.PagesCreated++
		}
	}

	// Update index file
	if err := w.updateIndex(ctx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	// Update log file
	if err := w.updateLog(ctx, "ingest", source.ID, result); err != nil {
		return nil, fmt.Errorf("failed to update log: %w", err)
	}

	return result, nil
}

// formatSourceSummary formats the source summary page
func (w *WikiImpl) formatSourceSummary(source *schema.Source, extracted *schema.ExtractedInfo) string {
	var sb strings.Builder

	sb.WriteString("# Source: ")
	sb.WriteString(source.ID)
	sb.WriteString("\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString(extracted.Summary)
	sb.WriteString("\n\n")

	if len(extracted.KeyPoints) > 0 {
		sb.WriteString("## Key Points\n\n")
		for _, point := range extracted.KeyPoints {
			sb.WriteString("- ")
			sb.WriteString(point)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(extracted.Entities) > 0 {
		sb.WriteString("## Entities\n\n")
		for _, entity := range extracted.Entities {
			sb.WriteString("- [[")
			sb.WriteString(entity.Name)
			sb.WriteString("]] (")
			sb.WriteString(entity.Type)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	if len(extracted.Concepts) > 0 {
		sb.WriteString("## Concepts\n\n")
		for _, concept := range extracted.Concepts {
			sb.WriteString("- [[")
			sb.WriteString(concept.Name)
			sb.WriteString("]]\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatEntityPage formats an entity page
func (w *WikiImpl) formatEntityPage(entity schema.Entity, extracted *schema.ExtractedInfo) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(entity.Name)
	sb.WriteString("\n\n")

	if entity.Description != "" {
		sb.WriteString(entity.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Type\n\n")
	sb.WriteString(entity.Type)
	sb.WriteString("\n\n")

	return sb.String()
}

// formatConceptPage formats a concept page
func (w *WikiImpl) formatConceptPage(concept schema.Concept, extracted *schema.ExtractedInfo) string {
	var sb strings.Builder

	sb.WriteString("# ")
	sb.WriteString(concept.Name)
	sb.WriteString("\n\n")

	sb.WriteString(concept.Description)
	sb.WriteString("\n\n")

	return sb.String()
}

// mergeEntityContent merges new content into existing entity page
func (w *WikiImpl) mergeEntityContent(existing string, entity schema.Entity, extracted *schema.ExtractedInfo) string {
	// Simple implementation: append new info
	return existing + "\n\n## Additional Information\n\n" + entity.Description
}

// mergeConceptContent merges new content into existing concept page
func (w *WikiImpl) mergeConceptContent(existing string, concept schema.Concept, extracted *schema.ExtractedInfo) string {
	// Simple implementation: append new info
	return existing + "\n\n## Additional Information\n\n" + concept.Description
}

// updateIndex updates the index.md file
func (w *WikiImpl) updateIndex(ctx context.Context) error {
	// List all pages
	pages, err := w.storage.ListPages(ctx, &storage.ListOptions{})
	if err != nil {
		return err
	}

	// Group by type
	entities := []*schema.Page{}
	concepts := []*schema.Page{}
	sources := []*schema.Page{}

	for _, page := range pages {
		switch page.Type {
		case schema.PageTypeEntity:
			entities = append(entities, page)
		case schema.PageTypeConcept:
			concepts = append(concepts, page)
		case schema.PageTypeSource:
			sources = append(sources, page)
		}
	}

	// Build index content
	var sb strings.Builder
	sb.WriteString("# Wiki Index\n\n")

	if len(entities) > 0 {
		sb.WriteString("## Entities\n\n")
		for _, page := range entities {
			sb.WriteString("- [")
			sb.WriteString(page.Title)
			sb.WriteString("](")
			sb.WriteString(page.Path)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	if len(concepts) > 0 {
		sb.WriteString("## Concepts\n\n")
		for _, page := range concepts {
			sb.WriteString("- [")
			sb.WriteString(page.Title)
			sb.WriteString("](")
			sb.WriteString(page.Path)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	if len(sources) > 0 {
		sb.WriteString("## Sources\n\n")
		for _, page := range sources {
			sb.WriteString("- [")
			sb.WriteString(page.Title)
			sb.WriteString("](")
			sb.WriteString(page.Path)
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	// Write index page
	indexPage := &schema.Page{
		Path: w.layout.IndexPath,
		Frontmatter: schema.Frontmatter{
			Type:  schema.PageTypeSynthesis,
			Title: "Index",
		},
		Content:   sb.String(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	indexPage.Frontmatter.Extra = map[string]interface{}{}

	return w.storage.WritePage(ctx, indexPage)
}

// updateLog updates the log.md file
func (w *WikiImpl) updateLog(ctx context.Context, action, id string, result *IngestResult) error {
	// Append to log
	logPath := w.layout.LogPath

	// Try to read existing log
	existing, err := w.storage.ReadPage(ctx, logPath)
	var content string
	if err == nil {
		content = existing.Content
	}

	// Append new entry
	content += fmt.Sprintf("## [%s] %s | %s\n\n", time.Now().Format("2006-01-02"), action, id)
	content += fmt.Sprintf("- Pages created: %d\n", result.PagesCreated)
	content += fmt.Sprintf("- Pages updated: %d\n\n", result.PagesUpdated)

	// Write log page
	logPage := &schema.Page{
		Path: logPath,
		Frontmatter: schema.Frontmatter{
			Type:  schema.PageTypeSynthesis,
			Title: "Log",
		},
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	logPage.Frontmatter.Extra = map[string]interface{}{}

	return w.storage.WritePage(ctx, logPage)
}

// GetPage retrieves a single page
func (w *WikiImpl) GetPage(ctx context.Context, path string) (*schema.Page, error) {
	return w.storage.ReadPage(ctx, path)
}

// UpdatePage updates an existing page
func (w *WikiImpl) UpdatePage(ctx context.Context, page *schema.Page) error {
	page.UpdatedAt = time.Now()
	if err := w.storage.WritePage(ctx, page); err != nil {
		return err
	}
	// Update index
	return w.index.Index(ctx, page)
}

// ListPages lists pages matching criteria
func (w *WikiImpl) ListPages(ctx context.Context, opts *ListOptions) ([]*schema.Page, error) {
	storageOpts := &storage.ListOptions{}
	if opts != nil {
		storageOpts.Type = opts.Type
		storageOpts.Prefix = opts.Prefix
		storageOpts.Limit = opts.Limit
		storageOpts.Tags = opts.Tags
	}
	return w.storage.ListPages(ctx, storageOpts)
}

// ensureIndexLoaded ensures the index is loaded (lazy initialization)
func (w *WikiImpl) ensureIndexLoaded(ctx context.Context) error {
	w.indexMu.Lock()
	defer w.indexMu.Unlock()

	if w.indexLoaded {
		return nil
	}

	// List all pages
	pages, err := w.storage.ListPages(ctx, &storage.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pages: %w", err)
	}

	// Index each page
	for _, page := range pages {
		if err := w.index.Index(ctx, page); err != nil {
			return fmt.Errorf("failed to index page %s: %w", page.Path, err)
		}
	}

	w.indexLoaded = true
	return nil
}

// RebuildIndex rebuilds the search index from existing pages
func (w *WikiImpl) RebuildIndex(ctx context.Context) error {
	w.indexMu.Lock()
	defer w.indexMu.Unlock()

	// Clear existing index
	if err := w.index.Close(); err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	// Reset loaded flag
	w.indexLoaded = false

	// Reload index
	return w.ensureIndexLoaded(ctx)
}

// Close releases resources
func (w *WikiImpl) Close() error {
	var errs []error

	if w.storage != nil {
		if err := w.storage.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if w.index != nil {
		if err := w.index.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing wiki: %v", errs)
	}

	return nil
}
