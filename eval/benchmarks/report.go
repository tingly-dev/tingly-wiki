package benchmarks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToMarkdown renders a benchmark comparison report as a Markdown string.
// Results are grouped by dataset, then each group is followed by a
// per-category summary and a column for each published baseline.
func ToMarkdown(results []*BenchmarkResult) string {
	if len(results) == 0 {
		return "# Benchmark Comparison Report\n\n_No results._\n"
	}

	var sb strings.Builder
	sb.WriteString("# Benchmark Comparison Report\n\n")

	groups := groupByDataset(results)
	for _, dataset := range sortedKeys(groups) {
		sub := groups[dataset]
		sb.WriteString(fmt.Sprintf("## %s\n\n", titleCase(dataset)))

		baselines := sub[0].Baselines
		bKeys := sortedKeys(baselines)

		// Table header.
		header := "| Scenario | P@K | R@K | MRR | p95 |"
		sep := "|----------|-----|-----|-----|-----|"
		for _, bk := range bKeys {
			header += fmt.Sprintf(" %s (%.1f%%) |", bk, baselines[bk]*100)
			sep += "--------|"
		}
		sb.WriteString(header + "\n")
		sb.WriteString(sep + "\n")

		for _, r := range sub {
			m := r.Metric
			row := fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %s |",
				m.Scenario, m.AvgPrecisionK, m.AvgRecallK, m.MRR,
				m.P95.Round(1_000_000))
			for _, bk := range bKeys {
				row += fmt.Sprintf(" %.3f |", baselines[bk])
			}
			sb.WriteString(row + "\n")
		}
		sb.WriteString("\n")

		// Category breakdown (only meaningful when there are multiple categories).
		catSummary := summariseByCategory(sub)
		if len(catSummary) > 1 {
			sb.WriteString("### By Category\n\n")
			sb.WriteString("| Category | Avg MRR | Scenarios |\n")
			sb.WriteString("|----------|---------|-----------|\n")
			for _, cat := range sortedKeys(catSummary) {
				cs := catSummary[cat]
				sb.WriteString(fmt.Sprintf("| %s | %.3f | %d |\n", cat, cs.avgMRR, cs.count))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ToJSON renders benchmark results as indented JSON bytes.
func ToJSON(results []*BenchmarkResult) ([]byte, error) {
	type row struct {
		Dataset      string             `json:"dataset"`
		Category     string             `json:"category"`
		Scenario     string             `json:"scenario"`
		PrecisionAtK float64            `json:"precision_at_k"`
		RecallAtK    float64            `json:"recall_at_k"`
		MRR          float64            `json:"mrr"`
		P95Ms        int64              `json:"p95_ms"`
		Baselines    map[string]float64 `json:"baselines"`
	}
	rows := make([]row, len(results))
	for i, r := range results {
		rows[i] = row{
			Dataset:      r.Dataset,
			Category:     r.Category,
			Scenario:     r.Metric.Scenario,
			PrecisionAtK: r.Metric.AvgPrecisionK,
			RecallAtK:    r.Metric.AvgRecallK,
			MRR:          r.Metric.MRR,
			P95Ms:        r.Metric.P95.Milliseconds(),
			Baselines:    r.Baselines,
		}
	}
	return json.MarshalIndent(rows, "", "  ")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

type catStats struct {
	avgMRR float64
	count  int
}

func summariseByCategory(results []*BenchmarkResult) map[string]*catStats {
	m := make(map[string]*catStats)
	for _, r := range results {
		cat := r.Category
		if cat == "" {
			cat = "unknown"
		}
		cs := m[cat]
		if cs == nil {
			cs = &catStats{}
			m[cat] = cs
		}
		cs.avgMRR = (cs.avgMRR*float64(cs.count) + r.Metric.MRR) / float64(cs.count+1)
		cs.count++
	}
	return m
}

func groupByDataset(results []*BenchmarkResult) map[string][]*BenchmarkResult {
	m := make(map[string][]*BenchmarkResult)
	for _, r := range results {
		m[r.Dataset] = append(m[r.Dataset], r)
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
