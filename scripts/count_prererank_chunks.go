package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	raw, _ := os.ReadFile("testdata/evals/rewrite/v2_24_rerank_on.json")
	i := strings.Index(string(raw), `{"suite"`)
	var d struct {
		Artifacts struct {
			Executions map[string]struct {
				SubQuestions []string `json:"sub_questions"`
				RetrievalComparison struct {
					CandidatePipeline map[string]any `json:"candidate_pipeline"`
				} `json:"retrieval_comparison"`
			} `json:"executions"`
		} `json:"artifacts"`
	}
	_ = json.Unmarshal(raw[i:], &d)

	type row struct {
		name string
		subs int
		pre, fin int
		merge, rerank bool
	}
	rows := []row{}
	for name, ex := range d.Artifacts.Executions {
		p := ex.RetrievalComparison.CandidatePipeline
		if p == nil {
			continue
		}
		pre := len(toStr(p["pre_rerank_chunk_ids"]))
		fin := len(toStr(p["final_chunk_ids"]))
		merge, _ := p["sub_question_merge"].(bool)
		rerank, _ := p["rerank_applied"].(bool)
		rows = append(rows, row{name, len(ex.SubQuestions), pre, fin, merge, rerank})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	fmt.Println("sample | #subQ | pre_rerank | final | sub_merge | rerank")
	for _, r := range rows {
		fmt.Printf("%s | %d | %d | %d | %v | %v\n", r.name, r.subs, r.pre, r.fin, r.merge, r.rerank)
	}
}

func toStr(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
