// Package locomo provides an adapter from the LoCoMo benchmark dataset
// (HuggingFace snap-research/locomo) to eval.Scenario instances.
package locomo

// Conversation is the top-level LoCoMo record — one per entry in the dataset.
type Conversation struct {
	ConversationID string    `json:"conversation_id"`
	Sessions       []Session `json:"sessions"`
	Questions      []QAPair  `json:"questions"`
}

// Session is a single chronological conversation block.
type Session struct {
	SessionID int    `json:"session_id"`
	Dialogue  []Turn `json:"dialogue"`
}

// Turn is a single utterance within a session.
type Turn struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// QAPair is a ground-truth question/answer pair derived from the conversation.
type QAPair struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	// Evidence lists session identifiers that contain the answer, e.g. "session_1".
	Evidence []string `json:"evidence"`
	// Category classifies the reasoning type, e.g. "single_session_qa".
	Category string   `json:"category"`
}
