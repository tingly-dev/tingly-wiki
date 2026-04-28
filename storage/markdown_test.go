package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// TestMarkdownStorage tests markdown storage
func TestMarkdownStorage(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wiki-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewMarkdownStorage(tmpDir, config.DefaultLayout())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Test write and read page
	page := &schema.Page{
		Path: "entities/test-page.md",
		Frontmatter: schema.Frontmatter{
			Type:    schema.PageTypeEntity,
			Title:   "Test Page",
			Tags:    []string{"test", "entity"},
			Sources: []string{"source-1"},
			Related: []string{"related-1"},
			Extra:   map[string]interface{}{},
		},
		Content: "# Test Page\n\nThis is a test page.",
	}

	err = storage.WritePage(ctx, page)
	if err != nil {
		t.Fatalf("Failed to write page: %v", err)
	}

	// Read page back
	readPage, err := storage.ReadPage(ctx, "entities/test-page.md")
	if err != nil {
		t.Fatalf("Failed to read page: %v", err)
	}

	if readPage.Title != "Test Page" {
		t.Errorf("Expected title 'Test Page', got '%s'", readPage.Title)
	}

	if readPage.Type != schema.PageTypeEntity {
		t.Errorf("Expected type %s, got %s", schema.PageTypeEntity, readPage.Type)
	}

	if len(readPage.Sources) != 1 {
		t.Errorf("Expected 1 source, got %d", len(readPage.Sources))
	}

	if len(readPage.Related) != 1 {
		t.Errorf("Expected 1 related, got %d", len(readPage.Related))
	}

	// Verify file format (check for proper frontmatter delimiters)
	fullPath := filepath.Join(tmpDir, "entities/test-page.md")
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	fileContent := string(content)
	// Check that frontmatter is properly formatted
	if fileContent[0:4] != "---\n" {
		t.Errorf("File should start with '---\\n', got %q", fileContent[0:4])
	}

	// Check that there's a newline before closing ---
	// This ensures we don't have the bug where --- is appended directly after content
	lastFmIdx := indexOf(fileContent, "\n---\n")
	if lastFmIdx == -1 {
		t.Error("Could not find closing frontmatter delimiter")
	}

	// Verify no consecutive --- without content between (the bug we saw: "value------")
	if indexOf(fileContent, "------") != -1 {
		t.Error("Found malformed frontmatter: consecutive '---' without newline")
	}

	// Test list pages
	entityType := schema.PageTypeEntity
	pages, err := storage.ListPages(ctx, &ListOptions{Type: &entityType})
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("Expected 1 page, got %d", len(pages))
	}

	// Test delete page
	err = storage.DeletePage(ctx, "entities/test-page.md")
	if err != nil {
		t.Fatalf("Failed to delete page: %v", err)
	}

	// Verify deleted
	_, err = storage.ReadPage(ctx, "entities/test-page.md")
	if err == nil {
		t.Error("Expected error when reading deleted page")
	}
}

// TestMarkdownStorageSerialization tests round-trip serialization
func TestMarkdownStorageSerialization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wiki-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewMarkdownStorage(tmpDir, config.DefaultLayout())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create pages with different types
	pages := []*schema.Page{
		{
			Path: "entities/openai.md",
			Frontmatter: schema.Frontmatter{
				Type:    schema.PageTypeEntity,
				Title:   "OpenAI",
				Tags:    []string{"entity", "organization"},
				Sources: []string{"source-1"},
				Extra:   map[string]interface{}{},
			},
			Content: "# OpenAI\n\nAn AI research organization.",
		},
		{
			Path: "concepts/agi.md",
			Frontmatter: schema.Frontmatter{
				Type:    schema.PageTypeConcept,
				Title:   "Artificial General Intelligence",
				Tags:    []string{"concept"},
				Sources: []string{"source-1", "source-2"},
				Related: []string{"entities/openai.md"},
				Extra:   map[string]interface{}{},
			},
			Content: "# AGI\n\nA type of AI that matches human cognitive abilities.",
		},
		{
			Path: "sources/source-1.md",
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeSource,
				Title: "source-1",
				Tags:  []string{"source"},
				Extra: map[string]interface{}{},
			},
			Content: "# Source: source-1\n\nOriginal content here.",
		},
	}

	// Write all pages
	for _, page := range pages {
		if err := storage.WritePage(ctx, page); err != nil {
			t.Fatalf("Failed to write page %s: %v", page.Path, err)
		}
	}

	// Read back and verify
	for _, original := range pages {
		read, err := storage.ReadPage(ctx, original.Path)
		if err != nil {
			t.Fatalf("Failed to read page %s: %v", original.Path, err)
		}

		if read.Title != original.Title {
			t.Errorf("Page %s: expected title %q, got %q", original.Path, original.Title, read.Title)
		}

		if read.Type != original.Type {
			t.Errorf("Page %s: expected type %q, got %q", original.Path, original.Type, read.Type)
		}

		if len(read.Sources) != len(original.Sources) {
			t.Errorf("Page %s: expected %d sources, got %d", original.Path, len(original.Sources), len(read.Sources))
		}

		if len(read.Related) != len(original.Related) {
			t.Errorf("Page %s: expected %d related, got %d", original.Path, len(original.Related), len(read.Related))
		}
	}

	// Test listing by type
	entityType := schema.PageTypeEntity
	entityPages, err := storage.ListPages(ctx, &ListOptions{Type: &entityType})
	if err != nil {
		t.Fatalf("Failed to list entities: %v", err)
	}

	if len(entityPages) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(entityPages))
	}

	conceptType := schema.PageTypeConcept
	conceptPages, err := storage.ListPages(ctx, &ListOptions{Type: &conceptType})
	if err != nil {
		t.Fatalf("Failed to list concepts: %v", err)
	}

	if len(conceptPages) != 1 {
		t.Errorf("Expected 1 concept, got %d", len(conceptPages))
	}

	// Test listing by prefix
	prefixPages, err := storage.ListPages(ctx, &ListOptions{Prefix: "entities/"})
	if err != nil {
		t.Fatalf("Failed to list by prefix: %v", err)
	}

	if len(prefixPages) != 1 {
		t.Errorf("Expected 1 page with entities/ prefix, got %d", len(prefixPages))
	}
}

// TestMarkdownStorageListOptions tests various list options
func TestMarkdownStorageListOptions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wiki-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewMarkdownStorage(tmpDir, config.DefaultLayout())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Create test pages
	pages := []*schema.Page{
		{
			Path: "entities/page1.md",
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeEntity,
				Title: "Page 1",
				Tags:  []string{"entity", "person"},
				Extra: map[string]interface{}{},
			},
			Content: "# Page 1",
		},
		{
			Path: "entities/page2.md",
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeEntity,
				Title: "Page 2",
				Tags:  []string{"entity", "organization"},
				Extra: map[string]interface{}{},
			},
			Content: "# Page 2",
		},
		{
			Path: "concepts/concept1.md",
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeConcept,
				Title: "Concept 1",
				Tags:  []string{"concept"},
				Extra: map[string]interface{}{},
			},
			Content: "# Concept 1",
		},
	}

	for _, page := range pages {
		if err := storage.WritePage(ctx, page); err != nil {
			t.Fatalf("Failed to write page: %v", err)
		}
	}

	// Test filter by type
	entityType := schema.PageTypeEntity
	entities, err := storage.ListPages(ctx, &ListOptions{Type: &entityType})
	if err != nil {
		t.Fatalf("Failed to list by type: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(entities))
	}

	// Test filter by prefix
	prefixPages, err := storage.ListPages(ctx, &ListOptions{Prefix: "entities/"})
	if err != nil {
		t.Fatalf("Failed to list by prefix: %v", err)
	}
	if len(prefixPages) != 2 {
		t.Errorf("Expected 2 pages with entities/ prefix, got %d", len(prefixPages))
	}

	// Test filter by tag
	tagPages, err := storage.ListPages(ctx, &ListOptions{Tags: []string{"person"}})
	if err != nil {
		t.Fatalf("Failed to list by tag: %v", err)
	}
	if len(tagPages) != 1 {
		t.Errorf("Expected 1 page with 'person' tag, got %d", len(tagPages))
	}

	// Test limit
	limitedPages, err := storage.ListPages(ctx, &ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("Failed to list with limit: %v", err)
	}
	if len(limitedPages) != 1 {
		t.Errorf("Expected 1 page with limit, got %d", len(limitedPages))
	}
}

// TestMarkdownStorageSubdirectories tests that subdirectories are created
func TestMarkdownStorageSubdirectories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wiki-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewMarkdownStorage(tmpDir, config.DefaultLayout())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Write pages in nested directories
	paths := []string{
		"entities/people/sam-altman.md",
		"entities/organizations/openai.md",
		"concepts/agi/safety.md",
	}

	for _, path := range paths {
		page := &schema.Page{
			Path: path,
			Frontmatter: schema.Frontmatter{
				Type:  schema.PageTypeEntity,
				Title: path,
				Extra: map[string]interface{}{},
			},
			Content: "# " + path,
		}
		if err := storage.WritePage(ctx, page); err != nil {
			t.Fatalf("Failed to write page %s: %v", path, err)
		}
	}

	// Verify all files exist
	for _, path := range paths {
		fullPath := filepath.Join(tmpDir, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("File was not created: %s", fullPath)
		}
	}

	// Verify we can list all pages
	pages, err := storage.ListPages(ctx, &ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}
	if len(pages) != len(paths) {
		t.Errorf("Expected %d pages, got %d", len(paths), len(pages))
	}
}

// indexOf returns the index of substr in s, or -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
