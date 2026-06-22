package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	raw, _ := os.ReadFile("testdata/evals/rewrite/v2_24_soft_gate.json")
	var data struct {
		RunMetadata struct {
			RunAt string `json:"run_at"`
		} `json:"run_metadata"`
		Samples []struct {
			Passed bool           `json:"passed"`
			Scores map[string]any `json:"scores"`
		} `json:"samples"`
		Aggregate struct {
			PassRate float64        `json:"pass_rate"`
			Metrics  map[string]any `json:"metrics"`
		} `json:"aggregate"`
	}
	_ = json.Unmarshal(raw, &data)

	type acc struct {
		n                          int
		rq, sem, sim, judge, rImp, diag float64
	}
	all, pass, fail := acc{}, acc{}, acc{}

	for _, s := range data.Samples {
		add := func(a *acc) {
			a.n++
			a.rq += f(s.Scores, "rewrite_quality")
			a.sem += f(s.Scores, "semantic_score")
			a.sim += f(s.Scores, "rewrite_similarity")
			a.judge += f(s.Scores, "judge_score")
			a.rImp += f(s.Scores, "retrieval_impact")
			a.diag += f(s.Scores, "diagnostic_score")
		}
		add(&all)
		if s.Passed {
			add(&pass)
		} else {
			add(&fail)
		}
	}

	m := data.Aggregate.Metrics
	fmt.Printf("Run: %s\n\n", data.RunMetadata.RunAt)
	fmt.Println("=== 汇总 ===")
	fmt.Printf("pass_rate        %.1f%% (%d/%d)\n", data.Aggregate.PassRate*100, pass.n, all.n)
	fmt.Printf("mrr_uplift       %v\n", m["mrr_uplift"])
	fmt.Printf("semantic_judge_override  %v\n\n", m["semantic_judge_override_count"])

	printAvg := func(title string, a acc) {
		if a.n == 0 {
			return
		}
		n := float64(a.n)
		fmt.Printf("=== %s (n=%d) ===\n", title, a.n)
		fmt.Printf("rewrite_quality  %.3f\n", a.rq/n)
		fmt.Printf("semantic_score   %.3f\n", a.sem/n)
		fmt.Printf("rewrite_similarity %.3f\n", a.sim/n)
		fmt.Printf("judge_score      %.3f\n", a.judge/n)
		fmt.Printf("retrieval_impact %.3f\n", a.rImp/n)
		fmt.Printf("diagnostic_score %.3f\n\n", a.diag/n)
	}
	printAvg("全部样本均值", all)
	printAvg("通过样本均值", pass)
	printAvg("失败样本均值", fail)
}

func f(scores map[string]any, key string) float64 {
	if scores == nil {
		return 0
	}
	v, ok := scores[key].(float64)
	if !ok {
		return 0
	}
	return v
}
