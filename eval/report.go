package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToMarkdown renders a slice of ScenarioMetric as a single markdown report.
// Format:
//
//	# Memory Quality Report
//
//	| Scenario | P@K | R@K | MRR | p50 | p95 |
//	|----------|-----|-----|-----|-----|-----|
//	| ...      | ... | ... | ... | ... | ... |
//
//	## scenario-name
//	(per-query breakdown)
func ToMarkdown(metrics []*ScenarioMetric) string {
	var sb strings.Builder
	sb.WriteString("# Memory Quality Report\n\n")

	// Summary table
	sb.WriteString("| Scenario | P@K | R@K | MRR | p50 | p95 |\n")
	sb.WriteString("|----------|-----|-----|-----|-----|-----|\n")
	for _, m := range metrics {
		fmt.Fprintf(&sb, "| %s | %.3f | %.3f | %.3f | %s | %s |\n",
			m.Scenario, m.AvgPrecisionK, m.AvgRecallK, m.MRR,
			fmtDur(m.P50), fmtDur(m.P95))
	}
	sb.WriteString("\n")

	// Per-scenario details
	for _, m := range metrics {
		fmt.Fprintf(&sb, "## %s\n\n", m.Scenario)
		sb.WriteString("| Query | P@K | R@K | RR | Latency | Returned (top 3) |\n")
		sb.WriteString("|-------|-----|-----|----|---------|------------------|\n")
		for _, q := range m.QueryMetrics {
			top := q.Returned
			if len(top) > 3 {
				top = top[:3]
			}
			fmt.Fprintf(&sb, "| `%s` | %.3f | %.3f | %.3f | %s | %s |\n",
				escapeMD(q.Query), q.PrecisionAtK, q.RecallAtK, q.RR,
				fmtDur(q.Latency), strings.Join(top, ", "))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToJSON serialises a slice of ScenarioMetric as pretty-printed JSON.
func ToJSON(metrics []*ScenarioMetric) ([]byte, error) {
	type metricOut struct {
		Scenario      string  `json:"scenario"`
		AvgPrecisionK float64 `json:"avg_precision_at_k"`
		AvgRecallK    float64 `json:"avg_recall_at_k"`
		MRR           float64 `json:"mrr"`
		P50Ms         int64   `json:"p50_ms"`
		P95Ms         int64   `json:"p95_ms"`
		Queries       []struct {
			Query        string  `json:"query"`
			K            int     `json:"k"`
			PrecisionAtK float64 `json:"precision_at_k"`
			RecallAtK    float64 `json:"recall_at_k"`
			RR           float64 `json:"rr"`
			LatencyMs    int64   `json:"latency_ms"`
			Returned     []string `json:"returned"`
			Relevant     []string `json:"relevant"`
		} `json:"queries"`
	}

	out := make([]metricOut, len(metrics))
	for i, m := range metrics {
		out[i].Scenario = m.Scenario
		out[i].AvgPrecisionK = m.AvgPrecisionK
		out[i].AvgRecallK = m.AvgRecallK
		out[i].MRR = m.MRR
		out[i].P50Ms = m.P50.Milliseconds()
		out[i].P95Ms = m.P95.Milliseconds()
		out[i].Queries = make([]struct {
			Query        string  `json:"query"`
			K            int     `json:"k"`
			PrecisionAtK float64 `json:"precision_at_k"`
			RecallAtK    float64 `json:"recall_at_k"`
			RR           float64 `json:"rr"`
			LatencyMs    int64   `json:"latency_ms"`
			Returned     []string `json:"returned"`
			Relevant     []string `json:"relevant"`
		}, len(m.QueryMetrics))
		for j, q := range m.QueryMetrics {
			out[i].Queries[j].Query = q.Query
			out[i].Queries[j].K = q.K
			out[i].Queries[j].PrecisionAtK = q.PrecisionAtK
			out[i].Queries[j].RecallAtK = q.RecallAtK
			out[i].Queries[j].RR = q.RR
			out[i].Queries[j].LatencyMs = q.Latency.Milliseconds()
			out[i].Queries[j].Returned = q.Returned
			out[i].Queries[j].Relevant = q.Relevant
		}
	}
	return json.MarshalIndent(out, "", "  ")
}

func fmtDur(d interface{ Milliseconds() int64 }) string {
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// escapeMD does minimal escaping for query strings appearing in a markdown table.
func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
