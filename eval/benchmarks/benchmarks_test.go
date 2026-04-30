package benchmarks_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-wiki/eval/benchmarks"
	"github.com/tingly-dev/tingly-wiki/eval/benchmarks/locomo"
	"github.com/tingly-dev/tingly-wiki/eval/benchmarks/longmemeval"
)

const (
	locomoFixture  = "locomo/testdata/fixture.json"
	longmemFixture = "longmemeval/testdata/fixture.json"
)

func TestLoCoMoAdapter_LoadAndConvert(t *testing.T) {
	convs, err := locomo.LoadConversations(locomoFixture)
	if err != nil {
		t.Fatalf("LoadConversations: %v", err)
	}
	if len(convs) == 0 {
		t.Fatal("expected at least one conversation")
	}

	scenarios := locomo.ToScenarios(convs, 0)
	if len(scenarios) == 0 {
		t.Fatal("expected at least one scenario")
	}

	sc := scenarios[0]
	if sc.Name == "" {
		t.Error("scenario name is empty")
	}
	if len(sc.Setup) == 0 {
		t.Error("expected setup ops from sessions")
	}
	if len(sc.Queries) == 0 {
		t.Error("expected queries from QA pairs")
	}
	// Each query must have at least one relevant path.
	for i, q := range sc.Queries {
		if len(q.RelevantPaths) == 0 {
			t.Errorf("query[%d] has no relevant paths", i)
		}
	}
}

func TestLoCoMoAdapter_Limit(t *testing.T) {
	convs, _ := locomo.LoadConversations(locomoFixture)
	// Fixture has 1 conversation; limit=0 returns all.
	all := locomo.ToScenarios(convs, 0)
	limited := locomo.ToScenarios(convs, 1)
	if len(limited) > len(all) {
		t.Errorf("limit=1 returned more scenarios (%d) than all (%d)", len(limited), len(all))
	}
}

func TestLongMemEvalAdapter_LoadAndConvert(t *testing.T) {
	items, err := longmemeval.LoadItems(longmemFixture)
	if err != nil {
		t.Fatalf("LoadItems: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected ≥2 items, got %d", len(items))
	}

	scenarios := longmemeval.ToScenarios(items, 0)
	if len(scenarios) != len(items) {
		t.Errorf("expected %d scenarios (one per item), got %d", len(items), len(scenarios))
	}

	for i, sc := range scenarios {
		if sc.Name == "" {
			t.Errorf("scenarios[%d] has empty name", i)
		}
		if len(sc.Setup) == 0 {
			t.Errorf("scenarios[%d] has no setup ops", i)
		}
		if len(sc.Queries) != 1 {
			t.Errorf("scenarios[%d] expected 1 query, got %d", i, len(sc.Queries))
		}
	}
}

func TestLongMemEvalAdapter_RelevantPaths(t *testing.T) {
	items, _ := longmemeval.LoadItems(longmemFixture)
	// First item has explicit relevant_session_ids = ["session_1"]
	scenarios := longmemeval.ToScenarios(items[:1], 0)
	if len(scenarios) == 0 {
		t.Fatal("no scenario generated")
	}
	q := scenarios[0].Queries[0]
	if len(q.RelevantPaths) != 1 {
		t.Errorf("expected 1 relevant path, got %d: %v", len(q.RelevantPaths), q.RelevantPaths)
	}
	if !strings.Contains(q.RelevantPaths[0], "session_1") {
		t.Errorf("expected relevant path to contain session_1, got %q", q.RelevantPaths[0])
	}
}

func TestBenchmarkRunner_LoCoMo(t *testing.T) {
	runner := benchmarks.NewBenchmarkRunner(nil)
	results, err := runner.RunLoCoMo(context.Background(), locomoFixture, 0)
	if err != nil {
		t.Fatalf("RunLoCoMo: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, r := range results {
		if r.Dataset != "locomo" {
			t.Errorf("dataset = %q, want locomo", r.Dataset)
		}
		if r.Metric == nil {
			t.Error("nil metric")
		}
		if len(r.Baselines) == 0 {
			t.Error("empty baselines")
		}
		// mem0 baseline must be present and reasonable.
		if r.Baselines["mem0"] < 0.5 {
			t.Errorf("mem0 baseline = %f, want ≥ 0.5", r.Baselines["mem0"])
		}
	}
}

func TestBenchmarkRunner_LongMemEval(t *testing.T) {
	runner := benchmarks.NewBenchmarkRunner(nil)
	results, err := runner.RunLongMemEval(context.Background(), longmemFixture, 0)
	if err != nil {
		t.Fatalf("RunLongMemEval: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, r := range results {
		if r.Dataset != "longmemeval" {
			t.Errorf("dataset = %q, want longmemeval", r.Dataset)
		}
		if r.Category == "" {
			t.Error("empty category")
		}
		if r.Metric == nil {
			t.Error("nil metric")
		}
	}
}

func TestToMarkdown_Structure(t *testing.T) {
	runner := benchmarks.NewBenchmarkRunner(nil)
	locomoResults, err := runner.RunLoCoMo(context.Background(), locomoFixture, 0)
	if err != nil {
		t.Fatalf("RunLoCoMo: %v", err)
	}
	lmeResults, err := runner.RunLongMemEval(context.Background(), longmemFixture, 0)
	if err != nil {
		t.Fatalf("RunLongMemEval: %v", err)
	}

	all := append(locomoResults, lmeResults...)
	md := benchmarks.ToMarkdown(all)

	if !strings.HasPrefix(md, "# Benchmark Comparison Report") {
		t.Errorf("missing top-level header:\n%s", md)
	}
	if !strings.Contains(md, "## Locomo") {
		t.Errorf("missing ## Locomo section:\n%s", md)
	}
	if !strings.Contains(md, "## Longmemeval") {
		t.Errorf("missing ## Longmemeval section:\n%s", md)
	}
	if !strings.Contains(md, "mem0") {
		t.Errorf("missing baseline column:\n%s", md)
	}
}

func TestToMarkdown_Empty(t *testing.T) {
	md := benchmarks.ToMarkdown(nil)
	if !strings.Contains(md, "No results") {
		t.Errorf("expected empty-results message, got:\n%s", md)
	}
}

func TestToJSON(t *testing.T) {
	runner := benchmarks.NewBenchmarkRunner(nil)
	results, err := runner.RunLoCoMo(context.Background(), locomoFixture, 0)
	if err != nil {
		t.Fatalf("RunLoCoMo: %v", err)
	}

	b, err := benchmarks.ToJSON(results)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	js := string(b)
	for _, field := range []string{`"dataset"`, `"scenario"`, `"mrr"`, `"baselines"`} {
		if !strings.Contains(js, field) {
			t.Errorf("JSON missing field %q:\n%s", field, js)
		}
	}
}
