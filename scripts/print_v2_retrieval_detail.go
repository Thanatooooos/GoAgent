package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	raw, _ := os.ReadFile("testdata/evals/rewrite/v2_24_soft_gate.json")
	var data struct {
		Artifacts map[string]any `json:"artifacts"`
	}
	_ = json.Unmarshal(raw, &data)
	execs, _ := data.Artifacts["executions"].(map[string]any)

	type row struct {
		name     string
		baseMRR  float64
		candMRR  float64
		baseHit1 bool
		candHit1 bool
		critOK   bool
	}
	rows := []row{}
	for name, rawArt := range execs {
		art, _ := rawArt.(map[string]any)
		rc, _ := art["retrieval_comparison"].(map[string]any)
		if rc == nil {
			continue
		}
		r := row{name: name, critOK: true}
		if v, ok := rc["critical_ids_ok"].(bool); ok {
			r.critOK = v
		}
		if b, ok := rc["baseline"].(map[string]any); ok {
			r.baseMRR = f(b, "reciprocalRank")
			r.baseHit1 = hit(b, 1)
		}
		if c, ok := rc["candidate"].(map[string]any); ok {
			r.candMRR = f(c, "reciprocalRank")
			r.candHit1 = hit(c, 1)
		}
		rows = append(rows, r)
	}

	fmt.Println("V2 per-sample retrieval (from artifacts):")
	fmt.Println("| sample | baseline MRR | candidate MRR | b@1 | c@1 | critical_ok |")
	for _, r := range rows {
		fmt.Printf("| %s | %.2f | %.2f | %t | %t | %t |\n",
			r.name, r.baseMRR, r.candMRR, r.baseHit1, r.candHit1, r.critOK)
	}

	var bSum, cSum float64
	bHit, cHit, critMiss := 0, 0, 0
	for _, r := range rows {
		bSum += r.baseMRR
		cSum += r.candMRR
		if r.baseHit1 {
			bHit++
		}
		if r.candHit1 {
			cHit++
		}
		if !r.critOK {
			critMiss++
		}
	}
	n := float64(len(rows))
	fmt.Printf("\nPer-sample avg MRR: baseline=%.3f candidate=%.3f\n", bSum/n, cSum/n)
	fmt.Printf("Per-sample Hit@1: baseline=%.1f%% candidate=%.1f%%\n", 100*float64(bHit)/n, 100*float64(cHit)/n)
	fmt.Printf("critical_ids miss: %d/%d\n", critMiss, len(rows))
}

func f(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

func hit(m map[string]any, k int) bool {
	h, _ := m["hitAtK"].(map[string]any)
	key := fmt.Sprintf("%d", k)
	v, _ := h[key].(bool)
	return v
}
