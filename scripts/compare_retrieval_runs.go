package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type run struct {
	path  string
	label string
}

func main() {
	runs := []run{
		{"testdata/evals/rewrite/v2_24_soft_gate.json", "V2 24"},
		{"testdata/evals/rewrite/latest_run_clean.json", "原 48"},
	}
	for _, r := range runs {
		raw, err := os.ReadFile(r.path)
		if err != nil {
			fmt.Printf("%s: missing (%v)\n\n", r.label, err)
			continue
		}
		var data struct {
			Samples []struct {
				Name   string         `json:"name"`
				Passed bool           `json:"passed"`
				Scores map[string]any `json:"scores"`
			} `json:"samples"`
			Aggregate struct {
				PassRate float64        `json:"pass_rate"`
				Metrics  map[string]any `json:"metrics"`
			} `json:"aggregate"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			fmt.Printf("%s: parse error %v\n\n", r.label, err)
			continue
		}
		m := data.Aggregate.Metrics
		fmt.Printf("=== %s ===\n", r.label)
		fmt.Printf("pass_rate %.1f%%\n", data.Aggregate.PassRate*100)
		fmt.Printf("MRR baseline=%.3f candidate=%.3f\n", f(m, "baseline_mrr"), f(m, "candidate_mrr"))
		printHit := func(key string) {
			h, _ := m[key].(map[string]any)
			fmt.Printf("Hit@K %s: @1=%.1f%% @3=%.1f%% @5=%.1f%%\n", key,
				100*fv(h, "1"), 100*fv(h, "3"), 100*fv(h, "5"))
		}
		printHit("baseline_hit_at_k")
		printHit("candidate_hit_at_k")

		zeroImpact, lowImpact := 0, 0
		for _, s := range data.Samples {
			ri, ok := s.Scores["retrieval_impact"].(float64)
			if !ok {
				continue
			}
			if ri == 0 {
				zeroImpact++
			} else if ri < 0.5 {
				lowImpact++
			}
		}
		fmt.Printf("retrieval_impact=0: %d/%d, (0,0.5): %d/%d\n\n", zeroImpact, len(data.Samples), lowImpact, len(data.Samples))
	}
}

func f(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

func fv(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}
