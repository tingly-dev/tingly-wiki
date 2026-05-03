package wiki

import (
	"context"
	"regexp"
	"testing"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// queryLinkExtractor pretends the query mentions a fixed entity name. Useful
// for tests where DefaultLinkPatterns would not match the toy query string.
type queryLinkExtractor struct{ subject, predicate, object string }

func (q queryLinkExtractor) Extract(_ context.Context, _ string) ([]ExtractedLink, error) {
	return []ExtractedLink{{Subject: q.subject, Predicate: q.predicate, Object: q.object}}, nil
}

// TestRetriever_EntityBoostPromotesEntityPage verifies that, with a
// LinkExtractor wired in, a page mentioning an entity from the query ranks
// above a page that doesn't — even when neither shares vocabulary with the
// raw query string.
func TestRetriever_EntityBoostPromotesEntityPage(t *testing.T) {
	fx := newFixture(false) // no vector index, isolate entity contribution
	ctx := context.Background()

	// Page A is about Alice; Page B is about Bob. Neither contains the literal
	// query token "tellme" — so vanilla keyword search would tie at zero.
	pageA := &schema.Page{
		Path: "memories/a.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "Alice meeting", Importance: 0.5,
		},
		Content: "Alice presented the roadmap.",
	}
	pageB := &schema.Page{
		Path: "memories/b.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "Bob meeting", Importance: 0.5,
		},
		Content: "Bob reviewed the roadmap.",
	}
	fx.addPage(t, pageA, nil)
	fx.addPage(t, pageB, nil)

	// Wire a LinkExtractor that always claims the query mentions "Alice".
	fx.r.SetLinkExtractor(queryLinkExtractor{subject: "user", predicate: "talked_about", object: "Alice"})

	scored, err := fx.r.Recall(ctx, "tellme", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
		Limit: 10,
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) == 0 {
		t.Fatal("expected entity-boosted hits, got 0")
	}
	if scored[0].Page.Path != "memories/a.md" {
		t.Errorf("entity boost should promote Alice page; got top=%s", scored[0].Page.Path)
	}
}

// TestRetriever_EntityBoostDormantWithoutExtractor confirms backward compat:
// without a LinkExtractor, recall behavior is unchanged from prior versions.
func TestRetriever_EntityBoostDormantWithoutExtractor(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	fx.addPage(t, &schema.Page{
		Path: "memories/x.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeMemory, Title: "x", Importance: 0.5,
		},
		Content: "shared keyword content",
	}, nil)

	scored, err := fx.r.Recall(ctx, "shared", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeMemory},
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(scored))
	}
}

// TestRetriever_NoopLinkExtractorTreatedAsDisabled documents that passing a
// NoopLinkExtractor leaves the boost path dormant — the same as nil.
func TestRetriever_NoopLinkExtractorTreatedAsDisabled(t *testing.T) {
	fx := newFixture(false)

	fx.r.SetLinkExtractor(NoopLinkExtractor{})
	if fx.r.linkExtractor != nil {
		t.Errorf("NoopLinkExtractor should be treated as nil, got %T", fx.r.linkExtractor)
	}
}

// TestRetriever_RegexLinkExtractorIntegration wires a real RegexLinkExtractor
// to confirm the production extractor reaches the retriever via a query that
// matches DefaultLinkPatterns.
func TestRetriever_RegexLinkExtractorIntegration(t *testing.T) {
	fx := newFixture(false)
	ctx := context.Background()

	// Page about OpenAI; query mentions OpenAI through a "works at" pattern.
	fx.addPage(t, &schema.Page{
		Path: "entities/openai.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeEntity, Title: "OpenAI", Importance: 0.5,
		},
		Content: "OpenAI is an AI research company.",
	}, nil)
	fx.addPage(t, &schema.Page{
		Path: "entities/microsoft.md",
		Frontmatter: schema.Frontmatter{
			Type: schema.PageTypeEntity, Title: "Microsoft", Importance: 0.5,
		},
		Content: "Microsoft develops Windows.",
	}, nil)

	// Use a single pattern matching "X works at Y"
	patterns := []LinkPattern{
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+)\s+works\s+at\s+(?P<obj>[A-Z][a-zA-Z]+)`),
			Predicate: "works_at",
		},
	}
	fx.r.SetLinkExtractor(NewRegexLinkExtractor(patterns))

	// Query has "Sam works at OpenAI" — entity boost should fire on "OpenAI".
	scored, err := fx.r.Recall(ctx, "Sam works at OpenAI", &RecallOptions{
		Types: []schema.PageType{schema.PageTypeEntity},
		Limit: 10,
	}, fx.storage)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(scored) == 0 {
		t.Fatal("expected at least one entity hit")
	}
	if scored[0].Page.Path != "entities/openai.md" {
		t.Errorf("OpenAI page should rank first via entity boost; got top=%s", scored[0].Page.Path)
	}
}
