package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
	"github.com/tingly-dev/tingly-wiki/wiki"
)

// CLI represents the CLI application
type CLI struct {
	// Global flags
	WikiDir      string `help:"Wiki storage directory" default:".wiki"`
	OpenAIKey    string `help:"OpenAI API key" env:"OPENAI_API_KEY" short:"k"`
	OpenAIBaseURL string `help:"Custom OpenAI-compatible endpoint"`
	OpenAIModel  string `help:"Model name" default:"gpt-4o-mini"`

	// Commands
	Ingest IngestCmd `cmd:"" help:"Ingest content from stdin or file"`
	Ask    AskCmd    `cmd:"" help:"Ask the wiki a question"`

	// IO for testing
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

// IngestCmd handles content ingestion
type IngestCmd struct {
	Path  string `arg:"" optional:"" help:"File path to ingest (reads from stdin if empty or '-')"`
	Title string `help:"Optional title for the source"`
}

// Run executes the ingest command
func (c *IngestCmd) Run(cli *CLI) error {
	// Validate required config
	if cli.OpenAIKey == "" {
		return fmt.Errorf("--openai-key is required")
	}

	// Read input content
	content, err := cli.readInput(c.Path)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if content == "" {
		return fmt.Errorf("no content to ingest")
	}

	// Create wiki
	w, err := cli.createWiki()
	if err != nil {
		return err
	}
	defer w.Close()

	// Create source
	source := &schema.Source{
		Type:    schema.SourceTypeText,
		Content: content,
	}

	// Ingest
	result, err := w.Ingest(context.Background(), source)
	if err != nil {
		return fmt.Errorf("failed to ingest: %w", err)
	}

	// Print results
	cli.printf("✓ Ingested: %d pages created, %d updated\n", result.PagesCreated, result.PagesUpdated)

	if len(result.CreatedPaths) > 0 {
		cli.printf("\nCreated:\n")
		for _, p := range result.CreatedPaths {
			cli.printf("  %s\n", p)
		}
	}

	if len(result.UpdatedPaths) > 0 {
		cli.printf("\nUpdated:\n")
		for _, p := range result.UpdatedPaths {
			cli.printf("  %s\n", p)
		}
	}

	return nil
}

// AskCmd handles queries to the wiki
type AskCmd struct {
	Question string `arg:"" help:"Question to ask the wiki"`
	Limit    int    `help:"Maximum number of context pages to use" default:"0"`
}

// Run executes the ask command
func (c *AskCmd) Run(cli *CLI) error {
	// Validate required config
	if cli.OpenAIKey == "" {
		return fmt.Errorf("--openai-key is required")
	}

	// Validate question
	if c.Question == "" {
		return fmt.Errorf("question is required")
	}

	// Create wiki
	w, err := cli.createWiki()
	if err != nil {
		return err
	}
	defer w.Close()

	// Query
	opts := &wiki.QueryOptions{
		Limit: c.Limit,
	}

	result, err := w.Query(context.Background(), c.Question, opts)
	if err != nil {
		return fmt.Errorf("failed to query: %w", err)
	}

	// Print answer
	cli.printf("Q: %s\n\n", c.Question)
	cli.printf("A: %s\n", result.Answer)

	if len(result.PagesRead) > 0 {
		cli.printf("\nSources:\n")
		for _, p := range result.PagesRead {
			cli.printf("  - %s\n", p)
		}
	}

	return nil
}

// readInput reads content from stdin or file
func (c *CLI) readInput(path string) (string, error) {
	if c.stdin == nil {
		c.stdin = os.Stdin
	}

	if path == "" || path == "-" {
		// Read from stdin
		content, err := io.ReadAll(c.stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		return string(content), nil
	}

	// Read from file
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return string(content), nil
}

// createWiki initializes a wiki instance from CLI config
func (c *CLI) createWiki() (*wiki.WikiImpl, error) {
	// Create LLM adapter
	llmAdapter, err := llm.NewOpenAIAdapter(&llm.OpenAIConfig{
		APIKey:  c.OpenAIKey,
		Model:   c.OpenAIModel,
		BaseURL: c.OpenAIBaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM adapter: %w", err)
	}

	// Create storage
	store, err := storage.NewMarkdownStorage(c.WikiDir, config.DefaultLayout())
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create wiki
	cfg := &config.Config{
		Storage: store,
		LLM:     llmAdapter,
		Layout:  config.DefaultLayout(),
	}

	w, err := wiki.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create wiki: %w", err)
	}

	return w, nil
}

// printf writes to stdout
func (c *CLI) printf(format string, args ...interface{}) {
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	fmt.Fprintf(c.stdout, format, args...)
}

// printfError writes to stderr
func (c *CLI) printfError(format string, args ...interface{}) {
	if c.stderr == nil {
		c.stderr = os.Stderr
	}
	fmt.Fprintf(c.stderr, format, args...)
}

// Main is the entry point for the CLI
func Main() int {
	cli := &CLI{}

	ctx, err := kong.New(cli,
		kong.Name("wiki"),
		kong.Description("Tingly-Wiki CLI - Build your knowledge base"),
		kong.UsageOnError(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	kongCtx, err := ctx.Parse(os.Args[1:])
	if err != nil {
		ctx.FatalIfErrorf(err)
		return 2
	}

	// Run the command
	if err := kongCtx.Run(cli); err != nil {
		cli.printfError("Error: %v\n", err)
		return 1
	}

	return 0
}

// Exec is an alternative entry point for testing
func Exec(args []string) int {
	cli := &CLI{}

	ctx, err := kong.New(cli)
	if err != nil {
		return 1
	}

	kongCtx, err := ctx.Parse(args)
	if err != nil {
		return 2
	}

	if err := kongCtx.Run(cli); err != nil {
		return 1
	}

	return 0
}
