# Tingly-Wiki Examples

This directory contains examples demonstrating how to use the tingly-wiki module.

## Acknowledgments

This project is inspired by the ideas discussed in [let me wikipedia this for you](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) by Andrej Karpathy. The concept of using LLMs as "programmers" that maintain and grow a persistent knowledge base is directly influenced by those insights.

## Quick Start

### Basic Usage with Memory Storage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/tingly-dev/tingly-wiki/config"
    "github.com/tingly-dev/tingly-wiki/llm"
    "github.com/tingly-dev/tingly-wiki/schema"
    "github.com/tingly-dev/tingly-wiki/storage"
    "github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
    // Create a wiki with memory storage (for testing)
    cfg := &config.Config{
        Storage: storage.NewMemoryStorage(),
        LLM:     llm.NewMockLLM(), // Use mock LLM for testing
        Layout:  config.DefaultLayout(),
    }

    w, err := wiki.New(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer w.Close()

    // Ingest a source document
    source := &schema.Source{
        Type:    schema.SourceTypeText,
        Content: `
OpenAI is an AI research organization consisting of the for-profit corporation OpenAI LP
and its parent company, the non-profit OpenAI Inc. The company was founded in December 2015
by Sam Altman, Elon Musk, and others.

GPT-4 is a large multimodal model that can accept image and text inputs and emit text outputs.
ChatGPT is a chatbot built on top of GPT models.
        `,
    }

    result, err := w.Ingest(context.Background(), source)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Ingested %d pages, updated %d pages\n", result.PagesCreated, result.PagesUpdated)
    fmt.Printf("Created: %v\n", result.CreatedPaths)

    // Query the wiki
    queryResult, err := w.Query(context.Background(), "What is GPT-4?", &wiki.QueryOptions{})
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Answer: %s\n", queryResult.Answer)
    fmt.Printf("Sources: %v\n", queryResult.PagesRead)

    // List all entities
    entityType := schema.PageTypeEntity
    pages, _ := w.ListPages(context.Background(), &wiki.ListOptions{Type: &entityType})
    for _, page := range pages {
        fmt.Printf("Entity: %s\n", page.Title)
    }
}
```

### Using with OpenAI

```go
package main

import (
    "context"
    "log"

    "github.com/tingly-dev/tingly-wiki/config"
    "github.com/tingly-dev/tingly-wiki/llm"
    "github.com/tingly-dev/tingly-wiki/schema"
    "github.com/tingly-dev/tingly-wiki/storage"
    "github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
    // Create OpenAI adapter
    openaiLLM, err := llm.NewOpenAIAdapter(&llm.OpenAIConfig{
        Model: "gpt-4o-mini", // or "gpt-4o", "gpt-4-turbo"
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create wiki with Markdown storage
    markdownStorage, err := storage.NewMarkdownStorage("./my-wiki", config.DefaultLayout())
    if err != nil {
        log.Fatal(err)
    }

    cfg := &config.Config{
        Storage: markdownStorage,
        LLM:     openaiLLM,
        Layout:  config.DefaultLayout(),
    }

    w, err := wiki.New(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer w.Close()

    // Use the wiki...
}
```

### Using with Anthropic Claude

```go
package main

import (
    "context"
    "log"

    "github.com/tingly-dev/tingly-wiki/config"
    "github.com/tingly-dev/tingly-wiki/llm"
    "github.com/tingly-dev/tingly-wiki/schema"
    "github.com/tingly-dev/tingly-wiki/storage"
    "github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
    // Create Anthropic adapter
    anthropicLLM, err := llm.NewAnthropicAdapter(&llm.AnthropicConfig{
        Model: "claude-3-5-sonnet-20241022", // or "claude-3-opus-20240229"
    })
    if err != nil {
        log.Fatal(err)
    }

    cfg := &config.Config{
        Storage: storage.NewMemoryStorage(),
        LLM:     anthropicLLM,
        Layout:  config.DefaultLayout(),
    }

    w, err := wiki.New(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer w.Close()

    // Use the wiki...
}
```

### Custom Layout

```go
package main

import (
    "github.com/tingly-dev/tingly-wiki/config"
)

func customLayoutExample() {
    // Create custom directory structure
    layout := &config.LayoutConfig{
        WikiRoot:    "./wiki",
        SourcesDir:  "docs/sources/",
        EntitiesDir: "kb/entities/",
        ConceptsDir: "kb/concepts/",
        SynthesisDir: "analysis/",
        IndexPath:   "catalog.md",
        LogPath:     "changelog.md",
        RawDir:      "assets/raw/",
    }

    cfg := &config.Config{
        Storage: nil, // Your storage
        LLM:     nil, // Your LLM
        Layout:  layout,
    }

    // Use config...
}
```

### Running a Health Check

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/tingly-dev/tingly-wiki/config"
    "github.com/tingly-dev/tingly-wiki/llm"
    "github.com/tingly-dev/tingly-wiki/storage"
    "github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
    // Setup wiki...
    cfg := &config.Config{
        Storage: storage.NewMemoryStorage(),
        LLM:     llm.NewMockLLM(),
        Layout:  config.DefaultLayout(),
    }

    w, _ := wiki.New(cfg)
    defer w.Close()

    // Ingest some content first
    source := &schema.Source{
        Type: schema.SourceTypeText,
        Content: "Example content for health check...",
    }
    w.Ingest(context.Background(), source)

    // Run health check
    lintOpts := &wiki.LintOptions{
        CheckContradictions: true,
        CheckOrphans:        true,
        CheckStale:          false,
        CheckMissingRefs:    true,
    }

    report, err := w.Lint(context.Background(), lintOpts)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d issues\n", len(report.Issues))
    for _, issue := range report.Issues {
        fmt.Printf("- [%s] %s: %s\n", issue.Severity, issue.Type, issue.Message)
    }

    fmt.Printf("\nSuggestions:\n")
    for _, suggestion := range report.Suggestions {
        fmt.Printf("- %s\n", suggestion)
    }
}
```

### Archiving Query Results

```go
package main

import (
    "context"
    "log"

    "github.com/tingly-dev/tingly-wiki/config"
    "github.com/tingly-dev/tingly-wiki/llm"
    "github.com/tingly-dev/tingly-wiki/storage"
    "github.com/tingly-dev/tingly-wiki/wiki"
)

func main() {
    cfg := &config.Config{
        Storage: storage.NewMemoryStorage(),
        LLM:     llm.NewMockLLM(),
        Layout:  config.DefaultLayout(),
    }

    w, _ := wiki.New(cfg)
    defer w.Close()

    // Query and archive the result as a new page
    queryResult, err := w.Query(context.Background(), "Summary of all entities", &wiki.QueryOptions{
        ArchiveResult: true,
        ArchivePath:   "synthesis/entity-summary.md",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Answer: %s", queryResult.Answer)
    log.Printf("Archived to: %s", queryResult.ArchivedPath)
}
```

## Running the Examples

Each example can be run as a standalone Go program:

```bash
cd examples/basic
go run main.go
```

## Environment Variables

For examples using real LLM providers:

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."

# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."
```

## Next Steps

- See [llm/prompts.go](../llm/prompts.go) for prompt templates
- See [schema/types.go](../schema/types.go) for data structures
- See [wiki/wiki.go](../wiki/wiki.go) for the Wiki interface
