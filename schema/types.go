package schema

import "time"

// PageType is the type of page
type PageType string

const (
	PageTypeEntity    PageType = "entity"
	PageTypeConcept   PageType = "concept"
	PageTypeSource    PageType = "source"
	PageTypeSynthesis PageType = "synthesis"

	// Memory-system page types
	PageTypePreference PageType = "preference" // persistent user/agent preferences
	PageTypeMemory     PageType = "memory"     // cross-session working memory
	PageTypeAuditLog   PageType = "audit_log"  // append-only agent operation log
	PageTypeProcedure  PageType = "procedure"  // reusable workflow / skill (how to do X)
)

// MemoryTier classifies how "hot" a memory is for retrieval prioritization
type MemoryTier string

const (
	MemoryTierHot  MemoryTier = "hot"  // accessed within 7 days, high importance
	MemoryTierWarm MemoryTier = "warm" // 7–30 days or moderate importance
	MemoryTierCold MemoryTier = "cold" // >30 days or low importance, archived
)

// Page represents a wiki page
type Page struct {
	// Path is the relative path within the wiki (e.g., "entities/openai.md")
	Path string `json:"path"`

	// Frontmatter contains metadata
	Frontmatter `yaml:",inline"`

	// Content is the markdown content (without frontmatter)
	Content string `json:"content"`

	// Backlinks are pages that link to this page
	Backlinks []string `json:"backlinks,omitempty"`

	// CreatedAt is when the page was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the page was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// Frontmatter contains page metadata
type Frontmatter struct {
	// Type is the page type
	Type PageType `json:"type" yaml:"type"`

	// Title is the page title
	Title string `json:"title" yaml:"title"`

	// Tags are page tags
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Related are related page paths
	Related []string `json:"related,omitempty" yaml:"related,omitempty"`

	// Sources are source document IDs (for derived pages)
	Sources []string `json:"sources,omitempty" yaml:"sources,omitempty"`

	// Memory lifecycle fields

	// Importance is a 0.0–1.0 score; higher = more critical to retain.
	// 0 means "use system default (0.5)".
	Importance float64 `json:"importance,omitempty" yaml:"importance,omitempty"`

	// ExpiresAt is the TTL deadline; nil means never expires.
	ExpiresAt *time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`

	// AccessCount tracks how many times this page was recalled.
	AccessCount int `json:"access_count,omitempty" yaml:"access_count,omitempty"`

	// LastAccessedAt is when the page was last recalled.
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty" yaml:"last_accessed_at,omitempty"`

	// MemoryTier classifies retrieval priority (hot/warm/cold).
	MemoryTier MemoryTier `json:"memory_tier,omitempty" yaml:"memory_tier,omitempty"`

	// Provenance fields

	// TenantID scopes this page to a specific agent or user namespace.
	TenantID string `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`

	// AgentID identifies which agent wrote this page.
	AgentID string `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`

	// Facts are atomic (subject, predicate, object) triples extracted on write.
	// Current facts have InvalidatedAt == nil; superseded facts carry a timestamp.
	Facts []MemoryFact `json:"facts,omitempty" yaml:"facts,omitempty"`

	// Custom fields
	Extra map[string]interface{} `json:"-" yaml:"-"`
}

// MemoryFact is an atomic fact extracted from memory content at write time.
type MemoryFact struct {
	// Subject is what the fact is about (e.g. "user", entity name)
	Subject string `json:"subject" yaml:"subject"`

	// Predicate is the relationship type (e.g. "prefers", "lives_in", "uses")
	Predicate string `json:"predicate" yaml:"predicate"`

	// Object is the fact's value (e.g. "dark mode", "San Francisco")
	Object string `json:"object" yaml:"object"`

	// Confidence is the extraction confidence in [0, 1]
	Confidence float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`

	// EventTime is when the fact occurred (optional; distinct from ingestion time)
	EventTime *time.Time `json:"event_time,omitempty" yaml:"event_time,omitempty"`

	// InvalidatedAt is when this fact was superseded by a newer conflicting fact.
	// nil means the fact is currently valid.
	InvalidatedAt *time.Time `json:"invalidated_at,omitempty" yaml:"invalidated_at,omitempty"`
}

// SourceType is the type of source
type SourceType string

const (
	SourceTypeFile SourceType = "file"
	SourceTypeURL  SourceType = "url"
	SourceTypeText SourceType = "text"
)

// Source represents a raw source document
type Source struct {
	// ID is a unique identifier
	ID string `json:"id"`

	// Type is the source type
	Type SourceType `json:"type"`

	// Location is where to find the source
	Location string `json:"location"`

	// Content is the extracted text content
	Content string `json:"content,omitempty"`

	// Metadata about the source
	Metadata map[string]string `json:"metadata,omitempty"`

	// IngestedAt is when this was added to the wiki
	IngestedAt time.Time `json:"ingested_at,omitempty"`
}

// ExtractedInfo is structured information extracted by LLM
type ExtractedInfo struct {
	// Summary of the source
	Summary string `json:"summary"`

	// Entities mentioned (people, organizations, products)
	Entities []Entity `json:"entities"`

	// Concepts discussed
	Concepts []Concept `json:"concepts"`

	// Relationships discovered
	Relationships []Relationship `json:"relationships"`

	// Key points extracted
	KeyPoints []string `json:"key_points"`
}

// Entity represents a named entity
type Entity struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// Concept represents a concept or idea
type Concept struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Related     []string `json:"related,omitempty"`
}

// Relationship represents a connection between entities/concepts
type Relationship struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Type    string `json:"type"`
	Context string `json:"context,omitempty"`
}

// QueryAnswer is the result of a query
type QueryAnswer struct {
	// Answer is the synthesized answer
	Answer string `json:"answer"`

	// Citations are references to source pages
	Citations []Citation `json:"citations"`

	// SuggestedPages are pages that should be created based on the answer
	SuggestedPages []string `json:"suggested_pages,omitempty"`
}

// Citation is a reference to a page
type Citation struct {
	// Path is the page path
	Path string `json:"path"`

	// Title is the page title
	Title string `json:"title"`

	// Relevance is how relevant this citation is (0-1)
	Relevance float64 `json:"relevance"`

	// Excerpt is the relevant excerpt
	Excerpt string `json:"excerpt,omitempty"`
}

// Schema defines the extraction schema
type Schema struct {
	// ExtractEntities whether to extract entities
	ExtractEntities bool `json:"extract_entities"`

	// ExtractConcepts whether to extract concepts
	ExtractConcepts bool `json:"extract_concepts"`

	// ExtractRelationships whether to extract relationships
	ExtractRelationships bool `json:"extract_relationships"`

	// EntityTypes specific entity types to look for
	EntityTypes []string `json:"entity_types,omitempty"`

	// ConceptTypes specific concept types to look for
	ConceptTypes []string `json:"concept_types,omitempty"`
}

// DefaultSchema returns the default extraction schema
func DefaultSchema() *Schema {
	return &Schema{
		ExtractEntities:      true,
		ExtractConcepts:      true,
		ExtractRelationships: true,
	}
}
