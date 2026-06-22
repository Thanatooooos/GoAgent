package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	for _, item := range []struct{ path, label string }{
		{"testdata/evals/rewrite/latest_run_clean.json", "48"},
		{"testdata/evals/rewrite/v2_24_soft_gate.json", "v2"},
	} {
		raw, _ := os.ReadFile(item.path)
		start := strings.Index(string(raw), `{"suite"`)
		var d struct {
			Samples []struct {
				Name string   `json:"name"`
				Tags []string `json:"tags"`
			} `json:"samples"`
			Artifacts struct {
				Executions map[string]struct {
					RetrievalComparison struct {
						Baseline  struct{ ReciprocalRank float64 `json:"reciprocalRank"` } `json:"baseline"`
						Candidate struct{ ReciprocalRank float64 `json:"reciprocalRank"` } `json:"candidate"`
					} `json:"retrieval_comparison"`
				} `json:"executions"`
			} `json:"artifacts"`
		}
		_ = json.Unmarshal(raw[start:], &d)

		type acc struct{ n int; b, c float64 }
		fu, non := acc{}, acc{}
		for _, s := range d.Samples {
			rc := d.Artifacts.Executions[s.Name].RetrievalComparison
			isFU := false
			for _, t := range s.Tags {
				if t == "followup" {
					isFU = true
					break
				}
			}
			target := &non
			if isFU {
				target = &fu
			}
			target.n++
			target.b += rc.Baseline.ReciprocalRank
			target.c += rc.Candidate.ReciprocalRank
		}
		fmt.Printf("\n=== %s ===\n", item.label)
		if fu.n > 0 {
			fmt.Printf("followup (n=%d): baseline MRR=%.3f candidate MRR=%.3f uplift=%+.3f\n",
				fu.n, fu.b/float64(fu.n), fu.c/float64(fu.n), (fu.c-fu.b)/float64(fu.n))
		}
		if non.n > 0 {
			fmt.Printf("non-followup (n=%d): baseline MRR=%.3f candidate MRR=%.3f uplift=%+.3f\n",
				non.n, non.b/float64(non.n), non.c/float64(non.n), (non.c-non.b)/float64(non.n))
		}
	}
}
