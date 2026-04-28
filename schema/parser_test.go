package schema

import (
	"strings"
	"testing"
)

// TestParserSerialize tests that serialization produces valid markdown with frontmatter
func TestParserSerialize(t *testing.T) {
	parser := NewParser()

	page := &Page{
		Frontmatter: Frontmatter{
			Type:    PageTypeEntity,
			Title:   "Test Page",
			Tags:    []string{"test", "entity"},
			Sources: []string{"source-1", "source-2"},
			Related: []string{"related-1"},
			Extra:   map[string]interface{}{},
		},
		Content: "# Test Page\n\nThis is test content.",
	}

	result, err := parser.Serialize(page)
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Check that it starts with frontmatter delimiter
	if !strings.HasPrefix(result, "---\n") {
		t.Errorf("Serialized page should start with '---\\n', got: %q", result[:10])
	}

	// Check that frontmatter ends with proper delimiter
	if !strings.Contains(result, "\n---\n\n") {
		t.Errorf("Serialized page should have '\\n---\\n\\n' delimiter, got: %q", result)
	}

	// Check for the bug: consecutive "---" without content
	if strings.Contains(result, "------") {
		t.Error("Found bug: consecutive '---' without newline between")
	}

	// Check that content is present
	if !strings.Contains(result, "# Test Page") {
		t.Error("Serialized page should contain content")
	}

	// Verify round-trip: parse it back
	parsed, err := parser.Parse(result)
	if err != nil {
		t.Fatalf("Failed to parse serialized page: %v", err)
	}

	if parsed.Title != page.Title {
		t.Errorf("Expected title %q, got %q", page.Title, parsed.Title)
	}

	if parsed.Type != page.Type {
		t.Errorf("Expected type %q, got %q", page.Type, parsed.Type)
	}

	if len(parsed.Tags) != len(page.Tags) {
		t.Errorf("Expected %d tags, got %d", len(page.Tags), len(parsed.Tags))
	}

	if len(parsed.Sources) != len(page.Sources) {
		t.Errorf("Expected %d sources, got %d", len(page.Sources), len(parsed.Sources))
	}

	if len(parsed.Related) != len(page.Related) {
		t.Errorf("Expected %d related, got %d", len(page.Related), len(parsed.Related))
	}
}

// TestParserSerializeEdgeCases tests edge cases in serialization
func TestParserSerializeEdgeCases(t *testing.T) {
	parser := NewParser()

	t.Run("Empty sources and related", func(t *testing.T) {
		page := &Page{
			Frontmatter: Frontmatter{
				Type:  PageTypeConcept,
				Title: "Test",
				Tags:  []string{"test"},
				Extra: map[string]interface{}{},
			},
			Content: "# Test",
		}

		result, err := parser.Serialize(page)
		if err != nil {
			t.Fatalf("Failed to serialize: %v", err)
		}

		// Should not contain "sources:" or "related:" keys when empty
		// This keeps the frontmatter clean
		parsed, err := parser.Parse(result)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		if parsed.Title != "Test" {
			t.Errorf("Expected title 'Test', got %q", parsed.Title)
		}
	})

	t.Run("Single item lists", func(t *testing.T) {
		page := &Page{
			Frontmatter: Frontmatter{
				Type:    PageTypeEntity,
				Title:   "Single",
				Tags:    []string{"single"},
				Sources: []string{"source-1"},
				Related: []string{"related-1"},
				Extra:   map[string]interface{}{},
			},
			Content: "# Single",
		}

		result, err := parser.Serialize(page)
		if err != nil {
			t.Fatalf("Failed to serialize: %v", err)
		}

		parsed, err := parser.Parse(result)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		if len(parsed.Sources) != 1 {
			t.Errorf("Expected 1 source, got %d", len(parsed.Sources))
		}

		if len(parsed.Related) != 1 {
			t.Errorf("Expected 1 related, got %d", len(parsed.Related))
		}
	})

	t.Run("Content with special characters", func(t *testing.T) {
		content := "# Test\n\nThis has:\n\n- Dashes ---\n- Underscores _\n- Backticks `code`"
		page := &Page{
			Frontmatter: Frontmatter{
				Type:  PageTypeEntity,
				Title: "Special Chars",
				Extra: map[string]interface{}{},
			},
			Content: content,
		}

		result, err := parser.Serialize(page)
		if err != nil {
			t.Fatalf("Failed to serialize: %v", err)
		}

		parsed, err := parser.Parse(result)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		if parsed.Content != content {
			t.Errorf("Content mismatch:\nExpected: %q\nGot: %q", content, parsed.Content)
		}
	})

	t.Run("Multiline content", func(t *testing.T) {
		page := &Page{
			Frontmatter: Frontmatter{
				Type:  PageTypeSource,
				Title: "Multi",
				Extra: map[string]interface{}{},
			},
			Content: `# Long Content

This is a long piece of content
with multiple paragraphs.

## Section

More content here.`,
		}

		result, err := parser.Serialize(page)
		if err != nil {
			t.Fatalf("Failed to serialize: %v", err)
		}

		parsed, err := parser.Parse(result)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		if parsed.Content != page.Content {
			t.Error("Multiline content should be preserved")
		}
	})
}

// TestParserRoundTrip tests that multiple serialize-parse cycles preserve data
func TestParserRoundTrip(t *testing.T) {
	parser := NewParser()

	original := &Page{
		Frontmatter: Frontmatter{
			Type:    PageTypeEntity,
			Title:   "OpenAI",
			Tags:    []string{"entity", "organization"},
			Sources: []string{"source-1", "source-2"},
			Related: []string{"entities/anthropic.md"},
			Extra:   map[string]interface{}{},
		},
		Content: `# OpenAI

OpenAI is an AI research organization.

## Mission

To ensure AGI benefits all of humanity.`,
	}

	// Three round trips
	current := original
	for i := 0; i < 3; i++ {
		serialized, err := parser.Serialize(current)
		if err != nil {
			t.Fatalf("Round trip %d: failed to serialize: %v", i, err)
		}

		parsed, err := parser.Parse(serialized)
		if err != nil {
			t.Fatalf("Round trip %d: failed to parse: %v", i, err)
		}

		if parsed.Title != original.Title {
			t.Errorf("Round trip %d: title changed from %q to %q", i, original.Title, parsed.Title)
		}

		if parsed.Type != original.Type {
			t.Errorf("Round trip %d: type changed from %q to %q", i, original.Type, parsed.Type)
		}

		if len(parsed.Tags) != len(original.Tags) {
			t.Errorf("Round trip %d: tags count changed from %d to %d", i, len(original.Tags), len(parsed.Tags))
		}

		if parsed.Content != original.Content {
			t.Errorf("Round trip %d: content changed", i)
		}

		current = parsed
	}
}

// TestParseNoFrontmatter tests parsing pages without frontmatter
func TestParseNoFrontmatter(t *testing.T) {
	parser := NewParser()

	content := "# Just Content\n\nNo frontmatter here."
	page, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if page.Content != content {
		t.Errorf("Expected content %q, got %q", content, page.Content)
	}

	if page.Type != "" {
		t.Errorf("Expected empty type, got %q", page.Type)
	}
}

// TestParseInvalidFrontmatter tests handling of invalid frontmatter
func TestParseInvalidFrontmatter(t *testing.T) {
	parser := NewParser()

	t.Run("Unclosed delimiter", func(t *testing.T) {
		content := `---
title: Test
# Content without closing delimiter`

		page, err := parser.Parse(content)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		// Should treat entire thing as content when frontmatter is unclosed
		if page.Title != "" {
			t.Errorf("Expected empty title, got %q", page.Title)
		}
	})

	t.Run("Empty frontmatter", func(t *testing.T) {
		content := `---
---
# Content`

		page, err := parser.Parse(content)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		// Should parse successfully with empty frontmatter
		if page.Content != "# Content" {
			t.Errorf("Expected content '# Content', got %q", page.Content)
		}
	})
}
