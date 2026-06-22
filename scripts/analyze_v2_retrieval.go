package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func main() {
	raw, err := os.ReadFile("testdata/evals/rewrite/v2_24_soft_gate.json")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var d struct {
		Samples []struct {
			Name             string   `json:"name"`
			Tags             []string `json:"tags"`
			Passed           bool     `json:"passed"`
			CriticalFailures []string `json:"critical_failures"`
			Scores           map[string]any `json:"scores"`
		} `json:"samples"`
		Artifacts map[string]any `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	execs, _ := d.Artifacts["executions"].(map[string]any)

	fmt.Println("=== FAIL samples retrieval detail ===")
	for _, s := range d.Samples {
		if s.Passed {
			continue
		}
		art, _ := execs[s.Name].(map[string]any)
		rw, _ := art["rewrite"].(map[string]any)
		rc, _ := art["retrieval_comparison"].(map[string]any)
		fmt.Printf("\n--- %s [%v] ---\n", s.Name, s.Tags)
		fmt.Printf("failures: %v  retrieval_impact=%v\n", s.CriticalFailures, s.Scores["retrieval_impact"])
		if rw != nil {
			fmt.Printf("rewritten: %q\n", rw["rewritten_query"])
			fmt.Printf("sub_questions: %v\n", rw["sub_questions"])
		}
		if rc != nil {
			fmt.Printf("critical_ids_ok: %v\n", rc["critical_ids_ok"])
			for _, side := range []string{"baseline", "candidate"} {
				m, _ := rc[side].(map[string]any)
				if m == nil {
					continue
				}
				fmt.Printf("%s MRR=%v rank=%v expected=%v\n", side, m["reciprocalRank"], m["firstRelevantRank"], m["expectedIds"])
				ids, _ := m["retrievedIds"].([]any)
				fmt.Printf("%s top retrieved: %v\n", side, ids)
			}
		}
	}

	type tagStat struct {
		n, bMRR, cMRR float64
		critMiss      int
	}
	tags := map[string]*tagStat{}
	tagOf := map[string][]string{}
	for _, s := range d.Samples {
		tagOf[s.Name] = s.Tags
	}
	names := make([]string, 0, len(execs))
	for n := range execs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		art, _ := execs[name].(map[string]any)
		rc, _ := art["retrieval_comparison"].(map[string]any)
		if rc == nil {
			continue
		}
		b, _ := rc["baseline"].(map[string]any)
		c, _ := rc["candidate"].(map[string]any)
		bMRR, _ := b["reciprocalRank"].(float64)
		cMRR, _ := c["reciprocalRank"].(float64)
		critOK, _ := rc["critical_ids_ok"].(bool)
		for _, tag := range tagOf[name] {
			st, ok := tags[tag]
			if !ok {
				st = &tagStat{}
				tags[tag] = st
			}
			st.n++
			st.bMRR += bMRR
			st.cMRR += cMRR
			if !critOK {
				st.critMiss++
			}
		}
	}
	fmt.Println("\n=== MRR by tag ===")
	tagNames := make([]string, 0, len(tags))
	for t := range tags {
		tagNames = append(tagNames, t)
	}
	sort.Strings(tagNames)
	for _, t := range tagNames {
		st := tags[t]
		fmt.Printf("%s: n=%.0f baseline=%.3f candidate=%.3f crit_miss=%d\n",
			t, st.n, st.bMRR/st.n, st.cMRR/st.n, st.critMiss)
	}
}
