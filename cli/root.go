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
	Config      string `help:"Path to configuration file" env:"WIKI_CONFIG"`
	WikiDir     string `help:"Wiki storage directory" env:"WIKI_DIR"`
	OpenAIKey   string `help:"OpenAI API key" env:"OPENAI_API_KEY" short:"k"`
	OpenAIBaseURL string `help:"Custom OpenAI-compatible endpoint" env:"OPENAI_BASE_URL"`
	OpenAIModel  string `help:"Chat model name" env:"OPENAI_MODEL"`
	OpenAIEmbedModel string `help:"Embedding model name" env:"OPENAI_EMBED_MODEL"`

	// Commands
	Cfg       ConfigCmd    `cmd:"" help:"Manage configuration"`
	Ingest    IngestCmd    `cmd:"" help:"Ingest content from stdin or file"`
	Ask       AskCmd       `cmd:"" help:"Ask the wiki a question"`
	Eval      EvalCmd      `cmd:"" help:"Run memory quality evaluation scenarios"`
	Benchmark BenchmarkCmd `cmd:"" help:"Run external benchmark datasets (LoCoMo, LongMemEval)"`

	// IO for testing
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader

	// Loaded configuration
	cliConfig *config.CLIConfig
}

// ConfigCmd handles configuration management
type ConfigCmd struct {
	Init  bool `help:"Create a new config file" short:"i"`
	Path  string `arg:"" optional:"" help:"Path to config file (default: .wiki/config.yml)"`
	Show  bool `help:"Show current configuration" short:"s"`
}

// Run executes the config command
func (c *ConfigCmd) Run(cli *CLI) error {
	// Determine config path
	configPath := c.Path
	if configPath == "" {
		if cli.WikiDir != "" {
			configPath = cli.WikiDir + "/config.yml"
		} else {
			configPath = ".wiki/config.yml"
		}
	}

	if c.Show {
		// Show current configuration
		if err := cli.loadConfigIfNeeded(); err != nil {
			return err
		}
		cli.printf("Wiki Directory: %s\n", cli.getWikiDir())

		if cli.cliConfig != nil {
			cli.printf("LLM Provider: %s\n", cli.cliConfig.LLM.Provider)
			if cli.cliConfig.LLM.Provider == "openai" {
				cli.printf("  Base URL: %s\n", cli.cliConfig.LLM.OpenAI.BaseURL)
				cli.printf("  Model: %s\n", cli.cliConfig.LLM.OpenAI.Model)
				cli.printf("  Embedding: %s\n", cli.cliConfig.LLM.OpenAI.EmbeddingModel)
			}
			cli.printf("Benchmark:\n")
			cli.printf("  Data Dir: %s\n", cli.cliConfig.Benchmark.DataDir)
			cli.printf("  Real LLM: %v\n", cli.cliConfig.Benchmark.RealLLM)
		}
		return nil
	}

	if c.Init {
		// Create new config file
		defaultCfg := config.DefaultCLIConfig()

		// Check if file already exists
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("config file already exists at %s", configPath)
		}

		// Create directory if needed
		if err := os.MkdirAll(configPath[:len(configPath)-len("/config.yml")], 0o755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		// Save config
		if err := defaultCfg.Save(configPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		cli.printf("✓ Created config file at %s\n", configPath)
		cli.printf("  Edit this file to customize your settings\n")
		return nil
	}

	// Default: show current config
	return (&ConfigCmd{Show: true}).Run(cli)
}

// IngestCmd handles content ingestion
type IngestCmd struct {
	Path  string `arg:"" optional:"" help:"File path to ingest (reads from stdin if empty or '-')"`
	Title string `help:"Optional title for the source"`
}

// Run executes the ingest command
func (c *IngestCmd) Run(cli *CLI) error {
	// Load configuration
	if err := cli.loadConfigIfNeeded(); err != nil {
		return err
	}

	// Create wiki
	w, err := cli.createWiki()
	if err != nil {
		return err
	}
	defer w.Close()

	// Read input content
	content, err := cli.readInput(c.Path)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if content == "" {
		return fmt.Errorf("no content to ingest")
	}

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
	// Load configuration
	if err := cli.loadConfigIfNeeded(); err != nil {
		return err
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

// loadConfigIfNeeded loads the configuration file if not already loaded
func (c *CLI) loadConfigIfNeeded() error {
	if c.cliConfig != nil {
		return nil
	}

	// Determine wiki directory for config search
	wikiDir := c.WikiDir
	if wikiDir == "" {
		wikiDir = ".wiki"
	}

	// Load config
	cfg, err := config.LoadCLIConfig(wikiDir, c.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	c.cliConfig = cfg

	// Apply CLI overrides (CLI flags take precedence over config file)
	if c.OpenAIKey != "" {
		c.cliConfig.LLM.OpenAI.APIKey = c.OpenAIKey
	}
	if c.OpenAIBaseURL != "" {
		c.cliConfig.LLM.OpenAI.BaseURL = c.OpenAIBaseURL
	}
	if c.OpenAIModel != "" {
		c.cliConfig.LLM.OpenAI.Model = c.OpenAIModel
	}
	if c.OpenAIEmbedModel != "" {
		c.cliConfig.LLM.OpenAI.EmbeddingModel = c.OpenAIEmbedModel
	}
	if c.WikiDir != "" {
		c.cliConfig.WikiDir = c.WikiDir
	}

	return nil
}

// getWikiDir returns the wiki directory from config or CLI flag
func (c *CLI) getWikiDir() string {
	if c.WikiDir != "" {
		return c.WikiDir
	}
	if c.cliConfig != nil && c.cliConfig.WikiDir != "" {
		return c.cliConfig.WikiDir
	}
	return ".wiki"
}

// getOpenAIConfig returns the OpenAI configuration
func (c *CLI) getOpenAIConfig() *llm.OpenAIConfig {
	if c.cliConfig == nil {
		// Use CLI flags only
		return &llm.OpenAIConfig{
			APIKey:  c.OpenAIKey,
			BaseURL: c.OpenAIBaseURL,
			Model:   c.OpenAIModel,
		}
	}

	// Use config with CLI overrides
	oaCfg := c.cliConfig.LLM.OpenAI
	return &llm.OpenAIConfig{
		APIKey:         oaCfg.APIKey,
		BaseURL:        oaCfg.BaseURL,
		Model:          oaCfg.Model,
		EmbeddingModel: oaCfg.EmbeddingModel,
	}
}

// createWiki initializes a wiki instance from CLI/config
func (c *CLI) createWiki() (*wiki.WikiImpl, error) {
	// Load config first
	if err := c.loadConfigIfNeeded(); err != nil {
		return nil, err
	}

	// Get OpenAI config
	llmCfg := c.getOpenAIConfig()

	// Validate API key
	if llmCfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required (set via --openai-key, OPENAI_API_KEY env, or config file)")
	}

	// Create LLM adapter
	llmAdapter, err := llm.NewOpenAIAdapter(llmCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM adapter: %w", err)
	}

	// Create storage
	store, err := storage.NewMarkdownStorage(c.getWikiDir(), config.DefaultLayout())
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
