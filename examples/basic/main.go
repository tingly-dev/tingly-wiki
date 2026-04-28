package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
	"github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	useMock := apiKey == ""

	// Get custom configuration from environment
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini" // default model
	}

	if useMock {
		fmt.Println("No OPENAI_API_KEY found, using Mock LLM...")
		fmt.Println("Set OPENAI_API_KEY to use real OpenAI API")
		fmt.Println()
		fmt.Println("Optional environment variables:")
		fmt.Println("  OPENAI_BASE_URL - Custom base URL for OpenAI-compatible API")
		fmt.Println("  OPENAI_MODEL    - Model to use (default: gpt-4o-mini)")
		fmt.Println()
	}

	// Create wiki configuration
	var llmInstance llm.LLM
	if useMock {
		llmInstance = llm.NewMockLLM()
	} else {
		var err error
		llmInstance, err = llm.NewOpenAIAdapter(&llm.OpenAIConfig{
			APIKey:  apiKey,
			Model:   model,
			BaseURL: baseURL,
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	cfg := &config.Config{
		Storage: storage.NewMemoryStorage(),
		LLM:     llmInstance,
		Layout:  config.DefaultLayout(),
	}

	w, err := wiki.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	fmt.Println("=== Tingly-Wiki Basic Example ===")
	fmt.Println()

	// Ingest a source document
	fmt.Println("1. Ingesting source document...")
	source := &schema.Source{
		Type: schema.SourceTypeText,
		Content: `
OpenAI is an AI research organization consisting of the for-profit corporation OpenAI LP
and its parent company, the non-profit OpenAI Inc. The company was founded in December 2015
by Sam Altman, Elon Musk, and others.

GPT-4 is a large multimodal model that can accept image and text inputs and emit text outputs.
It was released in March 2023 and demonstrates advanced reasoning capabilities.

ChatGPT is a chatbot built on top of GPT models. It was launched in November 2022 and quickly
gained popularity for its conversational abilities and helpfulness.

The company's mission is to ensure that artificial general intelligence benefits all of humanity.
		`,
	}

	result, err := w.Ingest(context.Background(), source)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   ✓ Created %d pages, updated %d pages\n", result.PagesCreated, result.PagesUpdated)
	fmt.Printf("   ✓ Created: %v\n", result.CreatedPaths)
	fmt.Println()

	// List all entities
	fmt.Println("2. Listing entities...")
	entityType := schema.PageTypeEntity
	pages, _ := w.ListPages(context.Background(), &wiki.ListOptions{Type: &entityType})
	for _, page := range pages {
		fmt.Printf("   - %s: %s\n", page.Title, page.Path)
	}
	fmt.Println()

	// List all concepts
	fmt.Println("3. Listing concepts...")
	conceptType := schema.PageTypeConcept
	concepts, _ := w.ListPages(context.Background(), &wiki.ListOptions{Type: &conceptType})
	for _, page := range concepts {
		fmt.Printf("   - %s: %s\n", page.Title, page.Path)
	}
	fmt.Println()

	// Query the wiki
	fmt.Println("4. Querying wiki...")
	queryResult, err := w.Query(context.Background(), "What is GPT-4?", &wiki.QueryOptions{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   Question: What is GPT-4?\n")
	fmt.Printf("   Answer: %s\n", queryResult.Answer)
	fmt.Printf("   Sources: %v\n", queryResult.PagesRead)
	fmt.Println()

	// Health check
	fmt.Println("5. Running health check...")
	lintOpts := &wiki.LintOptions{
		CheckContradictions: true,
		CheckOrphans:        true,
		CheckMissingRefs:    true,
	}

	report, err := w.Lint(context.Background(), lintOpts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   ✓ Found %d issues\n", len(report.Issues))
	if len(report.Issues) > 0 {
		for _, issue := range report.Issues {
			fmt.Printf("     - [%s] %s\n", issue.Severity, issue.Message)
		}
	}

	fmt.Println()
	fmt.Println("=== Example Complete ===")
}
