package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type sideMetrics struct {
	mrr, recall1, recall3, recall5, ndcg1, ndcg3, ndcg5 float64
	hit1, hit3, hit5                                       float64
	n                                                       int
}

type channelAgg struct {
	name string
	sideMetrics
}

func main() {
	for _, path := range []string{
		"testdata/evals/rewrite/v2_24_soft_gate.json",
		"testdata/evals/rewrite/latest_run_clean.json",
	} {
		if err := analyzeFile(path); err != nil {
			fmt.Printf("%s: %v\n", path, err)
		}
	}
}

func analyzeFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	start := strings.Index(string(raw), `{"suite"`)
	if start < 0 {
		return fmt.Errorf("no json object")
	}
	var d struct {
		Samples []struct {
			Name string `json:"name"`
		} `json:"samples"`
		Artifacts struct {
			Executions map[string]struct {
				RetrievalComparison struct {
					Baseline  sampleResult `json:"baseline"`
					Candidate sampleResult `json:"candidate"`
				} `json:"retrieval_comparison"`
			} `json:"executions"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(raw[start:], &d); err != nil {
		return err
	}

	label := path
	if strings.Contains(path, "v2_24") {
		label = "V2 24 holdout"
	} else if strings.Contains(path, "latest_run_clean") {
		label = "Original 48"
	}

	fmt.Printf("\n══════════════════════════════════════════════════\n")
	fmt.Printf("  %s — candidate retrieval\n", label)
	fmt.Printf("══════════════════════════════════════════════════\n")

	merged := sideMetrics{}
	channels := map[string]*channelAgg{}
	oracle := sideMetrics{}
	oracleHit1, oracleHit3, oracleHit5 := 0, 0, 0
	mergeWorseMRR, mergeWorseHit1, channelHitMergeMiss := 0, 0, 0
	var regressions []string

	for _, s := range d.Samples {
		rc := d.Artifacts.Executions[s.Name].RetrievalComparison
		c := rc.Candidate
		if c.RetrievedCount == 0 && len(c.Channels) == 0 {
			continue
		}

		merged.n++
		merged.mrr += c.ReciprocalRank
		merged.hit1 += boolF(c.HitAtK["1"])
		merged.hit3 += boolF(c.HitAtK["3"])
		merged.hit5 += boolF(c.HitAtK["5"])
		merged.recall1 += f(c.RecallAtK["1"])
		merged.recall3 += f(c.RecallAtK["3"])
		merged.recall5 += f(c.RecallAtK["5"])
		merged.ndcg1 += f(c.NDCGAtK["1"])
		merged.ndcg3 += f(c.NDCGAtK["3"])
		merged.ndcg5 += f(c.NDCGAtK["5"])

		bestMRR := 0.0
		bestHit1 := false
		bestCh := ""
		for _, ch := range c.Channels {
			chMRR := 0.0
			if ch.FirstRelevantRank > 0 {
				chMRR = 1.0 / float64(ch.FirstRelevantRank)
			}
			if chMRR > bestMRR {
				bestMRR = chMRR
				bestCh = ch.ChannelName
			}
			if ch.HitAtK["1"] {
				bestHit1 = true
			}

			agg, ok := channels[ch.ChannelName]
			if !ok {
				agg = &channelAgg{name: ch.ChannelName}
				channels[ch.ChannelName] = agg
			}
			agg.n++
			agg.mrr += chMRR
			agg.hit1 += boolF(ch.HitAtK["1"])
			agg.hit3 += boolF(ch.HitAtK["3"])
			agg.hit5 += boolF(ch.HitAtK["5"])
		}

		oracle.n++
		oracle.mrr += bestMRR
		anyHit3, anyHit5 := false, false
		if bestHit1 {
			oracleHit1++
		}
		for _, ch := range c.Channels {
			if ch.HitAtK["3"] {
				anyHit3 = true
			}
			if ch.HitAtK["5"] {
				anyHit5 = true
			}
		}
		if anyHit3 {
			oracleHit3++
		}
		if anyHit5 {
			oracleHit5++
		}

		if bestMRR > c.ReciprocalRank+1e-9 {
			mergeWorseMRR++
			regressions = append(regressions, fmt.Sprintf("  %s: merge MRR=%.2f vs best[%s]=%.2f (rank=%d vs merge rank=%d)",
				s.Name, c.ReciprocalRank, bestCh, bestMRR, rankFromMRR(bestMRR), c.FirstRelevantRank))
		}
		if bestHit1 && !c.HitAtK["1"] {
			mergeWorseHit1++
			channelHitMergeMiss++
		}
	}

	n := float64(merged.n)
	printRow("merged (final top-K)", merged, n)
	fmt.Println()
	printHeader()
	names := make([]string, 0, len(channels))
	for name := range channels {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ch := channels[name]
		printRow(name, ch.sideMetrics, float64(ch.n))
		printDelta("  Δ vs merge", merged, ch.sideMetrics, n, float64(ch.n))
	}
	fmt.Println()
	oracle.hit1 = float64(oracleHit1)
	oracle.hit3 = float64(oracleHit3)
	oracle.hit5 = float64(oracleHit5)
	printRow("oracle (best channel / sample)", oracle, float64(oracle.n))
	fmt.Printf("    vs merge: MRR %+.3f | Hit@1 %+.1fpp | Hit@3 %+.1fpp | Hit@5 %+.1fpp\n",
		oracle.mrr/float64(oracle.n)-merged.mrr/n,
		100*(oracle.hit1/float64(oracle.n)-merged.hit1/n),
		100*(oracle.hit3/float64(oracle.n)-merged.hit3/n),
		100*(oracle.hit5/float64(oracle.n)-merged.hit5/n))

	fmt.Printf("\nMerge regression summary (n=%d samples with channels):\n", merged.n)
	fmt.Printf("  merge MRR < best-channel MRR: %d (%.1f%%)\n", mergeWorseMRR, 100*float64(mergeWorseMRR)/n)
	fmt.Printf("  channel Hit@1 but merge miss:  %d (%.1f%%)\n", mergeWorseHit1, 100*float64(mergeWorseHit1)/n)
	if len(regressions) > 0 {
		fmt.Println("\nSamples where merge loses vs best channel:")
		for _, line := range regressions {
			fmt.Println(line)
		}
	}

	// baseline vs candidate merge for v2 context
	fmt.Printf("\n--- Baseline merged (raw query, no rewrite) ---\n")
	base := sideMetrics{}
	for _, s := range d.Samples {
		b := d.Artifacts.Executions[s.Name].RetrievalComparison.Baseline
		base.n++
		base.mrr += b.ReciprocalRank
		base.hit1 += boolF(b.HitAtK["1"])
		base.hit3 += boolF(b.HitAtK["3"])
		base.hit5 += boolF(b.HitAtK["5"])
	}
	printRow("baseline merged", base, float64(base.n))
	fmt.Printf("  rewrite uplift: MRR %+.3f | Hit@1 %+.1f%%\n",
		merged.mrr/n-base.mrr/float64(base.n),
		100*(merged.hit1/n-base.hit1/float64(base.n)))

	return nil
}

func printHeader() {
	fmt.Printf("%-28s %6s %7s %7s %7s %7s %7s %7s\n",
		"source", "MRR", "Hit@1", "Hit@3", "Hit@5", "R@1", "NDCG@1", "n")
	fmt.Println(strings.Repeat("-", 88))
}

func printRow(label string, m sideMetrics, n float64) {
	if n == 0 {
		return
	}
	fmt.Printf("%-28s %6.3f %6.1f%% %6.1f%% %6.1f%% %6.3f %6.3f %6.0f\n",
		label, m.mrr/n, 100*m.hit1/n, 100*m.hit3/n, 100*m.hit5/n,
		m.recall1/n, m.ndcg1/n, n)
}

func printDelta(label string, merge, other sideMetrics, mergeN, otherN float64) {
	if mergeN == 0 || otherN == 0 {
		return
	}
	fmt.Printf("%-28s %6s %7s %7s %7s\n", label, "MRR",
		"Hit@1", "Hit@3", "Hit@5")
	fmt.Printf("%-28s %+.3f %+.1fpp %+.1fpp %+.1fpp\n", "",
		other.mrr/otherN-merge.mrr/mergeN,
		100*(other.hit1/otherN-merge.hit1/mergeN),
		100*(other.hit3/otherN-merge.hit3/mergeN),
		100*(other.hit5/otherN-merge.hit5/mergeN))
}

type sampleResult struct {
	ReciprocalRank    float64         `json:"reciprocalRank"`
	FirstRelevantRank int             `json:"firstRelevantRank"`
	HitAtK            map[string]bool `json:"hitAtK"`
	RecallAtK         map[string]float64 `json:"recallAtK"`
	NDCGAtK           map[string]float64 `json:"ndcgAtK"`
	RetrievedCount    int             `json:"retrievedCount"`
	Channels          []struct {
		ChannelName       string          `json:"channelName"`
		HitAtK            map[string]bool `json:"hitAtK"`
		FirstRelevantRank int             `json:"firstRelevantRank"`
	} `json:"channels"`
}

func boolF(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func f(v float64) float64 { return v }

func rankFromMRR(mrr float64) int {
	if mrr <= 0 {
		return 0
	}
	return int(1.0/mrr + 0.5)
}
