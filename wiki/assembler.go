package wiki

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
	"github.com/tingly-dev/tingly-wiki/storage"
)

// AssembleFormat controls the text format produced by ContextAssembler.
type AssembleFormat string

const (
	// AssembleFormatMarkdown produces a human-readable markdown block (default).
	AssembleFormatMarkdown AssembleFormat = "markdown"

	// AssembleFormatJSON produces a compact JSON array of memory objects.
	AssembleFormatJSON AssembleFormat = "json"
)

// AssembleOptions controls ContextAssembler behaviour.
type AssembleOptions struct {
	// TenantID restricts assembly to a specific namespace (empty = all).
	TenantID string

	// Layers lists the PageTypes to include (nil = preference + procedure + memory).
	Layers []schema.PageType

	// MaxChars is the total character budget for the assembled text.
	// Pages are included in importance order until the budget is exhausted.
	// 0 means no limit.
	MaxChars int

	// Format is the output format (default: AssembleFormatMarkdown).
	Format AssembleFormat

	// Query, when non-empty, ranks pages by relevance via HybridRetriever.
	// When empty, pages are sorted by Importance descending, then LastAccessedAt.
	Query string
}

// AssembleStats describes what was included in an AssembledContext.
type AssembleStats struct {
	PagesIncluded  int
	PagesSkipped   int
	TotalChars     int
	LayerBreakdown map[schema.PageType]int
}

// AssembledContext is the result of a ContextAssembler.Assemble call.
type AssembledContext struct {
	// Text is the ready-to-use output; the caller decides where to inject it.
	Text string

	// Citations are the page paths that contributed to Text (for auditing).
	Citations []string

	Stats AssembleStats
}

// ContextAssembler converts stored memories into a compact, injectable text bundle.
// It is the "last-mile" interface between the memory layer and the agent prompt.
//
// The caller decides *when* to call Assemble, *where* to inject the Text,
// and whether to use prefix-cache optimisations. The assembler only decides
// *what* to include and *how* to format it.
type ContextAssembler interface {
	Assemble(ctx context.Context, opts *AssembleOptions) (*AssembledContext, error)
}

// DefaultAssembler is the built-in ContextAssembler implementation.
// It reads pages from storage sorted by Importance desc, LastAccessedAt desc.
//
// Layer output order: Preferences → Procedures → Memories (and any extras).
type DefaultAssembler struct {
	storage   storage.Storage
	retriever *HybridRetriever // optional; used only when opts.Query != ""
}

// NewDefaultAssembler creates a DefaultAssembler.
// retriever may be nil; Query-mode ranking is silently disabled in that case.
func NewDefaultAssembler(st storage.Storage, retriever *HybridRetriever) *DefaultAssembler {
	return &DefaultAssembler{storage: st, retriever: retriever}
}

// defaultAssembleLayers is used when AssembleOptions.Layers is nil.
var defaultAssembleLayers = []schema.PageType{
	schema.PageTypePreference,
	schema.PageTypeProcedure,
	schema.PageTypeMemory,
}

// sectionHeader maps a PageType to its markdown section heading.
var sectionHeader = map[schema.PageType]string{
	schema.PageTypePreference: "## User Preferences",
	schema.PageTypeProcedure:  "## Active Procedures",
	schema.PageTypeMemory:     "## Recent Memories",
	schema.PageTypeEntity:     "## Known Entities",
	schema.PageTypeConcept:    "## Key Concepts",
}

// assembledSection is an internal work unit for building the output.
type assembledSection struct {
	pt    schema.PageType
	pages []*schema.Page
}

// Assemble builds the context bundle according to opts.
func (a *DefaultAssembler) Assemble(ctx context.Context, opts *AssembleOptions) (*AssembledContext, error) {
	if opts == nil {
		opts = &AssembleOptions{}
	}
	layers := opts.Layers
	if len(layers) == 0 {
		layers = defaultAssembleLayers
	}
	format := opts.Format
	if format == "" {
		format = AssembleFormatMarkdown
	}

	if opts.Query != "" && a.retriever != nil {
		return a.assembleWithQuery(ctx, opts, layers, format)
	}
	return a.assembleByImportance(ctx, opts, layers, format)
}

func (a *DefaultAssembler) assembleByImportance(
	ctx context.Context,
	opts *AssembleOptions,
	layers []schema.PageType,
	format AssembleFormat,
) (*AssembledContext, error) {
	now := time.Now()
	var sections []assembledSection

	for _, pt := range layers {
		ptCopy := pt
		pages, err := a.storage.ListPages(ctx, &storage.ListOptions{Type: &ptCopy})
		if err != nil {
			return nil, fmt.Errorf("assembler: list %s: %w", pt, err)
		}
		var valid []*schema.Page
		for _, p := range pages {
			if opts.TenantID != "" && p.TenantID != opts.TenantID {
				continue
			}
			if p.ExpiresAt != nil && p.ExpiresAt.Before(now) {
				continue
			}
			valid = append(valid, p)
		}
		sort.Slice(valid, func(i, j int) bool {
			if valid[i].Importance != valid[j].Importance {
				return valid[i].Importance > valid[j].Importance
			}
			ti, tj := time.Time{}, time.Time{}
			if valid[i].LastAccessedAt != nil {
				ti = *valid[i].LastAccessedAt
			}
			if valid[j].LastAccessedAt != nil {
				tj = *valid[j].LastAccessedAt
			}
			return ti.After(tj)
		})
		sections = append(sections, assembledSection{pt: pt, pages: valid})
	}

	return a.renderSections(sections, opts.MaxChars, format)
}

func (a *DefaultAssembler) assembleWithQuery(
	ctx context.Context,
	opts *AssembleOptions,
	layers []schema.PageType,
	format AssembleFormat,
) (*AssembledContext, error) {
	scored, err := a.retriever.Recall(ctx, opts.Query, &RecallOptions{
		Types:    layers,
		TenantID: opts.TenantID,
		Limit:    50,
	}, a.storage)
	if err != nil {
		return nil, fmt.Errorf("assembler: recall: %w", err)
	}

	byLayer := make(map[schema.PageType][]*schema.Page)
	for _, s := range scored {
		byLayer[s.Page.Type] = append(byLayer[s.Page.Type], s.Page)
	}
	var sections []assembledSection
	for _, pt := range layers {
		sections = append(sections, assembledSection{pt: pt, pages: byLayer[pt]})
	}

	return a.renderSections(sections, opts.MaxChars, format)
}

// renderSections builds the final AssembledContext from ordered sections.
func (a *DefaultAssembler) renderSections(sections []assembledSection, maxChars int, format AssembleFormat) (*AssembledContext, error) {
	stats := AssembleStats{LayerBreakdown: make(map[schema.PageType]int)}
	var citations []string
	var sb strings.Builder

	for _, sec := range sections {
		if len(sec.pages) == 0 {
			continue
		}
		header := sectionHeader[sec.pt]
		if header == "" {
			header = "## " + string(sec.pt)
		}

		var itemBuf strings.Builder
		count := 0
		for _, p := range sec.pages {
			item := formatPageItem(p, sec.pt)
			needed := sb.Len() + itemBuf.Len() + len(header) + 2 + len(item) + 1
			if maxChars > 0 && needed > maxChars {
				stats.PagesSkipped++
				continue
			}
			itemBuf.WriteString(item)
			itemBuf.WriteByte('\n')
			citations = append(citations, p.Path)
			stats.LayerBreakdown[sec.pt]++
			stats.PagesIncluded++
			count++
		}
		if count > 0 {
			sb.WriteString(header)
			sb.WriteByte('\n')
			sb.WriteString(itemBuf.String())
			sb.WriteByte('\n')
		}
	}

	text := strings.TrimSpace(sb.String())
	if format == AssembleFormatJSON {
		text = sectionsToJSON(sections)
	}
	stats.TotalChars = len(text)

	return &AssembledContext{Text: text, Citations: citations, Stats: stats}, nil
}

// formatPageItem renders a single page as a markdown bullet.
func formatPageItem(p *schema.Page, pt schema.PageType) string {
	var sb strings.Builder
	sb.WriteString("- **")
	sb.WriteString(p.Title)
	sb.WriteString("**")

	facts := activeFacts(p)
	if len(facts) > 0 {
		sb.WriteString(": ")
		for i, f := range facts {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(f.Subject)
			sb.WriteByte(' ')
			sb.WriteString(f.Predicate)
			sb.WriteByte(' ')
			sb.WriteString(f.Object)
		}
	} else if p.Content != "" {
		excerpt := truncateStr(p.Content, 120)
		sb.WriteString(": ")
		sb.WriteString(strings.ReplaceAll(excerpt, "\n", " "))
	}

	if pt == schema.PageTypeMemory && p.UpdatedAt.Year() > 1 {
		sb.WriteString(fmt.Sprintf(" (%s)", p.UpdatedAt.Format("2006-01-02")))
	}
	return sb.String()
}

// activeFacts returns non-invalidated facts.
func activeFacts(p *schema.Page) []schema.MemoryFact {
	var out []schema.MemoryFact
	for _, f := range p.Facts {
		if f.InvalidatedAt == nil {
			out = append(out, f)
		}
	}
	return out
}

// sectionsToJSON produces a simple JSON array (no external deps).
func sectionsToJSON(sections []assembledSection) string {
	var items []string
	for _, sec := range sections {
		for _, p := range sec.pages {
			items = append(items, fmt.Sprintf(
				`{"type":%q,"title":%q,"importance":%g,"content":%q}`,
				p.Type, p.Title, p.Importance,
				truncateStr(strings.ReplaceAll(p.Content, `"`, `\"`), 200),
			))
		}
	}
	return "[" + strings.Join(items, ",") + "]"
}

// truncateStr truncates s to at most n runes, appending an ellipsis when the
// input was shortened. Counting in runes keeps multi-byte characters (e.g.
// CJK) from being sliced mid-character.
func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
