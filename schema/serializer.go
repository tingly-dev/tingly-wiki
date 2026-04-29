package schema

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// knownFrontmatterFields lists all fields handled explicitly by parseFrontmatter.
// Any key not in this set falls through to Frontmatter.Extra.
var knownFrontmatterFields = map[string]bool{
	"type": true, "title": true, "tags": true,
	"related": true, "sources": true,
	"created_at": true, "updated_at": true,
	// memory lifecycle
	"importance": true, "expires_at": true,
	"access_count": true, "last_accessed_at": true,
	"memory_tier": true,
	// provenance
	"tenant_id": true, "agent_id": true,
}

// parseFrontmatter parses YAML frontmatter
func parseFrontmatter(content string) (*Frontmatter, error) {
	fm := &Frontmatter{
		Extra: make(map[string]interface{}),
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Core fields
	if v, ok := raw["type"].(string); ok {
		fm.Type = PageType(v)
	}
	if v, ok := raw["title"].(string); ok {
		fm.Title = v
	}
	fm.Tags = parseStringSlice(raw["tags"])
	fm.Related = parseStringSlice(raw["related"])
	fm.Sources = parseStringSlice(raw["sources"])

	// Memory lifecycle fields
	if v, ok := raw["importance"]; ok {
		fm.Importance = toFloat64(v)
	}
	if v, ok := raw["expires_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			fm.ExpiresAt = &t
		}
	}
	if v, ok := raw["access_count"]; ok {
		fm.AccessCount = int(toFloat64(v))
	}
	if v, ok := raw["last_accessed_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			fm.LastAccessedAt = &t
		}
	}
	if v, ok := raw["memory_tier"].(string); ok {
		fm.MemoryTier = MemoryTier(v)
	}

	// Provenance fields
	if v, ok := raw["tenant_id"].(string); ok {
		fm.TenantID = v
	}
	if v, ok := raw["agent_id"].(string); ok {
		fm.AgentID = v
	}

	// Remaining unknown fields → Extra
	for key, value := range raw {
		if !knownFrontmatterFields[key] {
			fm.Extra[key] = value
		}
	}

	return fm, nil
}

// serializeFrontmatter serializes frontmatter to YAML
func serializeFrontmatter(fm *Frontmatter) (string, error) {
	raw := make(map[string]interface{})

	if fm.Type != "" {
		raw["type"] = string(fm.Type)
	}
	if fm.Title != "" {
		raw["title"] = fm.Title
	}
	if len(fm.Tags) > 0 {
		raw["tags"] = fm.Tags
	}
	if len(fm.Related) > 0 {
		raw["related"] = fm.Related
	}
	if len(fm.Sources) > 0 {
		raw["sources"] = fm.Sources
	}

	// Memory lifecycle
	if fm.Importance != 0 {
		raw["importance"] = fm.Importance
	}
	if fm.ExpiresAt != nil {
		raw["expires_at"] = fm.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if fm.AccessCount != 0 {
		raw["access_count"] = fm.AccessCount
	}
	if fm.LastAccessedAt != nil {
		raw["last_accessed_at"] = fm.LastAccessedAt.UTC().Format(time.RFC3339)
	}
	if fm.MemoryTier != "" {
		raw["memory_tier"] = string(fm.MemoryTier)
	}

	// Provenance
	if fm.TenantID != "" {
		raw["tenant_id"] = fm.TenantID
	}
	if fm.AgentID != "" {
		raw["agent_id"] = fm.AgentID
	}

	// Extra / custom fields
	for key, value := range fm.Extra {
		raw[key] = value
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("failed to serialize frontmatter: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// parseStringSlice converts a raw YAML value to []string
func parseStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch tv := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(tv))
		for _, item := range tv {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return tv
	}
	return nil
}

// toFloat64 converts numeric YAML values to float64
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
