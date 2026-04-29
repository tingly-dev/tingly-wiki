package wiki

import (
	"math"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// ImportanceScorer calculates a composite importance score for a memory page.
//
// Score = w_freq×accessFreq + w_decay×timeDecay + w_explicit×agentExplicit + w_backlink×backlinkFactor
//
// Weights sum to 1.0. All factors are normalised to [0, 1].
type ImportanceScorer struct {
	// Weight for access-frequency factor (default 0.35)
	AccessFrequencyWeight float64

	// Weight for time-decay factor (default 0.25)
	TimeDecayWeight float64

	// Weight for the agent-set explicit score (default 0.30)
	AgentExplicitWeight float64

	// Weight for backlink density (default 0.10)
	BacklinkWeight float64

	// DecayLambda controls how fast the time factor decays (default 0.02 → ~55% at 30 days)
	DecayLambda float64
}

// DefaultImportanceScorer returns an ImportanceScorer with sensible defaults.
func DefaultImportanceScorer() *ImportanceScorer {
	return &ImportanceScorer{
		AccessFrequencyWeight: 0.35,
		TimeDecayWeight:       0.25,
		AgentExplicitWeight:   0.30,
		BacklinkWeight:        0.10,
		DecayLambda:           0.02,
	}
}

// Score computes the composite importance for page p.
// Returns a value in [0, 1].
func (s *ImportanceScorer) Score(p *schema.Page) float64 {
	freq := s.accessFrequency(p.AccessCount)
	decay := s.timeDecay(p.LastAccessedAt)
	explicit := p.Importance // agent-assigned; already in [0,1]
	backlink := s.backlinkFactor(len(p.Backlinks))

	return s.AccessFrequencyWeight*freq +
		s.TimeDecayWeight*decay +
		s.AgentExplicitWeight*explicit +
		s.BacklinkWeight*backlink
}

// Tier classifies a page into hot/warm/cold based on its computed score and
// the time since last access.
func (s *ImportanceScorer) Tier(p *schema.Page) schema.MemoryTier {
	score := s.Score(p)
	daysSince := s.daysSinceAccess(p.LastAccessedAt)

	switch {
	case score >= 0.7 && daysSince <= 7:
		return schema.MemoryTierHot
	case score >= 0.3 && daysSince <= 30:
		return schema.MemoryTierWarm
	default:
		return schema.MemoryTierCold
	}
}

// accessFrequency maps AccessCount → [0,1] via sigmoid(count/50).
func (s *ImportanceScorer) accessFrequency(count int) float64 {
	if count <= 0 {
		return 0
	}
	// sigmoid(x) = 1 / (1 + e^-x)
	return 1.0 / (1.0 + math.Exp(-float64(count)/50.0))
}

// timeDecay returns e^(-λ × days), where λ = DecayLambda.
// Returns 1.0 if the page has never been accessed.
func (s *ImportanceScorer) timeDecay(lastAccessed *time.Time) float64 {
	if lastAccessed == nil {
		return 1.0 // newly written, not yet accessed — treat as fresh
	}
	days := time.Since(*lastAccessed).Hours() / 24
	return math.Exp(-s.DecayLambda * days)
}

// backlinkFactor maps backlink count → [0,1] linearly, capping at 10.
func (s *ImportanceScorer) backlinkFactor(count int) float64 {
	if count <= 0 {
		return 0
	}
	v := float64(count) / 10.0
	if v > 1.0 {
		return 1.0
	}
	return v
}

// daysSinceAccess returns days since last access (0 if never accessed).
func (s *ImportanceScorer) daysSinceAccess(lastAccessed *time.Time) float64 {
	if lastAccessed == nil {
		return 0
	}
	return time.Since(*lastAccessed).Hours() / 24
}
