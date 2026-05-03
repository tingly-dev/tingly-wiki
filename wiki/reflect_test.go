package wiki

import (
	"context"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-wiki/llm"
	"github.com/tingly-dev/tingly-wiki/schema"
)

// TestReflect_WritesSynthesisWithSources verifies Reflect creates a synthesis
// page whose Sources field cites the input memory paths.
func TestReflect_WritesSynthesisWithSources(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	// Configure mock LLM to fabricate a synthesis citing both inputs.
	mock := mw.WikiImpl.llm.(*llm.MockLLM)
	mock.ReflectFunc = func(_ context.Context, sources []llm.ReflectInput) ([]llm.ReflectSynthesis, error) {
		paths := make([]string, len(sources))
		for i, s := range sources {
			paths[i] = s.Path
		}
		return []llm.ReflectSynthesis{{
			Title:   "Cross-Cutting Insight",
			Content: "Combining the inputs reveals a pattern.",
			Sources: paths,
		}}, nil
	}

	// Seed two memory pages.
	for _, title := range []string{"alpha", "beta"} {
		_, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
			Type:       schema.PageTypeMemory,
			Title:      title,
			Content:    "Observation about " + title,
			Importance: 0.7,
		})
		if err != nil {
			t.Fatalf("StoreMemory(%s): %v", title, err)
		}
	}

	res, err := mw.Reflect(ctx, &ReflectOptions{MaxInputs: 5})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}
	if res.InputsConsidered != 2 {
		t.Errorf("InputsConsidered=%d, want 2", res.InputsConsidered)
	}
	if len(res.Planned) != 1 || res.Planned[0] != "Cross-Cutting Insight" {
		t.Errorf("Planned=%v, want ['Cross-Cutting Insight']", res.Planned)
	}
	if len(res.SynthesisCreated) != 1 {
		t.Fatalf("SynthesisCreated=%v, want 1 path", res.SynthesisCreated)
	}

	// Verify the page was actually written and links back to inputs.
	page, err := mw.storage.ReadPage(ctx, res.SynthesisCreated[0])
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if page.Type != schema.PageTypeSynthesis {
		t.Errorf("Type=%s, want synthesis", page.Type)
	}
	if len(page.Sources) != 2 {
		t.Errorf("Sources=%v, want 2 entries", page.Sources)
	}
	for _, src := range page.Sources {
		if !strings.Contains(src, "alpha") && !strings.Contains(src, "beta") {
			t.Errorf("unexpected source: %s", src)
		}
	}
}

// TestReflect_DryRunDoesNotWrite confirms DryRun returns the plan without
// touching storage.
func TestReflect_DryRunDoesNotWrite(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	_, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:       schema.PageTypeMemory,
		Title:      "input",
		Content:    "an observation",
		Importance: 0.7,
	})
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	res, err := mw.Reflect(ctx, &ReflectOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Reflect: %v", err)
	}
	if !res.DryRun {
		t.Error("DryRun flag not propagated to result")
	}
	if len(res.Planned) == 0 {
		t.Error("expected Planned titles in dry-run result")
	}
	if len(res.SynthesisCreated) != 0 {
		t.Errorf("DryRun should not write pages, got %d", len(res.SynthesisCreated))
	}
}

// TestReflect_NoOpWhenLLMNotReflector ensures Reflect succeeds silently when
// the configured LLM does not implement llm.Reflector.
func TestReflect_NoOpWhenLLMNotReflector(t *testing.T) {
	// Build a MemoryWikiImpl by hand so we can inject a non-reflector LLM.
	mw := asOfTestFixture(t)
	ctx := context.Background()

	mw.WikiImpl.llm = nonRerankerLLM{} // also lacks Reflect

	_, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
		Type:    schema.PageTypeMemory,
		Title:   "x",
		Content: "x content",
	})
	// StoreMemory uses LLM internally; nonRerankerLLM returns 0.5 importance,
	// nil facts, etc. — that's fine.
	if err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	mw.llm = nonRerankerLLM{}
	res, err := mw.Reflect(ctx, nil)
	if err != nil {
		t.Fatalf("Reflect should silently succeed, got error: %v", err)
	}
	if len(res.SynthesisCreated) != 0 || len(res.Planned) != 0 {
		t.Errorf("expected no syntheses when LLM lacks Reflector, got %+v", res)
	}
}

// TestReflect_QueryFocusesInputs uses opts.Query to scope which pages are
// passed to the reflector.
func TestReflect_QueryFocusesInputs(t *testing.T) {
	mw := asOfTestFixture(t)
	ctx := context.Background()

	mock := mw.WikiImpl.llm.(*llm.MockLLM)
	var sentTitles []string
	mock.ReflectFunc = func(_ context.Context, sources []llm.ReflectInput) ([]llm.ReflectSynthesis, error) {
		for _, s := range sources {
			sentTitles = append(sentTitles, s.Title)
		}
		return []llm.ReflectSynthesis{{Title: "T", Content: "C", Sources: nil}}, nil
	}

	for _, p := range []struct{ title, content string }{
		{"target topic", "content about target"},
		{"target deep", "more target content"},
		{"unrelated", "totally different topic"},
	} {
		if _, err := mw.StoreMemory(ctx, &StoreMemoryRequest{
			Type:       schema.PageTypeMemory,
			Title:      p.title,
			Content:    p.content,
			Importance: 0.6,
		}); err != nil {
			t.Fatalf("StoreMemory(%s): %v", p.title, err)
		}
	}

	// MaxInputs=2 so the top-2 query-relevant pages come through and the
	// unrelated page is cut off.
	if _, err := mw.Reflect(ctx, &ReflectOptions{Query: "target", MaxInputs: 2}); err != nil {
		t.Fatalf("Reflect: %v", err)
	}
	if len(sentTitles) != 2 {
		t.Fatalf("MaxInputs=2 should yield 2 sources, got %d (%v)", len(sentTitles), sentTitles)
	}
	for _, title := range sentTitles {
		if title == "unrelated" {
			t.Errorf("query='target' should rank target pages above 'unrelated'; got %v", sentTitles)
		}
	}
}
