package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	on := load("testdata/evals/rewrite/v2_24_soft_gate.json")
	off := load("testdata/evals/rewrite/v2_24_no_metadata.json")

	fmt.Println("=== 禁用 meta 为何提升不明显 ===\n")

	// headline numbers
	printHeadline("meta ON", on)
	printHeadline("meta OFF", off)

	onMerge, onOracle := avgMergeOracle(on)
	offMerge, offOracle := avgMergeOracle(off)

	fmt.Println("\n--- 预期 vs 实际 ---")
	fmt.Printf("之前分析的 merge↔oracle 差距 (meta ON):  %.3f MRR\n", onOracle-onMerge)
	fmt.Printf("禁用 meta 后 merge 实际提升:            %+.3f MRR\n", offMerge-onMerge)
	fmt.Printf("禁用 meta 后 merge↔oracle 仍差:         %.3f MRR  ← 大头还在\n", offOracle-offMerge)
	fmt.Printf("meta 通道本身平均 MRR 只有 ~0.22，从不是 oracle 主力\n")

	fmt.Println("\n--- merge 丢失的 9 条 (meta ON)，根因分类 ---")
	type row struct {
		name, bestCh string
		merge, best  float64
		bestRank     int
	}
	var rows []row
	for _, s := range on.Samples {
		rc := on.Artifacts.Executions[s.Name].RetrievalComparison
		c := rc.Candidate
		if len(c.Channels) == 0 {
			continue
		}
		bestCh, bestMRR, bestRank := "", 0.0, 0
		for _, ch := range c.Channels {
			if ch.FirstRelevantRank <= 0 {
				continue
			}
			m := 1.0 / float64(ch.FirstRelevantRank)
			if m > bestMRR {
				bestMRR, bestCh, bestRank = m, ch.ChannelName, ch.FirstRelevantRank
			}
		}
		if bestMRR <= c.ReciprocalRank+1e-9 {
			continue
		}
		rows = append(rows, row{s.Name, bestCh, c.ReciprocalRank, bestMRR, bestRank})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].merge < rows[j].merge })

	kwVec, metaOracle := 0, 0
	for _, r := range rows {
		cat := "RRF 融合丢 hit（keyword/vector 单路已命中）"
		if r.bestCh == "metadata_title" {
			cat = "oracle 靠 metadata（禁用后反而可能变差）"
			metaOracle++
		} else {
			kwVec++
		}
		fmt.Printf("  %-35s merge=%.2f  best=%s rank=%d (MRR=%.2f)  → %s\n",
			r.name, r.merge, r.bestCh, r.bestRank, r.best, cat)
	}
	fmt.Printf("\n汇总: %d 条是 keyword/vector 单路命中但 merge 丢 | %d 条 oracle 靠 metadata\n", kwVec, metaOracle)
	fmt.Println("→ 禁用 meta 只能修「meta 噪声挤占 top-5」，修不了 RRF 丢 keyword/vector 命中")

	fmt.Println("\n--- meta OFF 后仍 critical miss (4条) ---")
	for _, s := range off.Samples {
		rc := off.Artifacts.Executions[s.Name].RetrievalComparison
		if rc.CriticalIDsOK {
			continue
		}
		c := rc.Candidate
		bestCh, rank := "", 0
		for _, ch := range c.Channels {
			if ch.FirstRelevantRank > 0 && (rank == 0 || ch.FirstRelevantRank < rank) {
				rank, bestCh = ch.FirstRelevantRank, ch.ChannelName
			}
		}
		fmt.Printf("  %-35s merge MRR=%.2f  单路 %s rank=%d  top5=%v\n",
			s.Name, c.ReciprocalRank, bestCh, rank, trunc(rc.CandidateRetrievedIDs, 3))
	}

	fmt.Println("\n--- 禁用 meta 有收益的样本 (MRR 提升) ---")
	for _, s := range on.Samples {
		b := on.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		a := off.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		if a > b+1e-9 {
			fmt.Printf("  %s: %.2f → %.2f (%+.2f)\n", s.Name, b, a, a-b)
		}
	}
	fmt.Println("\n--- 禁用 meta 反噬的样本 ---")
	for _, s := range on.Samples {
		b := on.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		a := off.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		if a < b-1e-9 {
			fmt.Printf("  %s: %.2f → %.2f (%+.2f)  ← 之前靠 metadata 撑着\n", s.Name, b, a, a-b)
		}
	}
}

type doc struct {
	Samples []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
	} `json:"samples"`
	Artifacts struct {
		Executions map[string]struct {
			RetrievalComparison struct {
				CriticalIDsOK         bool     `json:"critical_ids_ok"`
				CandidateRetrievedIDs []string `json:"candidate_retrieved_ids"`
				Candidate             struct {
					ReciprocalRank float64 `json:"reciprocalRank"`
					Channels       []struct {
						ChannelName       string          `json:"channelName"`
						FirstRelevantRank int             `json:"firstRelevantRank"`
						HitAtK            map[string]bool `json:"hitAtK"`
					} `json:"channels"`
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

func avgMergeOracle(d doc) (merge, oracle float64) {
	n := 0
	for _, s := range d.Samples {
		c := d.Artifacts.Executions[s.Name].RetrievalComparison.Candidate
		if len(c.Channels) == 0 {
			continue
		}
		n++
		merge += c.ReciprocalRank
		best := 0.0
		for _, ch := range c.Channels {
			if ch.FirstRelevantRank > 0 {
				m := 1.0 / float64(ch.FirstRelevantRank)
				if m > best {
					best = m
				}
			}
		}
		oracle += best
	}
	if n == 0 {
		return 0, 0
	}
	return merge / float64(n), oracle / float64(n)
}

func printHeadline(label string, d doc) {
	m, o := avgMergeOracle(d)
	pass := 0
	for _, s := range d.Samples {
		if s.Passed {
			pass++
		}
	}
	fmt.Printf("[%s] pass=%d/%d | merge MRR=%.3f | oracle MRR=%.3f | gap=%.3f\n",
		label, pass, len(d.Samples), m, o, o-m)
}

func trunc(ids []string, n int) []string {
	if len(ids) <= n {
		return ids
	}
	return ids[:n]
}
