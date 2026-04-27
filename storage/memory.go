package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// MemoryStorage implements in-memory storage for testing
type MemoryStorage struct {
	mu      sync.RWMutex
	pages   map[string]*schema.Page
	sources map[string]*schema.Source
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		pages:   make(map[string]*schema.Page),
		sources: make(map[string]*schema.Source),
	}
}

// ReadPage reads a page from memory
func (m *MemoryStorage) ReadPage(ctx context.Context, path string) (*schema.Page, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	page, ok := m.pages[path]
	if !ok {
		return nil, fmt.Errorf("page not found: %s", path)
	}
	return page, nil
}

// WritePage writes a page to memory
func (m *MemoryStorage) WritePage(ctx context.Context, page *schema.Page) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Copy page to avoid mutation
	pageCopy := *page
	m.pages[page.Path] = &pageCopy
	return nil
}

// DeletePage removes a page from memory
func (m *MemoryStorage) DeletePage(ctx context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.pages, path)
	return nil
}

// ListPages lists all pages
func (m *MemoryStorage) ListPages(ctx context.Context, opts *ListOptions) ([]*schema.Page, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*schema.Page

	for _, page := range m.pages {
		// Apply filters
		if opts != nil {
			if opts.Type != nil && page.Type != *opts.Type {
				continue
			}
			if opts.Prefix != "" && !strings.HasPrefix(page.Path, opts.Prefix) {
				continue
			}
			if len(opts.Tags) > 0 {
				if !hasAnyTag(page, opts.Tags) {
					continue
				}
			}
		}
		result = append(result, page)
	}

	// Apply limit
	if opts != nil && opts.Limit > 0 && len(result) > opts.Limit {
		result = result[:opts.Limit]
	}

	return result, nil
}

// ReadSource reads a source from memory
func (m *MemoryStorage) ReadSource(ctx context.Context, id string) (*schema.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	source, ok := m.sources[id]
	if !ok {
		return nil, fmt.Errorf("source not found: %s", id)
	}
	return source, nil
}

// WriteSource writes a source to memory
func (m *MemoryStorage) WriteSource(ctx context.Context, source *schema.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Copy source to avoid mutation
	sourceCopy := *source
	m.sources[source.ID] = &sourceCopy
	return nil
}

// Close is a no-op for memory storage
func (m *MemoryStorage) Close() error {
	return nil
}

// hasAnyTag checks if a page has any of the given tags
func hasAnyTag(page *schema.Page, tags []string) bool {
	tagSet := make(map[string]bool)
	for _, tag := range page.Tags {
		tagSet[tag] = true
	}
	for _, tag := range tags {
		if tagSet[tag] {
			return true
		}
	}
	return false
}
