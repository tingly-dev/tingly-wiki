package wiki

import (
	"math"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-wiki/schema"
)

func TestImportanceScorer_Defaults(t *testing.T) {
	s := DefaultImportanceScorer()
	wsum := s.AccessFrequencyWeight + s.TimeDecayWeight + s.AgentExplicitWeight + s.BacklinkWeight
	if math.Abs(wsum-1.0) > 1e-9 {
		t.Fatalf("default weights must sum to 1.0, got %f", wsum)
	}
	if s.DecayLambda <= 0 {
		t.Fatalf("DecayLambda must be > 0, got %f", s.DecayLambda)
	}
}

func TestImportanceScorer_Score_NeverAccessed(t *testing.T) {
	s := DefaultImportanceScorer()
	p := &schema.Page{
		Frontmatter: schema.Frontmatter{Importance: 0.5},
	}
	got := s.Score(p)

	// AccessCount=0 → freq=0; LastAccessedAt=nil → decay=1.0; explicit=0.5; backlink=0
	expected := s.AccessFrequencyWeight*0 + s.TimeDecayWeight*1.0 + s.AgentExplicitWeight*0.5 + s.BacklinkWeight*0
	if math.Abs(got-expected) > 1e-9 {
		t.Errorf("Score(never-accessed)=%f, want %f", got, expected)
	}
}

func TestImportanceScorer_Score_Accessed(t *testing.T) {
	s := DefaultImportanceScorer()
	now := time.Now()
	p := &schema.Page{
		Frontmatter: schema.Frontmatter{
			Importance:     1.0,
			AccessCount:    100,
			LastAccessedAt: &now,
		},
		Backlinks: make([]string, 5),
	}
	got := s.Score(p)
	if got <= 0 || got > 1.0 {
		t.Errorf("Score must be in (0, 1], got %f", got)
	}
	// With 100 accesses (sigmoid≈1), recency≈1, explicit=1, backlink=0.5,
	// score should be very high (>0.9).
	if got < 0.9 {
		t.Errorf("Score with all factors high should be >0.9, got %f", got)
	}
}

func TestImportanceScorer_TimeDecay(t *testing.T) {
	s := DefaultImportanceScorer()
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	got := s.timeDecay(&thirtyDaysAgo)
	// With λ=0.02, e^(-0.02*30) ≈ 0.5488
	expected := math.Exp(-0.02 * 30)
	if math.Abs(got-expected) > 0.01 {
		t.Errorf("timeDecay(30d)=%f, want ≈%f", got, expected)
	}

	// nil → 1.0
	if v := s.timeDecay(nil); v != 1.0 {
		t.Errorf("timeDecay(nil)=%f, want 1.0", v)
	}
}

func TestImportanceScorer_AccessFrequency(t *testing.T) {
	s := DefaultImportanceScorer()

	tests := []struct {
		count int
		want  float64 // approximate
	}{
		{0, 0.0},
		{50, 1.0 / (1 + math.Exp(-1.0))},   // sigmoid(1) ≈ 0.731
		{200, 1.0 / (1 + math.Exp(-4.0))},  // sigmoid(4) ≈ 0.982
	}

	for _, tt := range tests {
		got := s.accessFrequency(tt.count)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("accessFrequency(%d)=%f, want ≈%f", tt.count, got, tt.want)
		}
	}
}

func TestImportanceScorer_BacklinkFactor(t *testing.T) {
	s := DefaultImportanceScorer()

	tests := []struct {
		count int
		want  float64
	}{
		{0, 0.0},
		{5, 0.5},
		{10, 1.0},
		{20, 1.0}, // capped
	}

	for _, tt := range tests {
		got := s.backlinkFactor(tt.count)
		if got != tt.want {
			t.Errorf("backlinkFactor(%d)=%f, want %f", tt.count, got, tt.want)
		}
	}
}

func TestImportanceScorer_Tier(t *testing.T) {
	s := DefaultImportanceScorer()
	now := time.Now()

	tests := []struct {
		name string
		page *schema.Page
		want schema.MemoryTier
	}{
		{
			name: "high importance + recent → hot",
			page: &schema.Page{
				Frontmatter: schema.Frontmatter{
					Importance:     1.0,
					AccessCount:    100,
					LastAccessedAt: &now,
				},
			},
			want: schema.MemoryTierHot,
		},
		{
			name: "moderate score + 20 days ago → warm",
			page: func() *schema.Page {
				t20 := now.Add(-20 * 24 * time.Hour)
				return &schema.Page{
					Frontmatter: schema.Frontmatter{
						Importance:     0.7,
						AccessCount:    10,
						LastAccessedAt: &t20,
					},
				}
			}(),
			want: schema.MemoryTierWarm,
		},
		{
			name: "low importance + old → cold",
			page: func() *schema.Page {
				t60 := now.Add(-60 * 24 * time.Hour)
				return &schema.Page{
					Frontmatter: schema.Frontmatter{
						Importance:     0.1,
						AccessCount:    0,
						LastAccessedAt: &t60,
					},
				}
			}(),
			want: schema.MemoryTierCold,
		},
		{
			// Newly written page, never accessed yet:
			// score = 0 (freq) + 0.25 (decay=1.0) + 0.27 (explicit=0.9) + 0 = 0.52
			// → warm (below 0.7 hot threshold despite high explicit importance)
			name: "no access yet, high explicit → warm",
			page: &schema.Page{
				Frontmatter: schema.Frontmatter{Importance: 0.9},
			},
			want: schema.MemoryTierWarm,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Tier(tt.page); got != tt.want {
				t.Errorf("Tier=%s, want %s (score=%f)", got, tt.want, s.Score(tt.page))
			}
		})
	}
}
