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
	Dataset string `help:"Benchmark dataset to run" enum:"locomo,longmemeval," default:""`

	// DataPath is the local JSON file for the selected dataset.
	DataPath string `help:"Path to dataset JSON file" default:""`

	// Limit caps the number of conversations/items evaluated. 0 means all.
	Limit int `help:"Maximum conversations/items to evaluate (0 = all)" default:"0"`

	// Report is the output path; empty means stdout.
	Report string `help:"Output file path (default: stdout)"`

	// Format controls output encoding.
	Format string `help:"Output format" default:"" enum:"markdown,json,"`

	// RealLLM switches from the deterministic mock to the configured OpenAI
	// adapter for embeddings — produces quality numbers closer to production.
	// If not specified, uses value from config file.
	RealLLM bool `help:"Use real OpenAI LLM for embeddings (requires --openai-key)"`
}

// Run executes the benchmark command.
func (c *BenchmarkCmd) Run(cli *CLI) error {
	// Load configuration
	if err := cli.loadConfigIfNeeded(); err != nil {
		return err
	}

	// Determine dataset and data path from config if not provided
	dataset := c.Dataset
	dataPath := c.DataPath
	if dataset == "" && cli.cliConfig != nil {
		dataset = cli.cliConfig.Benchmark.Dataset
	}
	if dataPath == "" && cli.cliConfig != nil {
		dataPath = cli.cliConfig.Benchmark.DataDir + "/" + dataset + "10.json"
	}

	if dataset == "" {
		return fmt.Errorf("--dataset is required (or set in config file)")
	}
	if dataPath == "" {
		return fmt.Errorf("--data-path is required (or set benchmark.data_dir in config file)")
	}

	// Determine whether to use real LLM
	realLLM := c.RealLLM
	if !realLLM && cli.cliConfig != nil {
		realLLM = cli.cliConfig.Benchmark.RealLLM
	}

	var runner *benchmarks.BenchmarkRunner
	if realLLM {
		// Get OpenAI config from config file or CLI flags
		llmCfg := cli.getOpenAIConfig()
		if llmCfg.APIKey == "" {
			return fmt.Errorf("--openai-key is required when --real-llm is set")
		}
		adapter, err := llm.NewOpenAIAdapter(llmCfg)
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

	switch dataset {
	case "locomo":
		results, err = runner.RunLoCoMo(ctx, dataPath, c.Limit)
	case "longmemeval":
		results, err = runner.RunLongMemEval(ctx, dataPath, c.Limit)
	default:
		return fmt.Errorf("unknown dataset: %s", dataset)
	}
	if err != nil {
		return fmt.Errorf("run benchmark: %w", err)
	}

	// Determine format from config if not specified
	format := c.Format
	if format == "" && cli.cliConfig != nil {
		format = cli.cliConfig.Benchmark.Format
	}
	if format == "" {
		format = "markdown"
	}

	var output []byte
	switch format {
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
	cli.printfError("Ran %d scenario(s) [%s]:\n", len(results), dataset)
	for _, r := range results {
		cli.printfError("  %s — P@K=%.2f R@K=%.2f MRR=%.2f\n",
			r.Metric.Scenario, r.Metric.AvgPrecisionK, r.Metric.AvgRecallK, r.Metric.MRR)
	}

	return nil
}
