// Package locomo provides an adapter from the LoCoMo benchmark dataset
// (HuggingFace snap-research/locomo) to eval.Scenario instances.
package locomo

// LoCoMoRecord is the top-level LoCoMo record from the actual dataset JSON.
type LoCoMoRecord struct {
	SampleID       string                 `json:"sample_id"`
	Conversation   ConversationData       `json:"conversation"`
	QA             []QAPair               `json:"qa"`
	EventSummary   string                 `json:"event_summary,omitempty"`
	SessionSummary string                 `json:"session_summary,omitempty"`
	Observation    string                 `json:"observation,omitempty"`
}

// ConversationData contains the dialogue sessions keyed by session name.
type ConversationData struct {
	SpeakerA string                 `json:"speaker_a"`
	SpeakerB string                 `json:"speaker_b"`
	Sessions map[string][]Turn      `json:"-"` // Unmarshaled dynamically
}

// Conversation is the normalized format after processing.
type Conversation struct {
	ConversationID string
	Sessions       []Session
	Questions      []QAPair
}

// Session is a single chronological conversation block.
type Session struct {
	SessionID int
	Dialogue  []Turn
}

// Turn is a single utterance within a session.
type Turn struct {
	Speaker     string   `json:"speaker"`
	Text        string   `json:"text"`
	DiaID       string   `json:"dia_id,omitempty"`
	ImgURL      []string `json:"img_url,omitempty"`
	BlipCaption string   `json:"blip_caption,omitempty"`
	Query       string   `json:"query,omitempty"`
}

// QAPair is a ground-truth question/answer pair derived from the conversation.
type QAPair struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	// Evidence lists dialogue identifiers that contain the answer, e.g. "D1:3".
	Evidence []string `json:"evidence"`
	// Category classifies the reasoning type (numeric in original JSON).
	Category int `json:"category"`
}
