package locomo

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-wiki/eval"
)

// rawConversationData is used for raw JSON unmarshaling to handle
// dynamic session keys (session_1, session_2, etc.).
type rawConversationData map[string]json.RawMessage

// LoadConversations reads a LoCoMo JSON file. The actual format is an array
// of LoCoMoRecord objects with dynamic session keys.
func LoadConversations(path string) ([]*Conversation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Parse as JSON array of LoCoMoRecord.
	var rawRecords []json.RawMessage
	if err := json.Unmarshal(b, &rawRecords); err != nil {
		return nil, fmt.Errorf("parse %s: expected JSON array: %w", path, err)
	}

	out := make([]*Conversation, 0, len(rawRecords))
	for i, raw := range rawRecords {
		record, err := parseLoCoMoRecord(raw)
		if err != nil {
			return nil, fmt.Errorf("parse record %d: %w", i, err)
		}
		out = append(out, record)
	}
	return out, nil
}

// parseLoCoMoRecord parses a single LoCoMo record from raw JSON.
func parseLoCoMoRecord(raw json.RawMessage) (*Conversation, error) {
	var rawObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawObj); err != nil {
		return nil, fmt.Errorf("parse object: %w", err)
	}

	record := &Conversation{
		ConversationID: getStringField(rawObj, "sample_id"),
		Questions:      []QAPair{},
		Sessions:       []Session{},
	}

	// Parse QA pairs.
	if qaRaw, ok := rawObj["qa"]; ok {
		var qa []QAPair
		if err := json.Unmarshal(qaRaw, &qa); err == nil {
			record.Questions = qa
		}
	}

	// Parse conversation data to extract sessions.
	if convRaw, ok := rawObj["conversation"]; ok {
		var convData map[string]json.RawMessage
		if err := json.Unmarshal(convRaw, &convData); err == nil {
			sessions, err := extractSessions(convData)
			if err != nil {
				return nil, fmt.Errorf("extract sessions: %w", err)
			}
			record.Sessions = sessions
		}
	}

	return record, nil
}

// getStringField extracts a string field from raw JSON map.
func getStringField(m map[string]json.RawMessage, key string) string {
	if raw, ok := m[key]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return ""
}

// extractSessions extracts sessions from the conversation data.
// Sessions are stored as dynamic keys: session_1, session_2, etc.
func extractSessions(convData map[string]json.RawMessage) ([]Session, error) {
	var sessions []Session

	// Find all session_* keys and sort them numerically.
	sessionKeys := make([]string, 0)
	for key := range convData {
		if strings.HasPrefix(key, "session_") {
			// Exclude *_date_time fields.
			if !strings.HasSuffix(key, "_date_time") && key != "speaker_a" && key != "speaker_b" {
				sessionKeys = append(sessionKeys, key)
			}
		}
	}
	sort.Slice(sessionKeys, func(i, j int) bool {
		return sessionNumber(sessionKeys[i]) < sessionNumber(sessionKeys[j])
	})

	for _, key := range sessionKeys {
		sessionID := sessionNumber(key)
		rawTurns := convData[key]

		var turns []Turn
		if err := json.Unmarshal(rawTurns, &turns); err != nil {
			return nil, fmt.Errorf("parse %s: %w", key, err)
		}

		sessions = append(sessions, Session{
			SessionID: sessionID,
			Dialogue:  turns,
		})
	}

	return sessions, nil
}

// sessionNumber extracts the numeric session ID from a key like "session_12".
func sessionNumber(key string) int {
	// Extract number after "session_".
	re := regexp.MustCompile(`session_(\d+)`)
	m := re.FindStringSubmatch(key)
	if len(m) >= 2 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// ToScenarios converts LoCoMo conversations to eval.Scenario instances.
// Each conversation becomes one scenario: sessions become setup ops and
// questions become queries. Pass limit ≤ 0 to convert all conversations.
func ToScenarios(convs []*Conversation, limit int) []*eval.Scenario {
	out := make([]*eval.Scenario, 0, len(convs))
	for _, c := range convs {
		sc := conversationToScenario(c)
		if sc != nil {
			out = append(out, sc)
			// Stop if we've reached the limit
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// diaIDToSessionRE matches strings like "D1:3", "D12:8" — dialogue ID format.
var diaIDToSessionRE = regexp.MustCompile(`^D(\d+):\d+$`)

// evidenceRE matches strings like "session_3", "session 2", "session-1".
// Kept for backward compatibility but primary format is now D1:3.
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

	// Build dialogue ID → session ID map for evidence resolution.
	diaIDToSession := make(map[string]int)
	for _, s := range c.Sessions {
		title := fmt.Sprintf("session-%d", s.SessionID)

		sc.Setup = append(sc.Setup, eval.SetupOp{
			Op:      "store",
			Type:    "memory",
			Title:   title,
			Content: sessionContent(s),
		})

		// Map each dialogue ID in this session to the session ID.
		for _, t := range s.Dialogue {
			if t.DiaID != "" {
				diaIDToSession[t.DiaID] = s.SessionID
			}
		}
	}

	for _, q := range c.Questions {
		relevant := resolveEvidence(q.Evidence, diaIDToSession)
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

// resolveEvidence maps evidence strings to memory paths.
// Supports both formats:
// - "D1:3" format: Dialogue ID → looks up session ID from diaIDToSession map
// - "session_3" format: Direct session ID (legacy/backward compat)
func resolveEvidence(evidence []string, diaIDToSession map[string]int) []string {
	seen := make(map[string]bool)
	var out []string
	for _, ev := range evidence {
		var sessionID int

		// Try D1:3 format first.
		if m := diaIDToSessionRE.FindStringSubmatch(ev); len(m) >= 2 {
			// Look up the session ID for this dialogue ID.
			if sid, ok := diaIDToSession[ev]; ok {
				sessionID = sid
			}
		} else if m := evidenceRE.FindStringSubmatch(ev); len(m) >= 2 {
			// Legacy session_N format.
			id, err := strconv.Atoi(m[1])
			if err == nil {
				sessionID = id
			}
		}

		if sessionID > 0 {
			path := fmt.Sprintf("memories/session-%d.md", sessionID)
			if !seen[path] {
				seen[path] = true
				out = append(out, path)
			}
		}
	}
	return out
}
