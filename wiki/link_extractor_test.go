package wiki

import (
	"context"
	"regexp"
	"testing"
)

func TestNoopLinkExtractor(t *testing.T) {
	le := NoopLinkExtractor{}
	links, err := le.Extract(context.Background(), "Anything goes here.")
	if err != nil {
		t.Errorf("NoopLinkExtractor should not error, got: %v", err)
	}
	if links != nil {
		t.Errorf("NoopLinkExtractor should return nil, got %v", links)
	}
}

func TestRegexLinkExtractor_DefaultPatterns(t *testing.T) {
	le := NewRegexLinkExtractor(DefaultLinkPatterns())
	tests := []struct {
		name     string
		content  string
		wantPred string
		wantSubj string
		wantObj  string
	}{
		{
			name:     "works_at",
			content:  "Pedro works at Brex.",
			wantPred: "works_at",
			wantSubj: "Pedro",
			wantObj:  "Brex",
		},
		{
			name:     "leads (CEO)",
			content:  "Sam Altman is the CEO of OpenAI.",
			wantPred: "leads",
			wantSubj: "Sam Altman",
			wantObj:  "OpenAI",
		},
		{
			name:     "lives_in",
			content:  "Alice lives in San Francisco.",
			wantPred: "lives_in",
			wantSubj: "Alice",
			wantObj:  "San Francisco",
		},
		{
			name:     "founded",
			content:  "Bob founded Acme.",
			wantPred: "founded",
			wantSubj: "Bob",
			wantObj:  "Acme",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, err := le.Extract(context.Background(), tt.content)
			if err != nil {
				t.Fatalf("Extract failed: %v", err)
			}
			found := false
			for _, l := range links {
				if l.Predicate == tt.wantPred && l.Subject == tt.wantSubj && l.Object == tt.wantObj {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected (%s, %s, %s); got %v", tt.wantSubj, tt.wantPred, tt.wantObj, links)
			}
		})
	}
}

func TestRegexLinkExtractor_Dedup(t *testing.T) {
	le := NewRegexLinkExtractor(DefaultLinkPatterns())
	// Same fact stated twice in different sentences
	content := "Pedro works at Brex. Later, Pedro works at Brex again."
	links, _ := le.Extract(context.Background(), content)

	count := 0
	for _, l := range links {
		if l.Predicate == "works_at" && l.Subject == "Pedro" && l.Object == "Brex" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected dedup → 1 entry, got %d (links=%v)", count, links)
	}
}

func TestRegexLinkExtractor_EmptyAndNoMatch(t *testing.T) {
	le := NewRegexLinkExtractor(DefaultLinkPatterns())

	// Empty input
	links, err := le.Extract(context.Background(), "")
	if err != nil {
		t.Errorf("empty input should not error, got: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("empty input should yield no links, got %v", links)
	}

	// Plain text without entities
	links, _ = le.Extract(context.Background(), "the quick brown fox jumps over the lazy dog")
	if len(links) != 0 {
		t.Errorf("non-matching text should yield no links, got %v", links)
	}
}

func TestRegexLinkExtractor_CustomPattern(t *testing.T) {
	custom := []LinkPattern{
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>\w+)\s+invested\s+in\s+(?P<obj>\w+)`),
			Predicate: "invested_in",
		},
	}
	le := NewRegexLinkExtractor(custom)
	links, _ := le.Extract(context.Background(), "Sequoia invested in Stripe.")

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Predicate != "invested_in" {
		t.Errorf("predicate=%s, want invested_in", links[0].Predicate)
	}
	if links[0].Subject != "Sequoia" || links[0].Object != "Stripe" {
		t.Errorf("got (%s, %s), want (Sequoia, Stripe)", links[0].Subject, links[0].Object)
	}
}
