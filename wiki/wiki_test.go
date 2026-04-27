package wiki

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// TestIngest tests the ingest workflow
func TestIngest(t *testing.T) {
	// Setup mock LLM
	mockLLM := &llm.MockLLM{
		ExtractFunc: func(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
			return &schema.ExtractedInfo{
				Summary: "Test summary of the source content",
				Entities: []schema.Entity{
					{Name: "OpenAI", Type: "organization", Description: "An AI research organization"},
					{Name: "GPT", Type: "model", Description: "A language model"},
				},
				Concepts: []schema.Concept{
					{Name: "Language Model", Description: "A model that processes and generates text"},
					{Name: "API", Description: "Application Programming Interface"},
				},
				KeyPoints: []string{
					"OpenAI develops AI technologies",
					"GPT is a language model",
					"APIs allow programmatic access",
				},
			}, nil
		},
		QueryFunc: func(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
			return &schema.QueryAnswer{
				Answer: "This is a test answer",
				Citations: []schema.Citation{
					{Path: "test/source.md", Title: "Test Source", Relevance: 1.0},
				},
			}, nil
		},
		LintFunc: func(ctx context.Context, pages []*schema.Page) (*llm.LintReport, error) {
			return &llm.LintReport{}, nil
		},
	}

	// Setup memory storage
	memStorage := storage.NewMemoryStorage()

	// Create wiki
	cfg := &config.Config{
		Storage: memStorage,
		LLM:     mockLLM,
		Layout:  config.DefaultLayout(),
	}

	wiki, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create wiki: %v", err)
	}

	// Test ingest
	source := &schema.Source{
		ID:      "test-source-1",
		Type:    schema.SourceTypeText,
		Content: "OpenAI develops GPT, which is a language model. It provides APIs for developers to use these capabilities.",
	}

	result, err := wiki.Ingest(context.Background(), source)
	if err != nil {
		t.Fatalf("Failed to ingest: %v", err)
	}

	// Verify results
	if result.PagesCreated < 1 {
		t.Errorf("Expected at least 1 page created, got %d", result.PagesCreated)
	}

	// Verify source page exists
	sourcePage, err := wiki.GetPage(context.Background(), "sources/test-source-1.md")
	if err != nil {
		t.Fatalf("Failed to get source page: %v", err)
	}

	if sourcePage.Type != schema.PageTypeSource {
		t.Errorf("Expected page type %s, got %s", schema.PageTypeSource, sourcePage.Type)
	}

	// Verify entity page exists
	entityPage, err := wiki.GetPage(context.Background(), "entities/openai.md")
	if err != nil {
		t.Fatalf("Failed to get entity page: %v", err)
	}

	if entityPage.Type != schema.PageTypeEntity {
		t.Errorf("Expected page type %s, got %s", schema.PageTypeEntity, entityPage.Type)
	}

	// Verify concept page exists
	conceptPage, err := wiki.GetPage(context.Background(), "concepts/language-model.md")
	if err != nil {
		t.Fatalf("Failed to get concept page: %v", err)
	}

	if conceptPage.Type != schema.PageTypeConcept {
		t.Errorf("Expected page type %s, got %s", schema.PageTypeConcept, conceptPage.Type)
	}

	// Cleanup
	wiki.Close()
}

// TestQuery tests the query workflow
func TestQuery(t *testing.T) {
	// Setup mock LLM
	mockLLM := &llm.MockLLM{
		ExtractFunc: func(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
			return &schema.ExtractedInfo{
				Summary: "Test summary",
				Entities: []schema.Entity{
					{Name: "TestEntity", Type: "organization"},
				},
			}, nil
		},
		QueryFunc: func(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
			return &schema.QueryAnswer{
				Answer: "TestEntity is a test organization for unit testing.",
				Citations: []schema.Citation{
					{Path: "entities/testentity.md", Title: "TestEntity", Relevance: 1.0},
				},
			}, nil
		},
		LintFunc: func(ctx context.Context, pages []*schema.Page) (*llm.LintReport, error) {
			return &llm.LintReport{}, nil
		},
	}

	// Setup memory storage
	memStorage := storage.NewMemoryStorage()

	// Create wiki
	cfg := &config.Config{
		Storage: memStorage,
		LLM:     mockLLM,
		Layout:  config.DefaultLayout(),
	}

	wiki, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create wiki: %v", err)
	}

	// First ingest some content
	source := &schema.Source{
		ID:      "test-source-query",
		Type:    schema.SourceTypeText,
		Content: "TestEntity is a test organization.",
	}

	_, err = wiki.Ingest(context.Background(), source)
	if err != nil {
		t.Fatalf("Failed to ingest: %v", err)
	}

	// Test query
	queryResult, err := wiki.Query(context.Background(), "What is TestEntity?", &QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	if queryResult.Answer == "" {
		t.Error("Expected non-empty answer")
	}

	if len(queryResult.Citations) == 0 {
		t.Error("Expected at least one citation")
	}

	// Cleanup
	wiki.Close()
}

// TestListPages tests listing pages
func TestListPages(t *testing.T) {
	// Setup mock LLM
	mockLLM := &llm.MockLLM{
		ExtractFunc: func(ctx context.Context, content string, schemaDef *schema.Schema) (*schema.ExtractedInfo, error) {
			return &schema.ExtractedInfo{
				Summary: "Test summary",
				Entities: []schema.Entity{
					{Name: "Entity1", Type: "organization"},
					{Name: "Entity2", Type: "organization"},
				},
				Concepts: []schema.Concept{
					{Name: "Concept1", Description: "Test concept"},
				},
			}, nil
		},
		QueryFunc: func(ctx context.Context, question string, contextPages []string) (*schema.QueryAnswer, error) {
			return &schema.QueryAnswer{}, nil
		},
		LintFunc: func(ctx context.Context, pages []*schema.Page) (*llm.LintReport, error) {
			return &llm.LintReport{}, nil
		},
	}

	memStorage := storage.NewMemoryStorage()

	cfg := &config.Config{
		Storage: memStorage,
		LLM:     mockLLM,
		Layout:  config.DefaultLayout(),
	}

	wiki, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create wiki: %v", err)
	}

	// Ingest some content
	source := &schema.Source{
		ID:      "test-source-list",
		Type:    schema.SourceTypeText,
		Content: "Entity1 and Entity2 are organizations. Concept1 is a concept.",
	}

	_, err = wiki.Ingest(context.Background(), source)
	if err != nil {
		t.Fatalf("Failed to ingest: %v", err)
	}

	// List all pages
	pages, err := wiki.ListPages(context.Background(), &ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list pages: %v", err)
	}

	if len(pages) < 3 {
		t.Errorf("Expected at least 3 pages, got %d", len(pages))
	}

	// List only entities
	entityType := schema.PageTypeEntity
	entityPages, err := wiki.ListPages(context.Background(), &ListOptions{Type: &entityType})
	if err != nil {
		t.Fatalf("Failed to list entity pages: %v", err)
	}

	if len(entityPages) < 2 {
		t.Errorf("Expected at least 2 entity pages, got %d", len(entityPages))
	}

	// Cleanup
	wiki.Close()
}
