package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./scripts/print_pipeline_traces.go <eval-result.json>")
		os.Exit(1)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	start := strings.Index(string(raw), `{"suite"`)
	var d struct {
		Samples []struct {
			Name   string `json:"name"`
			Passed bool   `json:"passed"`
		} `json:"samples"`
		Artifacts struct {
			Executions map[string]struct {
				RetrievalComparison struct {
					CandidateRetrievedIDs []string       `json:"candidate_retrieved_ids"`
					CandidatePipeline     map[string]any `json:"candidate_pipeline"`
					Candidate             struct {
						ReciprocalRank float64 `json:"reciprocalRank"`
					} `json:"candidate"`
				} `json:"retrieval_comparison"`
			} `json:"executions"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(raw[start:], &d); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rerankApplied, rerankChanged, missing := 0, 0, 0
	fmt.Printf("pipeline traces from %s\n\n", os.Args[1])
	for _, s := range d.Samples {
		rc := d.Artifacts.Executions[s.Name].RetrievalComparison
		p := rc.CandidatePipeline
		if p == nil {
			missing++
			continue
		}
		applied, _ := p["rerank_applied"].(bool)
		if applied {
			rerankApplied++
		}
		pre, _ := p["pre_rerank_chunk_ids"].([]any)
		final := rc.CandidateRetrievedIDs
		changed := !sameStringSlices(toStrings(pre), final)
		if applied && changed {
			rerankChanged++
		}
		fmt.Printf("%s | MRR=%.2f | rerank=%v model=%v | pre=%v | final=%v\n",
			s.Name, rc.Candidate.ReciprocalRank, applied, p["rerank_model"], toStrings(pre), final)
	}
	fmt.Printf("\nsummary: rerank_applied=%d rerank_reordered=%d missing_pipeline=%d total=%d\n",
		rerankApplied, rerankChanged, missing, len(d.Samples))
}

func toStrings(v []any) []string {
	if len(v) == 0 {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sameStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
