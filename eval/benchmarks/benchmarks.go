// Package benchmarks provides adapters for public external benchmark datasets
// (LoCoMo, LongMemEval) that convert them into eval.Scenario instances and
// produce comparison reports against published mem0/Zep baseline numbers.
//
// Usage:
//
//	runner := benchmarks.NewBenchmarkRunner(nil) // nil → DeterministicMockLLM
//	results, err := runner.RunLoCoMo(ctx, "locomo.json", 0)
//	fmt.Print(benchmarks.ToMarkdown(results))
package benchmarks

import "github.com/tingly-dev/tingly-wiki/eval"

// BenchmarkResult wraps a ScenarioMetric with published baseline numbers
// for direct comparison.
type BenchmarkResult struct {
	// Dataset is "locomo" or "longmemeval".
	Dataset string
	// Category groups results within a dataset (e.g. LongMemEval category name).
	Category string
	// Metric is the measured retrieval quality.
	Metric *eval.ScenarioMetric
	// Baselines maps system name → published score (MRR or accuracy, 0-1).
	Baselines map[string]float64
}

// LoCoMoBaselines holds published accuracy numbers for the LoCoMo benchmark.
// Source: LoCoMo paper (Maharana et al., 2024), Table 2 — single-session QA.
var LoCoMoBaselines = map[string]float64{
	"mem0":          0.916,
	"openai_memory": 0.656,
	"zep":           0.892,
}

// LongMemEvalBaselines holds published average accuracy for LongMemEval.
// Source: LongMemEval paper (Wu et al., 2024), overall accuracy across all
// five evaluation categories.
var LongMemEvalBaselines = map[string]float64{
	"zep_graphiti": 0.485,
	"mem0":         0.412,
	"full_context": 0.620,
}
