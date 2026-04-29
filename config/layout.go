package config

import "strings"

// LayoutConfig defines the directory structure of the wiki
type LayoutConfig struct {
	// WikiRoot is the root directory of the wiki
	WikiRoot string `json:"wiki_root"`

	// SourcesDir is where source summaries go (default: "sources/")
	SourcesDir string `json:"sources_dir"`

	// EntitiesDir is where entity pages go (default: "entities/")
	EntitiesDir string `json:"entities_dir"`

	// ConceptsDir is where concept pages go (default: "concepts/")
	ConceptsDir string `json:"concepts_dir"`

	// SynthesisDir is where synthesis pages go (default: "synthesis/")
	SynthesisDir string `json:"synthesis_dir"`

	// IndexPath is the index file (default: "index.md")
	IndexPath string `json:"index_path"`

	// LogPath is the log file (default: "log.md")
	LogPath string `json:"log_path"`

	// RawDir is for raw source documents (default: "raw/")
	RawDir string `json:"raw_dir"`

	// Memory-system directories

	// MemoriesDir is for cross-session working memory (default: "memories/")
	MemoriesDir string `json:"memories_dir"`

	// PreferencesDir is for persistent user/agent preferences (default: "preferences/")
	PreferencesDir string `json:"preferences_dir"`

	// AuditDir is for agent operation logs (default: "audit/")
	AuditDir string `json:"audit_dir"`

	// ProceduresDir is for reusable workflow/skill pages (default: "procedures/")
	ProceduresDir string `json:"procedures_dir"`
}

// DefaultLayout returns the default layout configuration
func DefaultLayout() *LayoutConfig {
	return &LayoutConfig{
		SourcesDir:     "sources/",
		EntitiesDir:    "entities/",
		ConceptsDir:    "concepts/",
		SynthesisDir:   "synthesis/",
		IndexPath:      "index.md",
		LogPath:        "log.md",
		RawDir:         "raw/",
		MemoriesDir:    "memories/",
		PreferencesDir: "preferences/",
		AuditDir:       "audit/",
		ProceduresDir:  "procedures/",
	}
}

// GetSourcePath returns the path for a source page
func (l *LayoutConfig) GetSourcePath(id string) string {
	return l.SourcesDir + id + ".md"
}

// GetEntityPath returns the path for an entity page
func (l *LayoutConfig) GetEntityPath(name string) string {
	// Sanitize name: lowercase, replace spaces with hyphens
	sanitized := sanitizeName(name)
	return l.EntitiesDir + sanitized + ".md"
}

// GetConceptPath returns the path for a concept page
func (l *LayoutConfig) GetConceptPath(name string) string {
	sanitized := sanitizeName(name)
	return l.ConceptsDir + sanitized + ".md"
}

// GetSynthesisPath returns the path for a synthesis page
func (l *LayoutConfig) GetSynthesisPath(name string) string {
	sanitized := sanitizeName(name)
	return l.SynthesisDir + sanitized + ".md"
}

// GetMemoryPath returns the path for a cross-session memory page
func (l *LayoutConfig) GetMemoryPath(title string) string {
	return l.MemoriesDir + sanitizeName(title) + ".md"
}

// GetPreferencePath returns the path for a preference page
func (l *LayoutConfig) GetPreferencePath(key string) string {
	return l.PreferencesDir + sanitizeName(key) + ".md"
}

// GetAuditLogPath returns the path for a date-scoped audit log file (e.g., "audit/2026-04.md")
func (l *LayoutConfig) GetAuditLogPath(yearMonth string) string {
	return l.AuditDir + yearMonth + ".md"
}

// GetProcedurePath returns the path for a procedure/skill page
func (l *LayoutConfig) GetProcedurePath(name string) string {
	return l.ProceduresDir + sanitizeName(name) + ".md"
}

// sanitizeName converts a name to a safe filename
func sanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace spaces and special chars with hyphens
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	// Collapse multiple hyphens
	name = replaceAllSequential(name, "--", "-")
	name = replaceAllSequential(name, "--", "-") // Twice to catch triples
	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")
	return name
}

// replaceAllSequential replaces all occurrences (Go 1.21 compatibility)
func replaceAllSequential(s, old, new string) string {
	for strings.Contains(s, old) {
		s = strings.ReplaceAll(s, old, new)
	}
	return s
}
