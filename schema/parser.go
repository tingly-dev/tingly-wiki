package schema

import (
	"strings"
	"time"
)

// Parser handles parsing and serializing pages with frontmatter
type Parser struct{}

// NewParser creates a new parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a page from markdown content with frontmatter
func (p *Parser) Parse(content string) (*Page, error) {
	lines := strings.Split(content, "\n")

	// Check for frontmatter delimiter
	if len(lines) < 2 || lines[0] != "---" {
		// No frontmatter, treat entire content as body
		return &Page{
			Content:   content,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Find end of frontmatter
	fmEnd := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			fmEnd = i
			break
		}
	}

	if fmEnd == -1 {
		// Unclosed frontmatter, treat entire content as body
		return &Page{
			Content:   content,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Parse frontmatter
	fmContent := strings.Join(lines[1:fmEnd], "\n")
	frontmatter, err := parseFrontmatter(fmContent)
	if err != nil {
		return nil, err
	}

	// Parse body content
	bodyContent := strings.Join(lines[fmEnd+1:], "\n")

	return &Page{
		Frontmatter: *frontmatter,
		Content:     strings.TrimSpace(bodyContent),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// Serialize serializes a page to markdown with frontmatter
func (p *Parser) Serialize(page *Page) (string, error) {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	fmYAML, err := serializeFrontmatter(&page.Frontmatter)
	if err != nil {
		return "", err
	}
	sb.WriteString(fmYAML)
	sb.WriteString("\n---\n\n")  // Add newline before closing delimiter

	// Write content
	sb.WriteString(page.Content)

	return sb.String(), nil
}
