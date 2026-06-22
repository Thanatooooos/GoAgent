package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	raw, err := os.ReadFile("testdata/evals/rewrite/v2_24_soft_gate.json")
	if err != nil {
		panic(err)
	}
	var data struct {
		RunMetadata struct {
			RunAt string `json:"run_at"`
		} `json:"run_metadata"`
		Samples []struct {
			Name             string         `json:"name"`
			Passed           bool           `json:"passed"`
			CriticalFailures []string       `json:"critical_failures"`
			FailureReasons   []string       `json:"failure_reasons"`
			RuleChecks       map[string]any `json:"rule_checks"`
			Scores           map[string]any `json:"scores"`
		} `json:"samples"`
		Aggregate struct {
			PassRate float64        `json:"pass_rate"`
			Metrics  map[string]any `json:"metrics"`
		} `json:"aggregate"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		panic(err)
	}

	m := data.Aggregate.Metrics
	fmt.Printf("Run: %s\n", data.RunMetadata.RunAt)
	fmt.Printf("Aggregate: pass=%.1f%%  mrr_uplift=%v  avg_semantic=%v  avg_judge=%v  overrides=%v\n\n",
		data.Aggregate.PassRate*100, m["mrr_uplift"], m["avg_semantic_score"], m["avg_judge_score"], m["semantic_judge_override_count"])

	names := make([]string, len(data.Samples))
	byName := map[string]int{}
	for i, s := range data.Samples {
		names[i] = s.Name
		byName[s.Name] = i
	}
	sort.Strings(names)

	fmt.Println("| sample | pass | path | rule_ok | soft_ok | rq | sem | sim | judge | r_imp | diag | failures |")
	fmt.Println("|--------|------|------|---------|---------|----|----|-----|-------|-------|------|----------|")
	for _, name := range names {
		s := data.Samples[byName[name]]
		fmt.Printf("| %s | %t | %s | %t | %t | %s | %s | %s | %s | %s | %s | %s |\n",
			name,
			s.Passed,
			scoreStr(s.Scores, "pass_path"),
			boolVal(s.RuleChecks, "rule_passed"),
			boolVal(s.RuleChecks, "semantic_soft_gate_ok"),
			floatVal(s.Scores, "rewrite_quality"),
			floatVal(s.Scores, "semantic_score"),
			floatVal(s.Scores, "rewrite_similarity"),
			floatVal(s.Scores, "judge_score"),
			floatVal(s.Scores, "retrieval_impact"),
			floatVal(s.Scores, "diagnostic_score"),
			failSummary(s.CriticalFailures, s.FailureReasons),
		)
	}
}

func scoreStr(scores map[string]any, key string) string {
	if scores == nil {
		return "-"
	}
	v, ok := scores[key]
	if !ok || v == nil {
		return "-"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func floatVal(scores map[string]any, key string) string {
	if scores == nil {
		return "-"
	}
	v, ok := scores[key]
	if !ok || v == nil {
		return "-"
	}
	f, ok := v.(float64)
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%.2f", f)
}

func boolVal(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}

func failSummary(critical, reasons []string) string {
	parts := append([]string(nil), critical...)
	seen := map[string]struct{}{}
	for _, p := range parts {
		seen[p] = struct{}{}
	}
	for _, r := range reasons {
		if _, ok := seen[r]; ok {
			continue
		}
		parts = append(parts, r)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "; ")
}
