# CLI Specification

**Category**: cli
**Feature**: ingest-and-ask
**Date**: 2026-04-28
**Status**: Draft

## Overview

Add a CLI interface for Tingly-Wiki to enable command-line based content ingestion and querying without writing Go code. The CLI will support two core commands: `ingest` (add content) and `ask` (query the wiki).

## Scope

- `wiki ingest` - Ingest content from stdin or file path
- `wiki ask <question>` - Query the wiki and get an answer
- Configurable wiki directory (default: `.wiki`)
- Configurable LLM settings (API key, base URL, model) via flags only

## Requirements

### Functional Requirements

1. **Ingest Command**
   - Accept content from stdin (`-` or no argument)
   - Accept content from file path
   - Support optional title/ID for the source
   - Display ingestion results (pages created/updated)

2. **Ask Command**
   - Accept question as argument
   - Display answer with sources
   - Support optional context limit

3. **Configuration**
   - `--wiki-dir` - Wiki storage directory (default: `.wiki`)
   - `--openai-key` - OpenAI API key
   - `--openai-base-url` - Custom OpenAI-compatible endpoint
   - `--openai-model` - Model name (default: `gpt-4o-mini`)

### Non-Functional Requirements

- Follow standard CLI conventions (flags, exit codes, stdout/stderr)
- Graceful error handling with clear messages
- Support for piping data

## User Stories

```bash
# Story 1: Quick ingestion from stdin
echo "OpenAI is an AI research company." | wiki ingest

# Story 2: Ingest from file
wiki ingest document.txt

# Story 3: Ingest with custom wiki location
wiki --wiki-dir ~/my-wiki ingest report.txt

# Story 4: Ask a question
wiki ask "What is OpenAI?"

# Story 5: Custom LLM endpoint
wiki --openai-base-url http://localhost:8080/v1 --openai-model local-llm ask "..."
```

## Data Structures

### CLI Configuration

```go
type CLIConfig struct {
    WikiDir      string
    OpenAIKey    string
    OpenAIBaseURL string
    OpenAIModel  string
}

// Default values
const (
    DefaultWikiDir     = ".wiki"
    DefaultOpenAIModel = "gpt-4o-mini"
)
```

## Command Interface

### Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--wiki-dir` | Wiki storage directory | `.wiki` |
| `--openai-key` | OpenAI API key | (from flag or error) |
| `--openai-base-url` | Custom base URL | (empty) |
| `--openai-model` | Model name | `gpt-4o-mini` |

### ingest Command

```
wiki ingest [flags] [path]

Arguments:
  path    File path to ingest (optional, reads from stdin if omitted or "-")
```

Example output:
```
✓ Ingested: 5 pages created, 2 updated
  entities/openai.md
  entities/sam-altman.md
  concepts/artificial-general-intelligence.md
  ...
```

### ask Command

```
wiki ask [flags] <question>

Arguments:
  question    Question to ask the wiki
```

Example output:
```
Q: What is GPT-4?
A: GPT-4 is a large multimodal model released in March 2023...

Sources:
  - sources/doc-123.md
  - entities/gpt-4.md
```

## Component Structure

```
cli/
├── root.go       # Root command and global flags
├── ingest.go     # Ingest command implementation
├── ask.go        # Ask command implementation
├── config.go     # CLI config handling
└── main.go       # Entry point
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid usage (wrong flags/arguments) |

## Implementation Plan

### Phase 1: Core Structure
1. Set up CLI framework (cobra or standard flag package)
2. Implement root command with global flags
3. Implement config loading from flags

### Phase 2: Ingest Command
1. Implement ingest command with stdin/file input
2. Add wiki initialization
3. Add result formatting

### Phase 3: Ask Command
1. Implement ask command
2. Add answer formatting
3. Add error handling

### Phase 4: Testing & Polish
1. Add CLI tests
2. Add help text and examples
3. Validate error handling

## Open Questions

1. Should we support a `--mock` flag for testing without API key?
2. Should ingest support multiple files at once (glob pattern)?

## Dependencies

- Standard library `flag` package OR `cobra` for CLI framework
- Existing `wiki`, `storage`, `llm` packages

## Alternatives Considered

1. **Environment variables vs Flags** - Chose flags only per user request
2. **Config file** - Not included in initial scope, could be added later
3. **Multiple LLM providers** - Start with OpenAI only, extend later
