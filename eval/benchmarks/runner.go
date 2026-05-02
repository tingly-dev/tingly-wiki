package benchmarks

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-wiki/eval"
	"github.com/tingly-dev/tingly-wiki/eval/benchmarks/locomo"
	"github.com/tingly-dev/tingly-wiki/eval/benchmarks/longmemeval"
	"github.com/tingly-dev/tingly-wiki/llm"
)

// BenchmarkRunner executes external benchmark datasets against the memory
// system and wraps results with published baseline numbers for comparison.
type BenchmarkRunner struct {
	// EvalRunner is the underlying scenario runner. Created by NewBenchmarkRunner.
	EvalRunner *eval.Runner
	// OutputDir is the directory where wiki data will be persisted.
	// If empty, uses in-memory storage (faster, no disk I/O).
	OutputDir string
}

// NewBenchmarkRunner returns a BenchmarkRunner. Pass a non-nil llm.LLM to
// use a real embedder; pass nil to use the DeterministicMockLLM (CI-safe).
func NewBenchmarkRunner(llmAdapter llm.LLM) *BenchmarkRunner {
	r := eval.NewRunner()
	if llmAdapter != nil {
		r.LLM = llmAdapter
	}
	return &BenchmarkRunner{EvalRunner: r}
}

// RunLoCoMo loads conversations from dataPath, converts them to scenarios,
// runs them, and attaches LoCoMoBaselines to each result.
// Pass limit ≤ 0 to run all conversations.
func (br *BenchmarkRunner) RunLoCoMo(ctx context.Context, dataPath string, limit int) ([]*BenchmarkResult, error) {
	convs, err := locomo.LoadConversations(dataPath)
	if err != nil {
		return nil, fmt.Errorf("locomo load: %w", err)
	}
	if len(convs) == 0 {
		return nil, fmt.Errorf("locomo: no conversations found in %s", dataPath)
	}
	fmt.Printf("📂 Loaded %d conversations from %s\n", len(convs), dataPath)

	scenarios := locomo.ToScenarios(convs, limit)
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("locomo: no valid scenarios generated from %s", dataPath)
	}
	fmt.Printf("✅ Converted to %d scenarios\n", len(scenarios))

	fmt.Printf("🔄 Running evaluation...\n")
	metrics, err := br.EvalRunner.RunAll(ctx, scenarios)
	if err != nil {
		return nil, fmt.Errorf("locomo run: %w", err)
	}
	fmt.Printf("✅ Evaluation complete: %d scenarios\n", len(metrics))

	results := make([]*BenchmarkResult, len(metrics))
	for i, m := range metrics {
		results[i] = &BenchmarkResult{
			Dataset:   "locomo",
			Category:  "conversation",
			Metric:    m,
			Baselines: LoCoMoBaselines,
		}
	}
	return results, nil
}

// RunLongMemEval loads items from dataPath, converts them to scenarios,
// runs them, and attaches LongMemEvalBaselines. Each item's category
// is preserved for grouped reporting.
// Pass limit ≤ 0 to run all items.
func (br *BenchmarkRunner) RunLongMemEval(ctx context.Context, dataPath string, limit int) ([]*BenchmarkResult, error) {
	items, err := longmemeval.LoadItems(dataPath)
	if err != nil {
		return nil, fmt.Errorf("longmemeval load: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("longmemeval: no items found in %s", dataPath)
	}

	scenarios := longmemeval.ToScenarios(items, limit)
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("longmemeval: no valid scenarios generated from %s", dataPath)
	}

	metrics, err := br.EvalRunner.RunAll(ctx, scenarios)
	if err != nil {
		return nil, fmt.Errorf("longmemeval run: %w", err)
	}

	results := make([]*BenchmarkResult, len(metrics))
	for i, m := range metrics {
		results[i] = &BenchmarkResult{
			Dataset:   "longmemeval",
			Category:  categoryFromLMEScenario(m.Scenario),
			Metric:    m,
			Baselines: LongMemEvalBaselines,
		}
	}
	return results, nil
}

// categoryFromLMEScenario extracts the category from a scenario name produced
// by the LongMemEval adapter: "lme-{category}-{id}" → "{category}".
func categoryFromLMEScenario(name string) string {
	// Strip the "lme-" prefix.
	s := strings.TrimPrefix(name, "lme-")
	// Category is everything before the last "-{id}" segment.
	// Items IDs may not contain hyphens, but categories might (e.g.
	// "multi_session_reasoning"). We split on the first underscore-free dash.
	// Simpler heuristic: the category is the second token when splitting on "-".
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return s
}
