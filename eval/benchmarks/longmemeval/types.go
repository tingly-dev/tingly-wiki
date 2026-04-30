// Package longmemeval provides an adapter from the LongMemEval benchmark
// dataset (github.com/xiaowu0162/LongMemEval) to eval.Scenario instances.
//
// LongMemEval covers five evaluation dimensions:
//   - information_extraction  – single-hop fact recall
//   - temporal_reasoning      – time-sensitive fact retrieval
//   - knowledge_update        – superseded fact handling (bi-temporal)
//   - multi_session_reasoning – facts distributed across sessions
//   - abstention              – correctly declining unanswerable questions
package longmemeval

// Item is a single LongMemEval evaluation instance.
type Item struct {
	ID       string       `json:"id"`
	// Category is one of the five evaluation dimensions above.
	Category string       `json:"category"`
	Question string       `json:"question"`
	Answer   string       `json:"answer"`
	Sessions []LMESession `json:"sessions"`
	// RelevantSessionIDs optionally lists sessions that contain the evidence.
	// When absent, all sessions are treated as relevant.
	RelevantSessionIDs []string `json:"relevant_session_ids"`
}

// LMESession is a memory session within a LongMemEval item.
type LMESession struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}
