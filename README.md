# Tingly-Wiki

A Go module for building LLM-powered personal knowledge bases with persistent, compounding wikis.

## Features

- **Persistent Knowledge Base**: Extract and maintain structured information from source documents
- **Smart Extraction**: Automatically identify entities, concepts, and relationships
- **Query & Synthesis**: Ask questions and get AI-powered answers with citations
- **Health Checking**: Detect contradictions, orphan pages, and stale information
- **Multiple LLM Providers**: Support for OpenAI and Anthropic (Claude)
- **Flexible Storage**: Markdown (Obsidian-compatible) or in-memory storage
- **Cross-References**: Automatic linking between related pages

## Installation

```bash
go get github.com/tingly-dev/tingly-wiki
```

## Quick Start

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
    // Create wiki with OpenAI
    openaiLLM, _ := llm.NewOpenAIAdapter(&llm.OpenAIConfig{
        Model: "gpt-4o-mini",
    })

    cfg := &config.Config{
        Storage: storage.NewMemoryStorage(),
        LLM:     openaiLLM,
        Layout:  config.DefaultLayout(),
    }

    w, _ := wiki.New(cfg)
    defer w.Close()

    // Ingest a document
    source := &schema.Source{
        Type:    schema.SourceTypeText,
        Content: "OpenAI develops GPT-4, a large language model...",
    }

    result, _ := w.Ingest(context.Background(), source)
    log.Printf("Created %d pages", result.PagesCreated)

    // Query the wiki
    answer, _ := w.Query(context.Background(), "What is GPT-4?", nil)
    log.Printf("Answer: %s", answer.Answer)
}
```

## Examples

See the [examples](./examples/) directory for more usage examples:

- [Basic Example](./examples/basic/) - Simple ingest and query workflow
- [Examples README](./examples/README.md) - Detailed usage guide

## Configuration

### Storage Options

```go
// In-memory (for testing)
storage := storage.NewMemoryStorage()

// Markdown files (Obsidian-compatible)
storage, _ := storage.NewMarkdownStorage("./my-wiki", config.DefaultLayout())
```

### LLM Options

```go
// OpenAI
openaiLLM, _ := llm.NewOpenAIAdapter(&llm.OpenAIConfig{
    APIKey: "sk-...",
    Model:  "gpt-4o-mini",
})

// Anthropic Claude
anthropicLLM, _ := llm.NewAnthropicAdapter(&llm.AnthropicConfig{
    APIKey: "sk-ant-...",
    Model:  "claude-3-5-sonnet-20241022",
})

// Mock (for testing)
mockLLM := llm.NewMockLLM()
```

### Custom Layout

```go
layout := &config.LayoutConfig{
    SourcesDir:  "docs/",
    EntitiesDir: "entities/",
    ConceptsDir: "concepts/",
    IndexPath:   "index.md",
    LogPath:     "log.md",
}
```

## How It Works

1. **Ingest**: Process source documents → Extract entities/concepts → Create/update pages
2. **Query**: Search relevant pages → LLM synthesizes answer → Return with citations
3. **Lint**: Analyze all pages → Detect issues → Suggest improvements

## Acknowledgments

This project is inspired by the ideas discussed in [let me wikipedia this for you](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) by Andrej Karpathy. The concept of using LLMs as "programmers" that maintain and grow a persistent knowledge base is directly influenced by those insights.

## License

See [LICENSE](../../LICENSE.txt) in the main project.
