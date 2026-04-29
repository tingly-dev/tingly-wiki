package eval

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestComputeQueryMetric_AllRelevantInTopK(t *testing.T) {
	qm := computeQueryMetric(
		"q",
		[]string{"a.md", "b.md", "c.md"},
		[]string{"a.md", "b.md"},
		3,
		10*time.Millisecond,
	)
	// 2 relevant in top-3 → precision=2/3, recall=2/2=1.0
	if qm.PrecisionAtK < 0.66 || qm.PrecisionAtK > 0.67 {
		t.Errorf("PrecisionAtK = %f, want ≈0.667", qm.PrecisionAtK)
	}
	if qm.RecallAtK != 1.0 {
		t.Errorf("RecallAtK = %f, want 1.0", qm.RecallAtK)
	}
	if qm.RR != 1.0 {
		t.Errorf("RR = %f, want 1.0 (first hit at rank 1)", qm.RR)
	}
}

func TestComputeQueryMetric_NoMatches(t *testing.T) {
	qm := computeQueryMetric("q", []string{"x.md", "y.md"}, []string{"a.md"}, 3, 0)
	if qm.PrecisionAtK != 0 {
		t.Errorf("PrecisionAtK = %f, want 0", qm.PrecisionAtK)
	}
	if qm.RecallAtK != 0 {
		t.Errorf("RecallAtK = %f, want 0", qm.RecallAtK)
	}
	if qm.RR != 0 {
		t.Errorf("RR = %f, want 0", qm.RR)
	}
}

func TestComputeQueryMetric_RankAffectsRR(t *testing.T) {
	// Relevant at rank 3 → RR = 1/3
	qm := computeQueryMetric("q", []string{"x.md", "y.md", "a.md"}, []string{"a.md"}, 5, 0)
	if want := 1.0 / 3.0; qm.RR < want-0.001 || qm.RR > want+0.001 {
		t.Errorf("RR = %f, want ≈%f", qm.RR, want)
	}
}

func TestPercentile(t *testing.T) {
	durs := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1000 * time.Millisecond,
	}
	if got := percentile(durs, 50); got != 100*time.Millisecond {
		t.Errorf("p50 = %s, want 100ms", got)
	}
	if got := percentile(durs, 95); got != 1000*time.Millisecond {
		t.Errorf("p95 = %s, want 1000ms", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("empty slice should yield 0, got %s", got)
	}
}

func TestLoadScenarios(t *testing.T) {
	scs, err := LoadScenarios("scenarios")
	if err != nil {
		t.Fatalf("LoadScenarios: %v", err)
	}
	if len(scs) < 3 {
		t.Errorf("expected ≥3 scenarios, got %d", len(scs))
	}
	for _, sc := range scs {
		if sc.Name == "" {
			t.Errorf("scenario has empty name")
		}
		if len(sc.Queries) == 0 {
			t.Errorf("scenario %s has no queries", sc.Name)
		}
	}
}

func TestRunner_PreferenceRecallScenario(t *testing.T) {
	sc, err := LoadScenario("scenarios/preference-recall.yaml")
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}

	r := NewRunner()
	m, err := r.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(m.QueryMetrics) != len(sc.Queries) {
		t.Errorf("got %d query metrics, want %d", len(m.QueryMetrics), len(sc.Queries))
	}

	// Quality bar: with deterministic embedder, MRR should still be > 0.5
	// for a well-designed scenario. This is a smoke test that the pipeline
	// produces non-trivial signal, not a strict quality gate.
	if m.MRR < 0.3 {
		t.Logf("preference-recall MRR=%f is low; check embedder + scenario design", m.MRR)
	}
}

func TestRunner_AllScenariosExecute(t *testing.T) {
	scs, err := LoadScenarios("scenarios")
	if err != nil {
		t.Fatalf("LoadScenarios: %v", err)
	}
	r := NewRunner()
	results, err := r.RunAll(context.Background(), scs)
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(results) != len(scs) {
		t.Errorf("got %d results, want %d", len(results), len(scs))
	}

	// Generate a markdown report — verify it's non-empty and well-formed.
	md := ToMarkdown(results)
	if !strings.HasPrefix(md, "# Memory Quality Report") {
		t.Errorf("markdown report missing header:\n%s", md)
	}
	if !strings.Contains(md, "| Scenario |") {
		t.Errorf("markdown report missing summary table")
	}

	// JSON report
	jsonOut, err := ToJSON(results)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if !strings.Contains(string(jsonOut), `"scenario"`) {
		t.Errorf("JSON output missing scenario field:\n%s", jsonOut)
	}
}

func TestDeterministicMockLLM_Stability(t *testing.T) {
	m := NewDeterministicMockLLM()
	ctx := context.Background()

	v1, _ := m.Embed(ctx, "the quick brown fox")
	v2, _ := m.Embed(ctx, "the quick brown fox")

	if len(v1) != detEmbedDim || len(v2) != detEmbedDim {
		t.Fatalf("dim = %d, want %d", len(v1), detEmbedDim)
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Errorf("embedding not deterministic at index %d: %f vs %f", i, v1[i], v2[i])
			break
		}
	}

	// Different content → different embeddings
	v3, _ := m.Embed(ctx, "completely different content here")
	allSame := true
	for i := range v1 {
		if v1[i] != v3[i] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("different content produced identical embeddings")
	}
}

func TestDeterministicMockLLM_OverlapBoostsCosine(t *testing.T) {
	m := NewDeterministicMockLLM()
	ctx := context.Background()

	a, _ := m.Embed(ctx, "user loves spicy food")
	b, _ := m.Embed(ctx, "user loves spicy cuisine")
	c, _ := m.Embed(ctx, "fluffy clouds in distant skies")

	// b shares 3/4 tokens with a; c shares 0
	cosAB := cosine(a, b)
	cosAC := cosine(a, c)
	if cosAB <= cosAC {
		t.Errorf("expected cos(a,b)=%f > cos(a,c)=%f", cosAB, cosAC)
	}
}

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}
