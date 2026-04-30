package longmemeval

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-wiki/eval"
)

// LoadItems reads a LongMemEval JSON file (array of Item objects).
func LoadItems(path string) ([]*Item, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var items []*Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return items, nil
}

// ToScenarios converts LongMemEval items to eval.Scenario instances.
// Each item becomes its own scenario with a fresh wiki instance, keeping
// contexts independent. Pass limit ≤ 0 to convert all items.
func ToScenarios(items []*Item, limit int) []*eval.Scenario {
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]*eval.Scenario, 0, len(items))
	for _, item := range items {
		sc := itemToScenario(item)
		if sc != nil {
			out = append(out, sc)
		}
	}
	return out
}

func itemToScenario(item *Item) *eval.Scenario {
	if len(item.Sessions) == 0 || item.Question == "" {
		return nil
	}

	sc := &eval.Scenario{
		Name:        fmt.Sprintf("lme-%s-%s", item.Category, item.ID),
		Description: fmt.Sprintf("LongMemEval [%s] %s", item.Category, item.ID),
		Config:      eval.ScenarioConfig{UseVector: true},
	}

	for _, s := range item.Sessions {
		sc.Setup = append(sc.Setup, eval.SetupOp{
			Op:      "store",
			Type:    "memory",
			Title:   s.SessionID,
			Content: s.Content,
		})
	}

	sc.Queries = []eval.Query{
		{
			Query:         item.Question,
			Types:         []string{"memory"},
			K:             5,
			RelevantPaths: relevantPaths(item),
		},
	}
	return sc
}

func relevantPaths(item *Item) []string {
	if len(item.RelevantSessionIDs) > 0 {
		paths := make([]string, len(item.RelevantSessionIDs))
		for i, sid := range item.RelevantSessionIDs {
			paths[i] = fmt.Sprintf("memories/%s.md", sid)
		}
		return paths
	}
	// Fallback: treat all sessions as relevant.
	paths := make([]string, len(item.Sessions))
	for i, s := range item.Sessions {
		paths[i] = fmt.Sprintf("memories/%s.md", s.SessionID)
	}
	return paths
}
