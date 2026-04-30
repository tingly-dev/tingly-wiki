package locomo

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-wiki/eval"
)

// LoadConversations reads a LoCoMo JSON file. Supports both an array of
// Conversation objects and a map keyed by conversation ID.
func LoadConversations(path string) ([]*Conversation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Try JSON array first.
	var arr []*Conversation
	if err := json.Unmarshal(b, &arr); err == nil {
		return arr, nil
	}

	// Fall back to map format: { "conv_id": { ... }, ... }
	var m map[string]*Conversation
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: expected JSON array or object: %w", path, err)
	}
	out := make([]*Conversation, 0, len(m))
	for id, c := range m {
		if c.ConversationID == "" {
			c.ConversationID = id
		}
		out = append(out, c)
	}
	return out, nil
}

// ToScenarios converts LoCoMo conversations to eval.Scenario instances.
// Each conversation becomes one scenario: sessions become setup ops and
// questions become queries. Pass limit ≤ 0 to convert all conversations.
func ToScenarios(convs []*Conversation, limit int) []*eval.Scenario {
	if limit > 0 && len(convs) > limit {
		convs = convs[:limit]
	}
	out := make([]*eval.Scenario, 0, len(convs))
	for _, c := range convs {
		sc := conversationToScenario(c)
		if sc != nil {
			out = append(out, sc)
		}
	}
	return out
}

// evidenceRE matches strings like "session_3", "session 2", "session-1".
var evidenceRE = regexp.MustCompile(`(?i)session[_\s-](\d+)`)

func conversationToScenario(c *Conversation) *eval.Scenario {
	if len(c.Sessions) == 0 || len(c.Questions) == 0 {
		return nil
	}

	sc := &eval.Scenario{
		Name: fmt.Sprintf("locomo-%s", c.ConversationID),
		Description: fmt.Sprintf("LoCoMo conversation %s (%d sessions, %d questions)",
			c.ConversationID, len(c.Sessions), len(c.Questions)),
		Config: eval.ScenarioConfig{UseVector: true},
	}

	// Build session-ID → path map used to resolve evidence strings.
	sessionPaths := make(map[int]string, len(c.Sessions))
	for _, s := range c.Sessions {
		title := fmt.Sprintf("session-%d", s.SessionID)
		sessionPaths[s.SessionID] = fmt.Sprintf("memories/%s.md", title)

		sc.Setup = append(sc.Setup, eval.SetupOp{
			Op:      "store",
			Type:    "memory",
			Title:   title,
			Content: sessionContent(s),
		})
	}

	for _, q := range c.Questions {
		relevant := resolveEvidence(q.Evidence, sessionPaths)
		if len(relevant) == 0 {
			continue
		}
		sc.Queries = append(sc.Queries, eval.Query{
			Query:         q.Question,
			Types:         []string{"memory"},
			K:             5,
			RelevantPaths: relevant,
		})
	}

	if len(sc.Queries) == 0 {
		return nil
	}
	return sc
}

func sessionContent(s Session) string {
	var sb strings.Builder
	for _, t := range s.Dialogue {
		sb.WriteString(t.Speaker)
		sb.WriteString(": ")
		sb.WriteString(t.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// resolveEvidence maps evidence strings (e.g. "session_3") to page paths.
func resolveEvidence(evidence []string, paths map[int]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, ev := range evidence {
		m := evidenceRE.FindStringSubmatch(ev)
		if len(m) < 2 {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if p, ok := paths[id]; ok && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
