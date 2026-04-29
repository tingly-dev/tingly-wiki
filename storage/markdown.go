package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// MarkdownStorage implements markdown file storage
type MarkdownStorage struct {
	mu     sync.RWMutex
	root   string
	layout *config.LayoutConfig
	parser *schema.Parser
}

// NewMarkdownStorage creates a new markdown storage
func NewMarkdownStorage(root string, layout *config.LayoutConfig) (*MarkdownStorage, error) {
	if layout == nil {
		layout = config.DefaultLayout()
	}

	// Ensure root directory exists
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &MarkdownStorage{
		root:   root,
		layout: layout,
		parser: schema.NewParser(),
	}, nil
}

// ReadPage reads a page from disk
func (m *MarkdownStorage) ReadPage(ctx context.Context, path string) (*schema.Page, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fullPath := filepath.Join(m.root, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("page not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	page, err := m.parser.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse page: %w", err)
	}

	page.Path = path
	return page, nil
}

// WritePage writes a page to disk
func (m *MarkdownStorage) WritePage(ctx context.Context, page *schema.Page) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(page.Path)
	if dir != "" && dir != "." {
		fullDir := filepath.Join(m.root, dir)
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Serialize page
	content, err := m.parser.Serialize(page)
	if err != nil {
		return fmt.Errorf("failed to serialize page: %w", err)
	}

	// Write to disk
	fullPath := filepath.Join(m.root, page.Path)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write page: %w", err)
	}

	return nil
}

// DeletePage removes a page from disk
func (m *MarkdownStorage) DeletePage(ctx context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fullPath := filepath.Join(m.root, path)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}

	return nil
}

// ListPages lists all pages
func (m *MarkdownStorage) ListPages(ctx context.Context, opts *ListOptions) ([]*schema.Page, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*schema.Page

	err := filepath.Walk(m.root, func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(fullPath, ".md") {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(m.root, fullPath)
		if err != nil {
			return err
		}

		// Skip index and log files
		if relPath == m.layout.IndexPath || relPath == m.layout.LogPath {
			return nil
		}

		// Read and parse page
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}

		page, err := m.parser.Parse(string(content))
		if err != nil {
			return err
		}

		page.Path = relPath

		// Skip expired pages (logical delete)
		if page.ExpiresAt != nil && page.ExpiresAt.Before(time.Now()) {
			return nil
		}

		// Apply filters
		if opts != nil {
			if opts.Type != nil && page.Type != *opts.Type {
				return nil
			}
			if opts.Prefix != "" && !strings.HasPrefix(page.Path, opts.Prefix) {
				return nil
			}
			if len(opts.Tags) > 0 && !hasAnyTag(page, opts.Tags) {
				return nil
			}
		}

		result = append(result, page)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}

	// Apply limit
	if opts != nil && opts.Limit > 0 && len(result) > opts.Limit {
		result = result[:opts.Limit]
	}

	return result, nil
}

// ReadSource reads a source from disk (stored as JSON in raw/ directory)
func (m *MarkdownStorage) ReadSource(ctx context.Context, id string) (*schema.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fullPath := filepath.Join(m.root, m.layout.RawDir, id+".json")
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("source not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	var source schema.Source
	if err := json.Unmarshal(data, &source); err != nil {
		return nil, fmt.Errorf("failed to parse source %s: %w", id, err)
	}
	return &source, nil
}

// WriteSource writes a source to disk
func (m *MarkdownStorage) WriteSource(ctx context.Context, source *schema.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rawDir := filepath.Join(m.root, m.layout.RawDir)
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return fmt.Errorf("failed to create raw directory: %w", err)
	}

	data, err := json.Marshal(source)
	if err != nil {
		return fmt.Errorf("failed to serialize source: %w", err)
	}

	fullPath := filepath.Join(rawDir, source.ID+".json")
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write source: %w", err)
	}

	return nil
}

// Close is a no-op for file storage
func (m *MarkdownStorage) Close() error {
	return nil
}
