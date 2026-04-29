package wiki

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-wiki/config"
	"github.com/tingly-dev/tingly-wiki/index"
	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// canonicalText returns a compact, LLM-embeddable representation of a page.
// It includes the title, active facts, and the first 512 bytes of content.
func canonicalText(page *schema.Page) string {
	var sb strings.Builder
	sb.WriteString(page.Title)
	for _, f := range page.Facts {
		if f.InvalidatedAt == nil {
			sb.WriteString(". ")
			sb.WriteString(f.Subject)
			sb.WriteString(" ")
			sb.WriteString(f.Predicate)
			sb.WriteString(" ")
			sb.WriteString(f.Object)
		}
	}
	if page.Content != "" {
		sb.WriteString(". ")
		content := page.Content
		if len(content) > 512 {
			content = content[:512]
		}
		sb.WriteString(content)
	}
	return sb.String()
}

// mergeMemoryFacts combines facts from an existing page with newly extracted facts.
// Where a new fact shares (Subject, Predicate) with an existing valid fact but has a
// different Object, the existing fact is marked InvalidatedAt = now (bi-temporal model).
func mergeMemoryFacts(existing *schema.Page, newFacts []schema.MemoryFact, now time.Time) []schema.MemoryFact {
	if existing == nil || len(existing.Facts) == 0 {
		return newFacts
	}

	merged := make([]schema.MemoryFact, 0, len(existing.Facts)+len(newFacts))

	// Build a lookup of incoming (subject, predicate) → object
	type key struct{ s, p string }
	incoming := make(map[key]string, len(newFacts))
	for _, f := range newFacts {
		incoming[key{f.Subject, f.Predicate}] = f.Object
	}

	// Carry forward old facts; invalidate those that conflict with new ones
	for _, f := range existing.Facts {
		if f.InvalidatedAt != nil {
			merged = append(merged, f) // already invalidated, keep as history
			continue
		}
		k := key{f.Subject, f.Predicate}
		if newObj, ok := incoming[k]; ok && newObj != f.Object {
			// Conflict: mark this fact as superseded
			t := now
			f.InvalidatedAt = &t
		}
		merged = append(merged, f)
	}

	// Append the new facts (they are always valid at this point)
	merged = append(merged, newFacts...)
	return merged
}

// withValidFacts returns a shallow copy of page with only non-invalidated facts.
func withValidFacts(page *schema.Page) *schema.Page {
	if len(page.Facts) == 0 {
		return page
	}
	valid := make([]schema.MemoryFact, 0, len(page.Facts))
	for _, f := range page.Facts {
		if f.InvalidatedAt == nil {
			valid = append(valid, f)
		}
	}
	// Shallow copy to avoid mutating the cached page
	copy := *page
	copy.Facts = valid
	return &copy
}

// MemoryWikiImpl wraps WikiImpl and satisfies the MemoryWiki interface.
// All existing Wiki methods are delegated to the embedded *WikiImpl unchanged.
type MemoryWikiImpl struct {
	*WikiImpl
	scorer    *ImportanceScorer
	vector    index.VectorIndex  // optional; nil → keyword-only recall
	retriever *HybridRetriever
}

// NewMemoryWiki creates a MemoryWiki-capable wiki instance.
// The cfg argument is identical to what wiki.New() accepts, with the optional
// cfg.VectorIndex field providing semantic retrieval capability.
func NewMemoryWiki(cfg *config.Config) (*MemoryWikiImpl, error) {
	base, err := New(cfg)
	if err != nil {
		return nil, err
	}

	scorer := DefaultImportanceScorer()

	var vec index.VectorIndex
	if cfg.VectorIndex != nil {
		if vi, ok := cfg.VectorIndex.(index.VectorIndex); ok {
			vec = vi
		}
	}

	retriever := NewHybridRetriever(base.index, vec, scorer, base.llm)

	return &MemoryWikiImpl{
		WikiImpl:  base,
		scorer:    scorer,
		vector:    vec,
		retriever: retriever,
	}, nil
}

// ---- MemoryWiki implementation ----

// StoreMemory writes a memory page, creating or updating by title.
// It automatically:
//   - rates importance via LLM when the caller supplies 0
//   - extracts atomic facts from the content
//   - invalidates previously valid facts that conflict with new ones
//   - indexes the page in both the keyword and (if configured) vector index
func (m *MemoryWikiImpl) StoreMemory(ctx context.Context, req *StoreMemoryRequest) (*StoreMemoryResult, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("StoreMemory: Title is required")
	}

	path := m.pathForType(req.Type, req.Title)
	now := time.Now()

	// ── Importance ────────────────────────────────────────────────────────────
	importance := req.Importance
	if importance == 0 {
		if rated, err := m.llm.RateImportance(ctx, req.Content); err == nil {
			importance = rated
		} else {
			importance = 0.5
		}
	}

	// ── Fact extraction ───────────────────────────────────────────────────────
	newFacts, _ := m.llm.ExtractMemoryFacts(ctx, req.Content, req.Type) // best-effort

	// ── TTL ───────────────────────────────────────────────────────────────────
	var expiresAt *time.Time
	if req.TTL != nil {
		t := now.Add(*req.TTL)
		expiresAt = &t
	}

	// ── Existing page (update vs create) ─────────────────────────────────────
	existing, _ := m.storage.ReadPage(ctx, path)
	created := existing == nil

	// Merge facts: invalidate old facts that conflict with incoming ones
	mergedFacts := mergeMemoryFacts(existing, newFacts, now)

	page := &schema.Page{
		Path: path,
		Frontmatter: schema.Frontmatter{
			Type:       req.Type,
			Title:      req.Title,
			Tags:       req.Tags,
			Importance: importance,
			ExpiresAt:  expiresAt,
			MemoryTier: schema.MemoryTierHot,
			TenantID:   req.TenantID,
			AgentID:    req.AgentID,
			Facts:      mergedFacts,
		},
		Content:   req.Content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if !created {
		page.CreatedAt = existing.CreatedAt
		page.AccessCount = existing.AccessCount
		page.LastAccessedAt = existing.LastAccessedAt
	}

	// ── Persist ───────────────────────────────────────────────────────────────
	if err := m.storage.WritePage(ctx, page); err != nil {
		return nil, fmt.Errorf("StoreMemory: write failed: %w", err)
	}
	if err := m.index.Index(ctx, page); err != nil {
		return nil, fmt.Errorf("StoreMemory: index failed: %w", err)
	}

	// ── Vector index (best-effort) ────────────────────────────────────────────
	if m.vector != nil {
		if vec, err := m.llm.Embed(ctx, canonicalText(page)); err == nil && len(vec) > 0 {
			_ = m.vector.IndexVector(ctx, path, vec, &index.VectorMeta{
				Type:      page.Type,
				TenantID:  page.TenantID,
				ExpiresAt: page.ExpiresAt,
			})
		}
	}

	return &StoreMemoryResult{Path: path, Created: created}, nil
}

// RecallMemory retrieves memory pages matching the query using the HybridRetriever.
// Each hit increments the page's AccessCount and updates LastAccessedAt.
// By default, invalidated facts are filtered from the returned pages; set
// opts.IncludeInvalidated = true to include historical facts.
func (m *MemoryWikiImpl) RecallMemory(ctx context.Context, query string, opts *RecallOptions) (*RecallResult, error) {
	if opts == nil {
		opts = &RecallOptions{}
	}

	// Apply caller-supplied strategy overrides to the retriever
	if len(opts.Strategies) > 0 {
		for pt, s := range opts.Strategies {
			m.retriever.strategies[pt] = s
		}
	}

	scored, err := m.retriever.Recall(ctx, query, opts, m.storage)
	if err != nil {
		return nil, fmt.Errorf("RecallMemory: %w", err)
	}

	now := time.Now()
	pages := make([]*schema.Page, 0, len(scored))
	for _, s := range scored {
		page := s.Page

		// Filter invalidated facts unless the caller explicitly wants history
		if !opts.IncludeInvalidated {
			page = withValidFacts(page)
		}
		pages = append(pages, page)

		// Write-through: update access tracking (best-effort)
		page.AccessCount++
		page.LastAccessedAt = &now
		page.MemoryTier = m.scorer.Tier(page)
		_ = m.storage.WritePage(ctx, page)
	}

	return &RecallResult{Pages: pages, Total: len(scored)}, nil
}

// AppendAuditLog appends an entry to the date-scoped audit log file.
// Entries are never overwritten; the file grows as new entries are added.
func (m *MemoryWikiImpl) AppendAuditLog(ctx context.Context, entry *AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	yearMonth := entry.Timestamp.UTC().Format("2006-01")
	path := m.layout.GetAuditLogPath(yearMonth)

	// Read existing content (create if absent)
	var existing string
	if page, err := m.storage.ReadPage(ctx, path); err == nil {
		existing = page.Content
	}

	line := formatAuditEntry(entry)
	newContent := existing
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += line

	now := time.Now()
	page := &schema.Page{
		Path: path,
		Frontmatter: schema.Frontmatter{
			Type:     schema.PageTypeAuditLog,
			Title:    "Audit Log " + yearMonth,
			TenantID: entry.TenantID,
		},
		Content:   newContent,
		UpdatedAt: now,
	}
	if existing == "" {
		page.CreatedAt = now
	}

	if err := m.storage.WritePage(ctx, page); err != nil {
		return fmt.Errorf("AppendAuditLog: write failed: %w", err)
	}
	return nil
}

// SetImportance updates the importance score of an existing page.
func (m *MemoryWikiImpl) SetImportance(ctx context.Context, path string, score float64) error {
	if score < 0 || score > 1 {
		return fmt.Errorf("SetImportance: score must be in [0, 1], got %f", score)
	}
	page, err := m.storage.ReadPage(ctx, path)
	if err != nil {
		return fmt.Errorf("SetImportance: %w", err)
	}
	page.Importance = score
	page.MemoryTier = m.scorer.Tier(page)
	page.UpdatedAt = time.Now()
	if err := m.storage.WritePage(ctx, page); err != nil {
		return fmt.Errorf("SetImportance: write failed: %w", err)
	}
	return m.index.Index(ctx, page)
}

// SetTTL sets or clears the expiry time of an existing page.
// Pass nil to remove the expiry (page never expires).
func (m *MemoryWikiImpl) SetTTL(ctx context.Context, path string, expiresAt *time.Time) error {
	page, err := m.storage.ReadPage(ctx, path)
	if err != nil {
		return fmt.Errorf("SetTTL: %w", err)
	}
	page.ExpiresAt = expiresAt
	page.UpdatedAt = time.Now()
	if err := m.storage.WritePage(ctx, page); err != nil {
		return fmt.Errorf("SetTTL: write failed: %w", err)
	}
	return m.index.Index(ctx, page)
}

// RunGC deletes expired pages and recalculates MemoryTier for all pages.
func (m *MemoryWikiImpl) RunGC(ctx context.Context) (*GCResult, error) {
	pages, err := m.storage.ListPages(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("RunGC: list failed: %w", err)
	}

	result := &GCResult{}
	now := time.Now()

	for _, page := range pages {
		// Delete expired pages
		if page.ExpiresAt != nil && page.ExpiresAt.Before(now) {
			if err := m.storage.DeletePage(ctx, page.Path); err == nil {
				_ = m.index.Remove(ctx, page.Path)
				result.DeletedCount++
				result.DeletedPaths = append(result.DeletedPaths, page.Path)
			}
			continue
		}

		// Recalculate MemoryTier and persist if it changed
		newTier := m.scorer.Tier(page)
		if newTier != page.MemoryTier {
			page.MemoryTier = newTier
			page.UpdatedAt = now
			if err := m.storage.WritePage(ctx, page); err == nil {
				_ = m.index.Index(ctx, page)
				result.DemotedCount++
			}
		}
	}

	return result, nil
}

// ConsolidateMemories uses LLM summarisation to merge semantically similar pages.
// Currently uses title/tag overlap as a similarity signal (no vector search required).
func (m *MemoryWikiImpl) ConsolidateMemories(ctx context.Context, opts *ConsolidateOptions) (*ConsolidateStats, error) {
	if opts == nil {
		opts = &ConsolidateOptions{}
	}

	targetTypes := opts.Types
	if len(targetTypes) == 0 {
		targetTypes = []schema.PageType{schema.PageTypeMemory, schema.PageTypePreference}
	}

	var candidates []*schema.Page
	for _, pt := range targetTypes {
		ptCopy := pt
		listOpts := &storage.ListOptions{Type: &ptCopy}
		if opts.TenantID != "" {
			listOpts.Prefix = ""
		}
		pages, err := m.storage.ListPages(ctx, listOpts)
		if err != nil {
			return nil, fmt.Errorf("ConsolidateMemories: list failed: %w", err)
		}
		for _, p := range pages {
			if opts.TenantID == "" || p.TenantID == opts.TenantID {
				candidates = append(candidates, p)
			}
		}
	}

	groups := groupBySimilarity(candidates)
	stats := &ConsolidateStats{DryRun: opts.DryRun}

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		stats.MergedGroups++

		if opts.DryRun {
			stats.PagesAbsorbed += len(group) - 1
			continue
		}

		// Summarise group using LLM
		summary, err := m.llm.Summarize(ctx, pagesToText(group))
		if err != nil {
			continue
		}

		// Keep the page with highest importance as the merge target
		target := highestImportance(group)
		target.Content = summary
		target.UpdatedAt = time.Now()
		// Merge tags from all absorbed pages
		target.Tags = mergedTags(group)

		if err := m.storage.WritePage(ctx, target); err != nil {
			continue
		}
		_ = m.index.Index(ctx, target)

		// Delete absorbed pages
		for _, p := range group {
			if p.Path == target.Path {
				continue
			}
			_ = m.storage.DeletePage(ctx, p.Path)
			_ = m.index.Remove(ctx, p.Path)
			stats.PagesAbsorbed++
		}
	}

	return stats, nil
}

// ---- helpers ----

func (m *MemoryWikiImpl) pathForType(pt schema.PageType, title string) string {
	switch pt {
	case schema.PageTypePreference:
		return m.layout.GetPreferencePath(title)
	case schema.PageTypeAuditLog:
		return m.layout.GetAuditLogPath(time.Now().UTC().Format("2006-01"))
	default:
		return m.layout.GetMemoryPath(title)
	}
}

func formatAuditEntry(e *AuditEntry) string {
	ts := e.Timestamp.UTC().Format(time.RFC3339)
	line := fmt.Sprintf("- `%s` **%s** agent=%s", ts, e.Action, e.AgentID)
	if e.TargetPath != "" {
		line += fmt.Sprintf(" target=%s", e.TargetPath)
	}
	for k, v := range e.Metadata {
		line += fmt.Sprintf(" %s=%s", k, v)
	}
	return line
}

func containsType(types []schema.PageType, t schema.PageType) bool {
	for _, pt := range types {
		if pt == t {
			return true
		}
	}
	return false
}

// groupBySimilarity clusters pages that share ≥2 tags or have very similar titles.
func groupBySimilarity(pages []*schema.Page) [][]*schema.Page {
	used := make([]bool, len(pages))
	var groups [][]*schema.Page

	for i, a := range pages {
		if used[i] {
			continue
		}
		group := []*schema.Page{a}
		used[i] = true
		for j, b := range pages {
			if used[j] || i == j {
				continue
			}
			if tagOverlap(a.Tags, b.Tags) >= 2 || titleSimilar(a.Title, b.Title) {
				group = append(group, b)
				used[j] = true
			}
		}
		groups = append(groups, group)
	}
	return groups
}

func tagOverlap(a, b []string) int {
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[strings.ToLower(t)] = true
	}
	count := 0
	for _, t := range b {
		if set[strings.ToLower(t)] {
			count++
		}
	}
	return count
}

func titleSimilar(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return true
	}
	// One is a prefix of the other (handles "language" vs "language preference")
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func highestImportance(pages []*schema.Page) *schema.Page {
	best := pages[0]
	for _, p := range pages[1:] {
		if p.Importance > best.Importance {
			best = p
		}
	}
	return best
}

func pagesToText(pages []*schema.Page) string {
	var parts []string
	for _, p := range pages {
		parts = append(parts, fmt.Sprintf("# %s\n%s", p.Title, p.Content))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func mergedTags(pages []*schema.Page) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range pages {
		for _, t := range p.Tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}
