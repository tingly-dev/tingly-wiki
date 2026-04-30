package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-wiki/eval/benchmarks"
	"github.com/tingly-dev/tingly-wiki/llm"
)

// BenchmarkCmd runs external benchmark datasets (LoCoMo, LongMemEval) against
// the memory system and produces comparison reports vs published baselines.
//
// Examples:
//
//	wiki benchmark --dataset locomo --data-path locomo.json
//	wiki benchmark --dataset longmemeval --data-path lme.json --format json
//	wiki benchmark --dataset locomo --data-path locomo.json --real-llm --limit 5
type BenchmarkCmd struct {
	// Dataset selects the benchmark to run.
	Dataset string `help:"Benchmark dataset to run" enum:"locomo,longmemeval" required:""`

	// DataPath is the local JSON file for the selected dataset.
	DataPath string `help:"Path to dataset JSON file" required:""`

	// Limit caps the number of conversations/items evaluated. 0 means all.
	Limit int `help:"Maximum conversations/items to evaluate (0 = all)" default:"0"`

	// Report is the output path; empty means stdout.
	Report string `help:"Output file path (default: stdout)"`

	// Format controls output encoding.
	Format string `help:"Output format" default:"markdown" enum:"markdown,json"`

	// RealLLM switches from the deterministic mock to the configured OpenAI
	// adapter for embeddings — produces quality numbers closer to production.
	RealLLM bool `help:"Use real OpenAI LLM for embeddings (requires --openai-key)"`
}

// Run executes the benchmark command.
func (c *BenchmarkCmd) Run(cli *CLI) error {
	var runner *benchmarks.BenchmarkRunner
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
		runner = benchmarks.NewBenchmarkRunner(adapter)
	} else {
		runner = benchmarks.NewBenchmarkRunner(nil)
	}

	ctx := context.Background()
	var results []*benchmarks.BenchmarkResult
	var err error

	switch c.Dataset {
	case "locomo":
		results, err = runner.RunLoCoMo(ctx, c.DataPath, c.Limit)
	case "longmemeval":
		results, err = runner.RunLongMemEval(ctx, c.DataPath, c.Limit)
	default:
		return fmt.Errorf("unknown dataset: %s", c.Dataset)
	}
	if err != nil {
		return fmt.Errorf("run benchmark: %w", err)
	}

	var output []byte
	switch c.Format {
	case "json":
		output, err = benchmarks.ToJSON(results)
		if err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	default:
		output = []byte(benchmarks.ToMarkdown(results))
	}

	if c.Report == "" {
		cli.printf("%s", string(output))
	} else {
		if err := os.WriteFile(c.Report, output, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", c.Report, err)
		}
		cli.printf("✓ Report written to %s\n", c.Report)
	}

	// One-line summary per scenario to stderr so callers see it even when
	// --report redirects the full output.
	cli.printfError("Ran %d scenario(s) [%s]:\n", len(results), c.Dataset)
	for _, r := range results {
		cli.printfError("  %s — P@K=%.2f R@K=%.2f MRR=%.2f\n",
			r.Metric.Scenario, r.Metric.AvgPrecisionK, r.Metric.AvgRecallK, r.Metric.MRR)
	}

	return nil
}
