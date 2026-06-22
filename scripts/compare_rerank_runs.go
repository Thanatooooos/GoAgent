package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	for _, item := range []struct{ label, path string }{
		{"rerank-noop", "testdata/evals/rewrite/v2_24_rerank_noop.json"},
		{"rerank-on (qwen3)", "testdata/evals/rewrite/v2_24_rerank_on.json"},
		{"meta OFF (prev)", "testdata/evals/rewrite/v2_24_no_metadata.json"},
	} {
		d := load(item.path)
		m := d.Aggregate.Metrics
		pass := 0
		for _, s := range d.Samples {
			if s.Passed {
				pass++
			}
		}
		rerankApplied, rerankReordered, preHit, finalHit := 0, 0, 0, 0
		for _, s := range d.Samples {
			rc := d.Artifacts.Executions[s.Name].RetrievalComparison
			p := rc.CandidatePipeline
			if p == nil {
				continue
			}
			if v, _ := p["rerank_applied"].(bool); v {
				rerankApplied++
			}
			pre := toStrSlice(p["pre_rerank_chunk_ids"])
			final := rc.CandidateRetrievedIDs
			exp := rc.ExpectedIDs
			if hitAny(pre, exp) {
				preHit++
			}
			if hitAny(final, exp) {
				finalHit++
			}
			if v, _ := p["rerank_applied"].(bool); v && !same(pre, final) {
				rerankReordered++
			}
		}
		fmt.Printf("\n=== %s ===\n", item.label)
		fmt.Printf("pass %d/%d | candidate MRR %.3f | Hit@1 %.1f%% | Hit@5 %.1f%%\n",
			pass, len(d.Samples), num(m["candidate_mrr"]), 100*num(ch(m, "1")), 100*num(ch(m, "5")))
		fmt.Printf("pipeline: rerank_applied=%d rerank_reordered=%d\n", rerankApplied, rerankReordered)
		fmt.Printf("expected in pre_rerank top5: %d/%d | in final top5: %d/%d\n",
			preHit, len(d.Samples), finalHit, len(d.Samples))
	}

	// pairwise diff noop vs on
	fmt.Println("\n=== Per-sample MRR: rerank-noop vs rerank-on ===")
	a, b := load("testdata/evals/rewrite/v2_24_rerank_noop.json"), load("testdata/evals/rewrite/v2_24_rerank_on.json")
	changed := 0
	for _, s := range a.Samples {
		am := a.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		bm := b.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		if am != bm {
			changed++
			fmt.Printf("  %s: %.2f -> %.2f (%+.2f)\n", s.Name, am, bm, bm-am)
		}
	}
	if changed == 0 {
		fmt.Println("  (no MRR differences)")
	}
}

type doc struct {
	Samples []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
	} `json:"samples"`
	Aggregate struct {
		Metrics map[string]any `json:"metrics"`
	} `json:"aggregate"`
	Artifacts struct {
		Executions map[string]struct {
			RetrievalComparison struct {
				ExpectedIDs           []string       `json:"expected_ids"`
				CandidateRetrievedIDs []string       `json:"candidate_retrieved_ids"`
				CandidatePipeline     map[string]any `json:"candidate_pipeline"`
				Candidate             struct {
					ReciprocalRank float64 `json:"reciprocalRank"`
				} `json:"candidate"`
			} `json:"retrieval_comparison"`
		} `json:"executions"`
	} `json:"artifacts"`
}

func load(path string) doc {
	raw, _ := os.ReadFile(path)
	i := strings.Index(string(raw), `{"suite"`)
	var d doc
	_ = json.Unmarshal(raw[i:], &d)
	return d
}

func num(v any) float64 { f, _ := v.(float64); return f }
func ch(m map[string]any, k string) any {
	h, _ := m["candidate_hit_at_k"].(map[string]any)
	return h[k]
}
func toStrSlice(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func hitAny(ids, expected []string) bool {
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for _, e := range expected {
		if _, ok := set[e]; ok {
			return true
		}
	}
	return false
}
func same(a, b []string) bool {
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
