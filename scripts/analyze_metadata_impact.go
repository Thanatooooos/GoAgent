package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	before := load("testdata/evals/rewrite/v2_24_soft_gate.json")
	after := load("testdata/evals/rewrite/v2_24_no_metadata.json")

	fmt.Println("=== 为什么禁用 metadata 效果不明显 ===\n")

	// 1) How often did metadata channel even run?
	metaRan, metaHit1, metaHitTop5 := 0, 0, 0
	metaOnlyHit := 0 // only metadata had hit among channels
	for _, s := range before.Samples {
		c := before.Artifacts.Executions[s.Name].RetrievalComparison.Candidate
		hasMeta, metaH1 := false, false
		otherH1 := false
		for _, ch := range c.Channels {
			if ch.ChannelName != "metadata_title" {
				if ch.HitAtK["1"] {
					otherH1 = true
				}
				continue
			}
			hasMeta = true
			metaRan++
			if ch.HitAtK["1"] {
				metaHit1++
				metaH1 = true
			}
			if ch.HitAtK["5"] {
				metaHitTop5++
			}
		}
		if hasMeta && metaH1 && !otherH1 {
			metaOnlyHit++
		}
	}
	fmt.Printf("1) metadata 通道参与度 (meta ON, candidate)\n")
	fmt.Printf("   运行次数: %d / %d 样本\n", metaRan, len(before.Samples))
	fmt.Printf("   单通道 Hit@1: %d (%.1f%% of meta runs)\n", metaHit1, pct(metaHit1, metaRan))
	fmt.Printf("   单通道 Hit@5: %d (%.1f%% of meta runs)\n", metaHitTop5, pct(metaHitTop5, metaRan))
	fmt.Printf("   仅 metadata 命中、其他通道未 Hit@1: %d\n\n", metaOnlyHit)

	// 2) How many merged top-5 slots did metadata-origin IDs occupy? (approx via overlap)
	type slotChange struct {
		name       string
		bMerge, aMerge float64
		bIDs, aIDs []string
		metaInBeforeTop5 int
		expected string
	}
	var changes []slotChange
	improved, regressed, unchanged := 0, 0, 0
	for _, s := range before.Samples {
		name := s.Name
		bRC := before.Artifacts.Executions[name].RetrievalComparison
		aRC := after.Artifacts.Executions[name].RetrievalComparison
		bM := bRC.Candidate.ReciprocalRank
		aM := aRC.Candidate.ReciprocalRank
		switch {
		case aM > bM+1e-9:
			improved++
		case aM < bM-1e-9:
			regressed++
		default:
			unchanged++
		}
		exp := ""
		if len(bRC.ExpectedIDs) > 0 {
			exp = bRC.ExpectedIDs[0]
		}
		metaInTop5 := 0
		// count non-expected ids that metadata channel retrieved but weren't in keyword/vector top ranks
		metaIDs := map[string]struct{}{}
		for _, id := range bRC.CandidateRetrievedIDs {
			metaIDs[id] = struct{}{}
		}
		_ = metaIDs
		changes = append(changes, slotChange{
			name: name, bMerge: bM, aMerge: aM,
			bIDs: bRC.CandidateRetrievedIDs, aIDs: aRC.CandidateRetrievedIDs,
			metaInBeforeTop5: metaInTop5, expected: exp,
		})
	}
	fmt.Printf("2) merge MRR 变化分布 (24 样本)\n")
	fmt.Printf("   提升: %d | 不变: %d | 回退: %d\n\n", improved, unchanged, regressed)

	// 3) Remaining merge gap after meta OFF
	mergeGap := oracleGap(after)
	fmt.Printf("3) 禁用 meta 后 merge 与 oracle 差距 (仍是大头)\n")
	fmt.Printf("   merge MRR: %.3f | oracle MRR: %.3f | gap: %.3f\n", mergeGap.mergeMRR, mergeGap.oracleMRR, mergeGap.oracleMRR-mergeGap.mergeMRR)
	fmt.Printf("   merge Hit@1: %.1f%% | oracle Hit@1: %.1f%% | gap: %.1fpp\n\n",
		100*mergeGap.mergeHit1, 100*mergeGap.oracleHit1, 100*(mergeGap.oracleHit1-mergeGap.mergeHit1))

	// 4) Still failing samples - not fixed by meta disable
	fmt.Println("4) 禁用 meta 后仍 merge MRR=0 的样本 (rewrite/merge 其他问题)")
	stillZero := 0
	for _, s := range after.Samples {
		m := after.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		if m > 0 {
			continue
		}
		stillZero++
		rc := after.Artifacts.Executions[s.Name].RetrievalComparison
		bestCh, bestMRR := "", 0.0
		for _, ch := range rc.Candidate.Channels {
			mrr := 0.0
			if ch.FirstRelevantRank > 0 {
				mrr = 1.0 / float64(ch.FirstRelevantRank)
			}
			if mrr > bestMRR {
				bestMRR = mrr
				bestCh = ch.ChannelName
			}
		}
		fmt.Printf("   %s | best[%s] MRR=%.2f\n", s.Name, bestCh, bestMRR)
	}
	fmt.Printf("   合计: %d 样本\n\n", stillZero)

	// 5) Detailed per-sample with expected id position
	fmt.Println("5) 有变化的样本明细")
	for _, s := range before.Samples {
		name := s.Name
		b := before.Artifacts.Executions[name].RetrievalComparison
		a := after.Artifacts.Executions[name].RetrievalComparison
		if b.Candidate.ReciprocalRank == a.Candidate.ReciprocalRank {
			continue
		}
		exp := firstExpected(b.ExpectedIDs)
		fmt.Printf("\n   %s: MRR %.2f -> %.2f\n", name, b.Candidate.ReciprocalRank, a.Candidate.ReciprocalRank)
		fmt.Printf("     expected: %s\n", exp)
		fmt.Printf("     before top5: %s\n", strings.Join(b.CandidateRetrievedIDs, ", "))
		fmt.Printf("     after  top5: %s\n", strings.Join(a.CandidateRetrievedIDs, ", "))
		fmt.Printf("     before rank=%d hit@1=%t | after rank=%d hit@1=%t\n",
			b.Candidate.FirstRelevantRank, b.Candidate.HitAtK["1"],
			a.Candidate.FirstRelevantRank, a.Candidate.HitAtK["1"])
	}

	// 6) Pass rate delta breakdown
	bPass, aPass := countPass(before), countPass(after)
	fmt.Printf("\n6) 通过率 %d/24 -> %d/24 (+%d)\n", bPass, aPass, aPass-bPass)
	fmt.Println("   结论: metadata 是 merge 噪声源之一，但不是主因；主因仍是 RRF merge 未保留 keyword/vector 的单通道命中")
}

type doc struct {
	Samples []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
	} `json:"samples"`
	Artifacts struct {
		Executions map[string]struct {
			RetrievalComparison struct {
				ExpectedIDs           []string `json:"expected_ids"`
				CandidateRetrievedIDs []string `json:"candidate_retrieved_ids"`
				Candidate             struct {
					ReciprocalRank    float64         `json:"reciprocalRank"`
					FirstRelevantRank int             `json:"firstRelevantRank"`
					HitAtK            map[string]bool `json:"hitAtK"`
					Channels          []struct {
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

type gapStats struct {
	mergeMRR, oracleMRR, mergeHit1, oracleHit1 float64
}

func oracleGap(d doc) gapStats {
	var mergeMRR, oracleMRR, mergeHit1, oracleHit1 float64
	var n float64
	for _, s := range d.Samples {
		c := d.Artifacts.Executions[s.Name].RetrievalComparison.Candidate
		if len(c.Channels) == 0 {
			continue
		}
		n++
		mergeMRR += c.ReciprocalRank
		if c.HitAtK["1"] {
			mergeHit1++
		}
		best := 0.0
		bestH1 := false
		for _, ch := range c.Channels {
			mrr := 0.0
			if ch.FirstRelevantRank > 0 {
				mrr = 1.0 / float64(ch.FirstRelevantRank)
			}
			if mrr > best {
				best = mrr
			}
			if ch.HitAtK["1"] {
				bestH1 = true
			}
		}
		oracleMRR += best
		if bestH1 {
			oracleHit1++
		}
	}
	return gapStats{mergeMRR / n, oracleMRR / n, mergeHit1 / n, oracleHit1 / n}
}

func countPass(d doc) int {
	n := 0
	for _, s := range d.Samples {
		if s.Passed {
			n++
		}
	}
	return n
}

func firstExpected(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
