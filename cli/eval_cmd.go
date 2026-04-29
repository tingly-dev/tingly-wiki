package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-wiki/eval"
	"github.com/tingly-dev/tingly-wiki/llm"
)

// EvalCmd runs the memory-system evaluation scenarios.
//
// Examples:
//
//	wiki eval --scenarios eval/scenarios
//	wiki eval --scenarios eval/scenarios --format json --report report.json
//	wiki eval --scenarios eval/scenarios/preference-recall.yaml --real-llm
type EvalCmd struct {
	// Scenarios is the path to a directory of *.yaml files or a single .yaml.
	Scenarios string `help:"Path to scenarios directory or single .yaml file" required:""`

	// Report is the output path. Empty string writes to stdout.
	Report string `help:"Output file path (default: stdout)"`

	// Format is the output format: "markdown" (default) or "json".
	Format string `help:"Output format" default:"markdown" enum:"markdown,json"`

	// RealLLM uses the configured OpenAI adapter for embeddings instead of
	// the deterministic mock. Slower and more expensive, but produces
	// quality numbers comparable to production.
	RealLLM bool `help:"Use real OpenAI LLM for embeddings (requires --openai-key)"`
}

// Run executes the eval command.
func (c *EvalCmd) Run(cli *CLI) error {
	if c.Scenarios == "" {
		return fmt.Errorf("--scenarios is required")
	}

	scs, err := loadScenariosFromPath(c.Scenarios)
	if err != nil {
		return fmt.Errorf("load scenarios: %w", err)
	}
	if len(scs) == 0 {
		return fmt.Errorf("no scenarios found at %s", c.Scenarios)
	}

	runner := eval.NewRunner()
	if c.RealLLM {
		if cli.OpenAIKey == "" {
			return fmt.Errorf("--openai-key is required when --real-llm is set")
		}
		adapter, err := llm.NewOpenAIAdapter(&llm.OpenAIConfig{
			APIKey:  cli.OpenAIKey,
			Model:   cli.OpenAIModel,
			BaseURL: cli.OpenAIBaseURL,
		})
		if err != nil {
			return fmt.Errorf("create OpenAI adapter: %w", err)
		}
		runner.LLM = adapter
	}

	results, err := runner.RunAll(context.Background(), scs)
	if err != nil {
		return fmt.Errorf("run scenarios: %w", err)
	}

	var output []byte
	switch c.Format {
	case "json":
		output, err = eval.ToJSON(results)
		if err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	default: // markdown
		output = []byte(eval.ToMarkdown(results))
	}

	if c.Report == "" {
		cli.printf("%s", string(output))
	} else {
		if err := os.WriteFile(c.Report, output, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", c.Report, err)
		}
		cli.printf("✓ Report written to %s\n", c.Report)
	}

	// Print a one-line summary to stderr so callers see it even when --report
	// captures the full output.
	cli.printfError("Ran %d scenarios:\n", len(results))
	for _, m := range results {
		cli.printfError("  %s — P@K=%.2f R@K=%.2f MRR=%.2f\n",
			m.Scenario, m.AvgPrecisionK, m.AvgRecallK, m.MRR)
	}

	return nil
}

// loadScenariosFromPath accepts either a directory or a single .yaml file.
func loadScenariosFromPath(path string) ([]*eval.Scenario, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return eval.LoadScenarios(path)
	}
	sc, err := eval.LoadScenario(path)
	if err != nil {
		return nil, err
	}
	return []*eval.Scenario{sc}, nil
}
