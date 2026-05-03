package index

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tingly-dev/tingly-wiki/schema"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// FullTextIndex implements in-memory full-text search with BM25 scoring.
type FullTextIndex struct {
	mu        sync.RWMutex
	tokenizer Tokenizer
	index     map[string]*IndexedPage // path -> IndexedPage
	tokens    map[string][]string     // token -> paths

	// BM25 state
	docFreq    map[string]int // token -> number of docs containing it
	docLengths map[string]int // path -> token count
	totalDocs  int
	totalLen   int // sum of all doc lengths (for avgDocLen)
}

// IndexedPage represents an indexed page with metadata needed for filtering
type IndexedPage struct {
	Path       string
	Content    string
	Tokens     []string
	Type       schema.PageType
	TenantID   string
	Importance float64
	MemoryTier schema.MemoryTier
	ExpiresAt  *time.Time
}

// NewFullTextIndex creates a new full-text index with the default tokenizer
// (CJK-aware, see MixedScriptTokenizer).
func NewFullTextIndex() *FullTextIndex {
	return NewFullTextIndexWithTokenizer(DefaultTokenizer())
}

// NewFullTextIndexWithTokenizer creates a full-text index that uses the
// provided Tokenizer for both indexing and querying. Pass a custom
// implementation to support dictionary-based segmentation, language-specific
// stemming, or other strategies. A nil tokenizer falls back to the default.
func NewFullTextIndexWithTokenizer(t Tokenizer) *FullTextIndex {
	if t == nil {
		t = DefaultTokenizer()
	}
	return &FullTextIndex{
		tokenizer:  t,
		index:      make(map[string]*IndexedPage),
		tokens:     make(map[string][]string),
		docFreq:    make(map[string]int),
		docLengths: make(map[string]int),
	}
}

// Index adds a page to the index
func (f *FullTextIndex) Index(ctx context.Context, page *schema.Page) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Remove existing entry first (handles upsert)
	if _, exists := f.index[page.Path]; exists {
		f.removeLocked(page.Path)
	}

	// Tokenize content
	tokens := f.tokenizer.Tokenize(page.Content)
	tokens = append(tokens, f.tokenizer.Tokenize(page.Title)...)
	for _, tag := range page.Tags {
		tokens = append(tokens, f.tokenizer.Tokenize(tag)...)
	}

	// Store indexed page with metadata needed for filtering
	f.index[page.Path] = &IndexedPage{
		Path:       page.Path,
		Content:    page.Content,
		Tokens:     tokens,
		Type:       page.Type,
		TenantID:   page.TenantID,
		Importance: page.Importance,
		MemoryTier: page.MemoryTier,
		ExpiresAt:  page.ExpiresAt,
	}

	// Update token inverted index (deduplicated for posting list)
	tokenSet := make(map[string]bool)
	for _, token := range tokens {
		if !tokenSet[token] {
			f.tokens[token] = append(f.tokens[token], page.Path)
			f.docFreq[token]++
			tokenSet[token] = true
		}
	}

	// Update BM25 length stats
	f.docLengths[page.Path] = len(tokens)
	f.totalDocs++
	f.totalLen += len(tokens)

	return nil
}

// Search finds relevant pages using BM25 scoring.
func (f *FullTextIndex) Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if opts == nil {
		opts = &SearchOptions{}
	}

	queryTokens := f.tokenizer.Tokenize(query)
	if len(queryTokens) == 0 {
		return &SearchResult{}, nil
	}

	avgDocLen := 1.0
	if f.totalDocs > 0 {
		avgDocLen = float64(f.totalLen) / float64(f.totalDocs)
	}

	// Collect candidate paths from inverted index
	candidates := make(map[string]struct{})
	for _, token := range queryTokens {
		for _, path := range f.tokens[token] {
			candidates[path] = struct{}{}
		}
	}

	// Score each candidate with BM25
	scores := make(map[string]float64, len(candidates))
	for path := range candidates {
		indexed, ok := f.index[path]
		if !ok {
			continue
		}
		docLen := float64(f.docLengths[path])

		// Count term frequency per token in this doc
		tf := termFreq(indexed.Tokens)

		var score float64
		for _, qt := range queryTokens {
			tfVal := float64(tf[qt])
			if tfVal == 0 {
				continue
			}
			df := float64(f.docFreq[qt])
			N := float64(f.totalDocs)
			// IDF (Robertson-Sparck Jones, smoothed)
			idf := math.Log((N-df+0.5)/(df+0.5) + 1)
			// BM25 tf component
			tfScore := (tfVal * (bm25K1 + 1)) / (tfVal + bm25K1*(1-bm25B+bm25B*docLen/avgDocLen))
			score += idf * tfScore
		}
		scores[path] = score
	}

	// Build results with filters
	var results []SearchResultItem
	now := time.Now()
	for path, score := range scores {
		indexed, ok := f.index[path]
		if !ok {
			continue
		}

		if opts.MinScore > 0 && score < opts.MinScore {
			continue
		}
		if opts.Type != nil && indexed.Type != *opts.Type {
			continue
		}
		if opts.TenantID != "" && indexed.TenantID != opts.TenantID {
			continue
		}
		if opts.MinImportance > 0 && indexed.Importance < opts.MinImportance {
			continue
		}
		if opts.MemoryTier != "" && indexed.MemoryTier != opts.MemoryTier {
			continue
		}
		if opts.ExcludeExpired && indexed.ExpiresAt != nil && indexed.ExpiresAt.Before(now) {
			continue
		}

		excerpt := findExcerpt(indexed.Content, queryTokens)
		results = append(results, SearchResultItem{
			Page:    &schema.Page{Path: path},
			Score:   score,
			Excerpt: excerpt,
		})
	}

	sortResults(results)

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
	f.removeLocked(path)
	return nil
}

// removeLocked removes a page; caller must hold write lock.
func (f *FullTextIndex) removeLocked(path string) {
	page, ok := f.index[path]
	if !ok {
		return
	}

	// Update docFreq: only decrement once per unique token
	seen := make(map[string]bool)
	for _, token := range page.Tokens {
		if !seen[token] {
			f.docFreq[token]--
			if f.docFreq[token] <= 0 {
				delete(f.docFreq, token)
			}
			seen[token] = true
		}
	}

	// Remove from inverted index
	for _, token := range page.Tokens {
		paths := f.tokens[token]
		for i, p := range paths {
			if p == path {
				f.tokens[token] = append(paths[:i], paths[i+1:]...)
				break
			}
		}
	}

	// Update length stats
	f.totalLen -= f.docLengths[path]
	f.totalDocs--
	delete(f.docLengths, path)
	delete(f.index, path)
}

// Close resets the index
func (f *FullTextIndex) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.index = make(map[string]*IndexedPage)
	f.tokens = make(map[string][]string)
	f.docFreq = make(map[string]int)
	f.docLengths = make(map[string]int)
	f.totalDocs = 0
	f.totalLen = 0
	return nil
}

// termFreq counts occurrences of each token in a token list.
func termFreq(tokens []string) map[string]int {
	tf := make(map[string]int, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}
	return tf
}

// findExcerpt finds a relevant excerpt from content. Window boundaries are
// snapped to UTF-8 rune starts so that multi-byte characters (e.g. CJK) are
// never sliced mid-character.
func findExcerpt(content string, queryTokens []string) string {
	for _, token := range queryTokens {
		idx := strings.Index(content, token)
		if idx == -1 {
			continue
		}
		start := alignRuneStart(content, idx-50)
		end := alignRuneStart(content, idx+50)
		excerpt := content[start:end]
		if start > 0 {
			excerpt = "..." + excerpt
		}
		if end < len(content) {
			excerpt = excerpt + "..."
		}
		return excerpt
	}
	return ""
}

// alignRuneStart clamps i to [0, len(s)] and walks it forward (if needed) so
// it lands on a UTF-8 rune boundary.
func alignRuneStart(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}

// sortResults sorts results by score (descending)
func sortResults(results []SearchResultItem) {
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].Score < results[j+1].Score {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
