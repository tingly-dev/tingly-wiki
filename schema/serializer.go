package schema

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseFrontmatter parses YAML frontmatter
func parseFrontmatter(content string) (*Frontmatter, error) {
	fm := &Frontmatter{
		Extra: make(map[string]interface{}),
	}

	// First pass: unmarshal into map to handle custom fields
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Extract known fields
	if v, ok := raw["type"].(string); ok {
		fm.Type = PageType(v)
	}
	if v, ok := raw["title"].(string); ok {
		fm.Title = v
	}
	if v, ok := raw["tags"]; ok {
		switch tv := v.(type) {
		case []interface{}:
			fm.Tags = make([]string, len(tv))
			for i, item := range tv {
				if s, ok := item.(string); ok {
					fm.Tags[i] = s
				}
			}
		case []string:
			fm.Tags = tv
		}
	}
	if v, ok := raw["related"]; ok {
		switch tv := v.(type) {
		case []interface{}:
			fm.Related = make([]string, len(tv))
			for i, item := range tv {
				if s, ok := item.(string); ok {
					fm.Related[i] = s
				}
			}
		case []string:
			fm.Related = tv
		}
	}
	if v, ok := raw["sources"]; ok {
		switch tv := v.(type) {
		case []interface{}:
			fm.Sources = make([]string, len(tv))
			for i, item := range tv {
				if s, ok := item.(string); ok {
					fm.Sources[i] = s
				}
			}
		case []string:
			fm.Sources = tv
		}
	}

	// Put remaining fields in Extra
	knownFields := map[string]bool{
		"type": true, "title": true, "tags": true,
		"related": true, "sources": true,
		"created_at": true, "updated_at": true,
	}
	for key, value := range raw {
		if !knownFields[key] {
			fm.Extra[key] = value
		}
	}

	return fm, nil
}

// serializeFrontmatter serializes frontmatter to YAML
func serializeFrontmatter(fm *Frontmatter) (string, error) {
	// Build map for serialization
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

	// Add extra fields
	for key, value := range fm.Extra {
		raw[key] = value
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("failed to serialize frontmatter: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
