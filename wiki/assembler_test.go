package wiki

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

func makePref(title, content string, importance float64, tenant string) *schema.Page {
	now := time.Now()
	return &schema.Page{
		Path: "preferences/" + title + ".md",
		Frontmatter: schema.Frontmatter{
			Type:       schema.PageTypePreference,
			Title:      title,
			Importance: importance,
			TenantID:   tenant,
		},
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makeMem(title, content string, importance float64, tenant string) *schema.Page {
	p := makePref(title, content, importance, tenant)
	p.Path = "memories/" + title + ".md"
	p.Type = schema.PageTypeMemory
	return p
}

func makeProc(title, content string, importance float64, tenant string) *schema.Page {
	p := makePref(title, content, importance, tenant)
	p.Path = "procedures/" + title + ".md"
	p.Type = schema.PageTypeProcedure
	return p
}

func TestAssembler_Defaults(t *testing.T) {
	st := storage.NewMemoryStorage()
	a := NewDefaultAssembler(st, nil)

	got, err := a.Assemble(context.Background(), nil)
	if err != nil {
		t.Fatalf("Assemble(nil) errored: %v", err)
	}
	if got.Text != "" {
		t.Errorf("empty storage should yield empty text, got %q", got.Text)
	}
	if got.Stats.PagesIncluded != 0 {
		t.Errorf("PagesIncluded = %d, want 0", got.Stats.PagesIncluded)
	}
}

func TestAssembler_LayerOrdering(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	// Write one of each layer in REVERSE display order to verify ordering is enforced
	_ = st.WritePage(ctx, makeMem("recent-event", "discussed roadmap", 0.7, ""))
	_ = st.WritePage(ctx, makeProc("ticket-flow", "step 1, step 2", 0.7, ""))
	_ = st.WritePage(ctx, makePref("dark-mode", "user prefers dark theme", 0.7, ""))

	a := NewDefaultAssembler(st, nil)
	got, err := a.Assemble(ctx, &AssembleOptions{Format: AssembleFormatMarkdown})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	prefIdx := strings.Index(got.Text, "## User Preferences")
	procIdx := strings.Index(got.Text, "## Active Procedures")
	memIdx := strings.Index(got.Text, "## Recent Memories")

	if prefIdx == -1 || procIdx == -1 || memIdx == -1 {
		t.Fatalf("expected all 3 sections, got:\n%s", got.Text)
	}
	if !(prefIdx < procIdx && procIdx < memIdx) {
		t.Errorf("section order wrong: pref=%d proc=%d mem=%d\n%s", prefIdx, procIdx, memIdx, got.Text)
	}
}

func TestAssembler_ImportanceSort(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	_ = st.WritePage(ctx, makePref("low-importance", "low", 0.1, ""))
	_ = st.WritePage(ctx, makePref("high-importance", "high", 0.95, ""))
	_ = st.WritePage(ctx, makePref("mid-importance", "mid", 0.5, ""))

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
	})

	// "high-importance" should appear before "mid-importance" before "low-importance"
	hi := strings.Index(got.Text, "high-importance")
	mi := strings.Index(got.Text, "mid-importance")
	lo := strings.Index(got.Text, "low-importance")
	if hi == -1 || mi == -1 || lo == -1 {
		t.Fatalf("missing entries:\n%s", got.Text)
	}
	if !(hi < mi && mi < lo) {
		t.Errorf("importance order wrong: hi=%d mi=%d lo=%d\n%s", hi, mi, lo, got.Text)
	}
}

func TestAssembler_MaxCharsBudget(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	// Each preference content is ~50 chars; 10 of them = 500+ chars total.
	for i := 0; i < 10; i++ {
		_ = st.WritePage(ctx, makePref(
			"pref"+string(rune('a'+i)),
			"this is a moderately long content body for testing budget enforcement",
			float64(10-i)/10.0, // descending importance so later ones are skipped first
			"",
		))
	}

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers:   []schema.PageType{schema.PageTypePreference},
		MaxChars: 200,
	})

	if len(got.Text) > 200 {
		t.Errorf("text length %d exceeds MaxChars=200", len(got.Text))
	}
	if got.Stats.PagesIncluded == 0 {
		t.Errorf("expected at least 1 page included, got 0")
	}
	if got.Stats.PagesSkipped == 0 {
		t.Errorf("expected some pages skipped due to budget, got 0")
	}
	if got.Stats.PagesIncluded+got.Stats.PagesSkipped != 10 {
		t.Errorf("included(%d) + skipped(%d) should equal 10",
			got.Stats.PagesIncluded, got.Stats.PagesSkipped)
	}
}

func TestAssembler_TenantFilter(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	_ = st.WritePage(ctx, makePref("p-A", "tenant A pref", 0.5, "user-A"))
	_ = st.WritePage(ctx, makePref("p-B", "tenant B pref", 0.5, "user-B"))

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers:   []schema.PageType{schema.PageTypePreference},
		TenantID: "user-A",
	})

	if !strings.Contains(got.Text, "p-A") {
		t.Errorf("expected p-A in output\n%s", got.Text)
	}
	if strings.Contains(got.Text, "p-B") {
		t.Errorf("p-B leaked across tenant boundary\n%s", got.Text)
	}
}

func TestAssembler_ExpiredSkipped(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	expired := makePref("stale", "old data", 0.9, "")
	expired.ExpiresAt = &past
	_ = st.WritePage(ctx, expired)

	fresh := makePref("alive", "current data", 0.5, "")
	_ = st.WritePage(ctx, fresh)

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
	})

	if strings.Contains(got.Text, "stale") {
		t.Errorf("expired page should be skipped\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "alive") {
		t.Errorf("fresh page missing\n%s", got.Text)
	}
}

func TestAssembler_JSONFormat(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	_ = st.WritePage(ctx, makePref("language", "Chinese", 0.9, ""))

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
		Format: AssembleFormatJSON,
	})

	if !strings.HasPrefix(got.Text, "[") || !strings.HasSuffix(got.Text, "]") {
		t.Errorf("JSON output should be a JSON array, got %q", got.Text)
	}
	if !strings.Contains(got.Text, `"type":"preference"`) {
		t.Errorf("JSON should contain type field, got %q", got.Text)
	}
}

func TestAssembler_ActiveFactsRendered(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	now := time.Now()
	p := makePref("compound", "user has multiple preferences", 0.8, "")
	p.Facts = []schema.MemoryFact{
		{Subject: "user", Predicate: "prefers", Object: "dark mode"},
		{Subject: "user", Predicate: "speaks", Object: "Chinese", InvalidatedAt: &now}, // hidden
	}
	_ = st.WritePage(ctx, p)

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
	})

	if !strings.Contains(got.Text, "dark mode") {
		t.Errorf("active fact missing\n%s", got.Text)
	}
	if strings.Contains(got.Text, "Chinese") {
		t.Errorf("invalidated fact leaked\n%s", got.Text)
	}
}

func TestAssembler_CitationsCollected(t *testing.T) {
	st := storage.NewMemoryStorage()
	ctx := context.Background()

	_ = st.WritePage(ctx, makePref("a", "x", 0.5, ""))
	_ = st.WritePage(ctx, makePref("b", "x", 0.5, ""))

	a := NewDefaultAssembler(st, nil)
	got, _ := a.Assemble(ctx, &AssembleOptions{
		Layers: []schema.PageType{schema.PageTypePreference},
	})

	if len(got.Citations) != 2 {
		t.Errorf("expected 2 citations, got %d", len(got.Citations))
	}
}
