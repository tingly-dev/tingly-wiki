package eval

import (
	"sort"
	"time"
)

// QueryMetric captures the per-query measurement.
type QueryMetric struct {
	Query        string
	Returned     []string // paths in rank order
	Relevant     []string // ground-truth paths (set-valued)
	K            int
	Latency      time.Duration
	PrecisionAtK float64 // |Returned[:k] ∩ Relevant| / k
	RecallAtK    float64 // |Returned[:k] ∩ Relevant| / |Relevant|
	RR           float64 // 1 / rank of first relevant; 0 if none in Returned[:k]
}

// ScenarioMetric aggregates per-query metrics across an entire scenario.
type ScenarioMetric struct {
	Scenario      string
	QueryMetrics  []QueryMetric
	AvgPrecisionK float64
	AvgRecallK    float64
	MRR           float64
	P50           time.Duration
	P95           time.Duration
}

// computeQueryMetric calculates precision@k, recall@k, and reciprocal rank.
func computeQueryMetric(query string, returned, relevant []string, k int, latency time.Duration) QueryMetric {
	if k <= 0 {
		k = 5
	}
	relSet := make(map[string]bool, len(relevant))
	for _, r := range relevant {
		relSet[r] = true
	}

	cutoff := returned
	if len(cutoff) > k {
		cutoff = cutoff[:k]
	}

	hits := 0
	rr := 0.0
	for i, path := range cutoff {
		if relSet[path] {
			hits++
			if rr == 0 {
				rr = 1.0 / float64(i+1)
			}
		}
	}

	var precision, recall float64
	if k > 0 {
		precision = float64(hits) / float64(k)
	}
	if len(relevant) > 0 {
		recall = float64(hits) / float64(len(relevant))
	}

	return QueryMetric{
		Query:        query,
		Returned:     returned,
		Relevant:     relevant,
		K:            k,
		Latency:      latency,
		PrecisionAtK: precision,
		RecallAtK:    recall,
		RR:           rr,
	}
}

// aggregateScenarioMetric rolls up per-query stats into scenario-level metrics.
func aggregateScenarioMetric(name string, qm []QueryMetric) ScenarioMetric {
	out := ScenarioMetric{Scenario: name, QueryMetrics: qm}
	if len(qm) == 0 {
		return out
	}

	var sumP, sumR, sumRR float64
	durations := make([]time.Duration, len(qm))
	for i, m := range qm {
		sumP += m.PrecisionAtK
		sumR += m.RecallAtK
		sumRR += m.RR
		durations[i] = m.Latency
	}
	n := float64(len(qm))
	out.AvgPrecisionK = sumP / n
	out.AvgRecallK = sumR / n
	out.MRR = sumRR / n

	out.P50 = percentile(durations, 50)
	out.P95 = percentile(durations, 95)
	return out
}

// percentile returns the p-th percentile (0-100) of a duration slice.
// Uses nearest-rank method on a copy of the input.
func percentile(durations []time.Duration, p int) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	cp := make([]time.Duration, len(durations))
	copy(cp, durations)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })

	rank := (p * len(cp)) / 100
	if rank >= len(cp) {
		rank = len(cp) - 1
	}
	return cp[rank]
}
