package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// TestMemoryStorage tests memory storage
func TestMemoryStorage(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	// Test write and read page
	page := &schema.Page{
		Path: "test/page.md",
		Frontmatter: schema.Frontmatter{
			Type:  schema.PageTypeEntity,
			Title: "Test Page",
			Tags:  []string{"test", "entity"},
			Extra: map[string]interface{}{},
		},
		Content:   "# Test Page\n\nThis is a test page.",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := storage.WritePage(ctx, page)
	if err != nil {
		t.Fatalf("Failed to write page: %v", err)
	}

	// Read page back
	readPage, err := storage.ReadPage(ctx, "test/page.md")
	if err != nil {
		t.Fatalf("Failed to read page: %v", err)
	}

	if readPage.Title != "Test Page" {
		t.Errorf("Expected title 'Test Page', got '%s'", readPage.Title)
	}

	if readPage.Type != schema.PageTypeEntity {
		t.Errorf("Expected type %s, got %s", schema.PageTypeEntity, readPage.Type)
	}

	// Test list pages
	pages, err := storage.ListPages(ctx, &ListOptions{Type: &page.Type})
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("Expected 1 page, got %d", len(pages))
	}

	// Test delete page
	err = storage.DeletePage(ctx, "test/page.md")
	if err != nil {
		t.Fatalf("Failed to delete page: %v", err)
	}

	// Verify deleted
	_, err = storage.ReadPage(ctx, "test/page.md")
	if err == nil {
		t.Error("Expected error when reading deleted page")
	}

	// Test source operations
	source := &schema.Source{
		ID:      "test-source",
		Type:    schema.SourceTypeText,
		Content: "Test source content",
	}

	err = storage.WriteSource(ctx, source)
	if err != nil {
		t.Fatalf("Failed to write source: %v", err)
	}

	readSource, err := storage.ReadSource(ctx, "test-source")
	if err != nil {
		t.Fatalf("Failed to read source: %v", err)
	}

	if readSource.Content != "Test source content" {
		t.Errorf("Expected content 'Test source content', got '%s'", readSource.Content)
	}

	// Cleanup
	storage.Close()
}

// TestMemoryStorageConcurrent tests concurrent access
func TestMemoryStorageConcurrent(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	// Write multiple pages concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			page := &schema.Page{
				Path: fmt.Sprintf("test/page%d.md", idx),
				Frontmatter: schema.Frontmatter{
					Type:  schema.PageTypeEntity,
					Title: fmt.Sprintf("Page %d", idx),
					Extra: map[string]interface{}{},
				},
				Content:   fmt.Sprintf("Content %d", idx),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			storage.WritePage(ctx, page)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all pages were written
	pages, err := storage.ListPages(ctx, &ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}

	if len(pages) != 10 {
		t.Errorf("Expected 10 pages, got %d", len(pages))
	}

	storage.Close()
}
