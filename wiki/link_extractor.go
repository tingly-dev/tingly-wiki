package wiki

import (
	"context"
	"regexp"
	"strings"
)

// ExtractedLink is a typed directed relationship parsed from page content.
type ExtractedLink struct {
	// Subject is the entity the link originates from (often the page title or "user")
	Subject string

	// Predicate is the relationship type (e.g. "works_at", "attended", "lives_in")
	Predicate string

	// Object is the target entity or value
	Object string

	// SourceSpan is the original text fragment that triggered the match (for debugging)
	SourceSpan string
}

// LinkExtractor parses entity relationships from free-form text.
// Implementations range from zero-LLM regex patterns (fast, low recall)
// to full NER/LLM pipelines (slower, higher recall).
// Extracted links are used to populate Page.Frontmatter.Related automatically.
type LinkExtractor interface {
	// Extract returns typed links found in content.
	// Returns an empty slice (not an error) when no links are detected.
	Extract(ctx context.Context, content string) ([]ExtractedLink, error)
}

// NoopLinkExtractor always returns no links.
// Default when the caller does not supply a LinkExtractor.
type NoopLinkExtractor struct{}

func (NoopLinkExtractor) Extract(_ context.Context, _ string) ([]ExtractedLink, error) {
	return nil, nil
}

// LinkPattern pairs a compiled regexp with the predicate it signals.
// The regexp must have exactly two named capture groups: "subj" and "obj".
//
// Example:
//
//	LinkPattern{
//	  Re:        regexp.MustCompile(`(?i)(?P<subj>\w+)\s+works\s+(?:at|for)\s+(?P<obj>[\w\s]+)`),
//	  Predicate: "works_at",
//	}
type LinkPattern struct {
	Re        *regexp.Regexp
	Predicate string
}

// RegexLinkExtractor applies a configurable set of LinkPattern rules.
// It runs zero LLM calls and is suitable for high-volume ingestion pipelines.
//
// DefaultLinkPatterns provides a ready-to-use English starter set; callers can
// extend or replace it to match their domain vocabulary.
type RegexLinkExtractor struct {
	Patterns []LinkPattern
}

// NewRegexLinkExtractor creates a RegexLinkExtractor with the given patterns.
// Pass DefaultLinkPatterns() as a starting point.
func NewRegexLinkExtractor(patterns []LinkPattern) *RegexLinkExtractor {
	return &RegexLinkExtractor{Patterns: patterns}
}

// Extract scans content against all patterns and returns deduplicated links.
func (r *RegexLinkExtractor) Extract(_ context.Context, content string) ([]ExtractedLink, error) {
	seen := make(map[string]bool)
	var links []ExtractedLink

	for _, p := range r.Patterns {
		matches := p.Re.FindAllStringSubmatch(content, -1)
		names := p.Re.SubexpNames()

		for _, m := range matches {
			var subj, obj, span string
			for i, name := range names {
				switch name {
				case "subj":
					subj = strings.TrimSpace(m[i])
				case "obj":
					obj = strings.TrimSpace(m[i])
				}
			}
			span = m[0]
			if subj == "" || obj == "" {
				continue
			}
			key := p.Predicate + "|" + strings.ToLower(subj) + "|" + strings.ToLower(obj)
			if seen[key] {
				continue
			}
			seen[key] = true
			links = append(links, ExtractedLink{
				Subject:    subj,
				Predicate:  p.Predicate,
				Object:     obj,
				SourceSpan: span,
			})
		}
	}
	return links, nil
}

// DefaultLinkPatterns returns a starter set of English-language link patterns.
// Callers should extend this list for domain-specific vocabulary.
func DefaultLinkPatterns() []LinkPattern {
	return []LinkPattern{
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)\s+(?:works|worked)\s+(?:at|for)\s+(?P<obj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)`),
			Predicate: "works_at",
		},
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)\s+(?:is|was)\s+(?:the\s+)?(?:CEO|CTO|CFO|COO|founder|co-founder|president|director)\s+(?:of\s+)?(?P<obj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)`),
			Predicate: "leads",
		},
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)\s+(?:lives|lived|is based)\s+in\s+(?P<obj>[A-Z][a-zA-Z]+(?:[\s,]+[A-Z][a-zA-Z]+)*)`),
			Predicate: "lives_in",
		},
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)\s+(?:attended|joined|participated in)\s+(?P<obj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)`),
			Predicate: "attended",
		},
		{
			Re:        regexp.MustCompile(`(?i)(?P<subj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)\s+(?:founded|co-founded|started)\s+(?P<obj>[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*)`),
			Predicate: "founded",
		},
	}
}
