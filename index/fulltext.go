package index

import (
	"context"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// FullTextIndex implements in-memory full-text search
type FullTextIndex struct {
	mu     sync.RWMutex
	index  map[string]*IndexedPage // path -> IndexedPage
	tokens map[string][]string      // token -> paths
}

// IndexedPage represents an indexed page
type IndexedPage struct {
	Path    string
	Content string
	Tokens  []string
}

// NewFullTextIndex creates a new full-text index
func NewFullTextIndex() *FullTextIndex {
	return &FullTextIndex{
		index:  make(map[string]*IndexedPage),
		tokens: make(map[string][]string),
	}
}

// Index adds a page to the index
func (f *FullTextIndex) Index(ctx context.Context, page *schema.Page) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Tokenize content
	tokens := tokenize(page.Content)
	tokens = append(tokens, tokenize(page.Title)...)
	for _, tag := range page.Tags {
		tokens = append(tokens, tokenize(tag)...)
	}

	// Store indexed page
	f.index[page.Path] = &IndexedPage{
		Path:    page.Path,
		Content: page.Content,
		Tokens:  tokens,
	}

	// Update token index
	tokenSet := make(map[string]bool)
	for _, token := range tokens {
		if !tokenSet[token] {
			f.tokens[token] = append(f.tokens[token], page.Path)
			tokenSet[token] = true
		}
	}

	return nil
}

// Search finds relevant pages
func (f *FullTextIndex) Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if opts == nil {
		opts = &SearchOptions{}
	}

	// Tokenize query
	queryTokens := tokenize(query)

	// Score each page
	scores := make(map[string]float64)
	for _, token := range queryTokens {
		// Find pages containing this token
		paths, ok := f.tokens[token]
		if !ok {
			continue
		}

		// Boost score for each page containing this token
		for _, path := range paths {
			scores[path] += 1.0
		}
	}

	// Convert to results
	var results []SearchResultItem
	for path, score := range scores {
		// Normalize score by query tokens
		score = score / float64(len(queryTokens))

		page, ok := f.index[path]
		if !ok {
			continue
		}

		// Apply filters
		if opts.Type != nil && page.Path != "" { // Page type check would need page loading
			// Skip if type doesn't match
		}

		if opts.MinScore > 0 && score < opts.MinScore {
			continue
		}

		// Find excerpt
		excerpt := findExcerpt(page.Content, queryTokens)

		results = append(results, SearchResultItem{
			Page: &schema.Page{Path: path}, // Minimal page, would load full page in real impl
			Score: score,
			Excerpt: excerpt,
		})
	}

	// Sort by score
	sortResults(results)

	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return &SearchResult{
		Results: results,
		Total:   len(scores),
	}, nil
}

// Remove removes a page from the index
func (f *FullTextIndex) Remove(ctx context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Get indexed page
	page, ok := f.index[path]
	if !ok {
		return nil
	}

	// Remove from token index
	for _, token := range page.Tokens {
		paths := f.tokens[token]
		for i, p := range paths {
			if p == path {
				f.tokens[token] = append(paths[:i], paths[i+1:]...)
				break
			}
		}
	}

	// Remove from page index
	delete(f.index, path)

	return nil
}

// Close closes the index
func (f *FullTextIndex) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.index = make(map[string]*IndexedPage)
	f.tokens = make(map[string][]string)
	return nil
}

// tokenize splits text into tokens
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Split on non-alphanumeric
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	return tokens
}

// findExcerpt finds a relevant excerpt from content
func findExcerpt(content string, queryTokens []string) string {
	// Simple implementation: find first query token in content
	for _, token := range queryTokens {
		idx := strings.Index(content, token)
		if idx != -1 {
			start := idx - 50
			if start < 0 {
				start = 0
			}
			end := idx + 50
			if end > len(content) {
				end = len(content)
			}
			excerpt := content[start:end]
			if start > 0 {
				excerpt = "..." + excerpt
			}
			if end < len(content) {
				excerpt = excerpt + "..."
			}
			return excerpt
		}
	}
	return ""
}

// sortResults sorts results by score (descending)
func sortResults(results []SearchResultItem) {
	// Simple bubble sort
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].Score < results[j+1].Score {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
