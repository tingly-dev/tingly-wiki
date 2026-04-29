// Package eval provides scenario-driven quality evaluation for tingly-wiki.
//
// A Scenario describes a setup phase (a series of memory writes) and a query
// phase (a series of recall operations with expected results). The Runner
// executes a Scenario against a real MemoryWiki and produces precision/recall/
// MRR metrics, suitable for CI gates or comparative reports.
package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Scenario is the top-level evaluation unit.
type Scenario struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Config      ScenarioConfig `yaml:"config"`
	Setup       []SetupOp      `yaml:"setup"`
	Queries     []Query        `yaml:"queries"`
}

// ScenarioConfig controls runtime configuration for a scenario.
type ScenarioConfig struct {
	// UseVector enables the vector index. When false, retrieval is keyword-only.
	UseVector bool `yaml:"use_vector"`

	// UseLinkExtractor enables RegexLinkExtractor with DefaultLinkPatterns.
	UseLinkExtractor bool `yaml:"use_link_extractor"`
}

// SetupOp is a single write-side operation executed before queries.
type SetupOp struct {
	// Op is one of: "store", "set_ttl", "set_importance".
	// Default (empty string) is "store".
	Op string `yaml:"op"`

	// Type is the PageType: preference / memory / procedure / audit_log.
	Type string `yaml:"type"`

	Title      string  `yaml:"title"`
	Content    string  `yaml:"content"`
	Importance float64 `yaml:"importance"`
	Tags       []string `yaml:"tags"`

	// TTL is a Go duration string ("1h", "30m"); empty means no expiry.
	TTL string `yaml:"ttl"`

	Tenant string `yaml:"tenant"`
	Agent  string `yaml:"agent"`

	// Path identifies the page for set_ttl / set_importance ops.
	Path string `yaml:"path"`

	// Score is the new importance for set_importance.
	Score float64 `yaml:"score"`
}

// Query is one recall measurement.
type Query struct {
	// Query is the user-facing query string.
	Query string `yaml:"query"`

	// Tenant scopes the query.
	Tenant string `yaml:"tenant"`

	// Types filters retrieval to specific PageTypes.
	Types []string `yaml:"types"`

	// K is the cut-off rank for precision@k / recall@k. Defaults to 5.
	K int `yaml:"k"`

	// RelevantPaths is the ground-truth set of paths considered "relevant" for
	// this query. Order does not matter.
	RelevantPaths []string `yaml:"relevant_paths"`

	// MinImportance is an optional floor on importance.
	MinImportance float64 `yaml:"min_importance"`

	// IncludeInvalidated lets the query expose facts past their InvalidatedAt.
	IncludeInvalidated bool `yaml:"include_invalidated"`
}

// LoadScenario reads a single YAML file.
func LoadScenario(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var sc Scenario
	if err := yaml.Unmarshal(b, &sc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if sc.Name == "" {
		// Use filename (without extension) as fallback name
		sc.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if err := validate(&sc); err != nil {
		return nil, fmt.Errorf("invalid scenario %s: %w", sc.Name, err)
	}
	return &sc, nil
}

// LoadScenarios reads every *.yaml or *.yml file under root (non-recursive).
// Returns scenarios sorted by Name for deterministic ordering.
func LoadScenarios(root string) ([]*Scenario, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", root, err)
	}
	var out []*Scenario
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		sc, err := LoadScenario(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

func validate(sc *Scenario) error {
	if len(sc.Queries) == 0 {
		return fmt.Errorf("scenario must have at least one query")
	}
	for i, op := range sc.Setup {
		opName := op.Op
		if opName == "" {
			opName = "store"
		}
		switch opName {
		case "store":
			if op.Title == "" {
				return fmt.Errorf("setup[%d] store: title is required", i)
			}
			if op.Type == "" {
				return fmt.Errorf("setup[%d] store: type is required", i)
			}
		case "set_ttl", "set_importance":
			if op.Path == "" {
				return fmt.Errorf("setup[%d] %s: path is required", i, opName)
			}
		default:
			return fmt.Errorf("setup[%d] unknown op: %s", i, opName)
		}
	}
	for i, q := range sc.Queries {
		if q.Query == "" {
			return fmt.Errorf("queries[%d]: query is required", i)
		}
	}
	return nil
}

// ParseDuration parses an SetupOp.TTL string. Empty string returns nil.
func ParseDuration(s string) (*time.Duration, error) {
	if s == "" {
		return nil, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
