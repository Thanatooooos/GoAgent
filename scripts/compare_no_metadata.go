package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	type result struct {
		label string
		a     aggStats
		merge mergeStats
		chs   map[string]mergeStats
		oracle mergeStats
		metaN int
	}
	var results []result
	for _, item := range []struct{ label, path string }{
		{"before (meta ON)", "testdata/evals/rewrite/v2_24_soft_gate.json"},
		{"after (meta OFF)", "testdata/evals/rewrite/v2_24_no_metadata.json"},
	} {
		d := load(item.path)
		m, chs, oracle, metaN := channelMerge(d)
		results = append(results, result{
			label: item.label, a: aggregate(d), merge: m, chs: chs, oracle: oracle, metaN: metaN,
		})
	}

	for _, r := range results {
		fmt.Printf("\n=== %s ===\n", r.label)
		fmt.Printf("pass %d/%d\n", r.a.pass, r.a.total)
		fmt.Printf("aggregate MRR: baseline %.3f -> candidate %.3f (delta %+.3f)\n",
			r.a.bMRR, r.a.cMRR, r.a.cMRR-r.a.bMRR)
		fmt.Printf("Hit@1: %.1f%% -> %.1f%% | Hit@5: %.1f%% -> %.1f%%\n",
			100*r.a.bH1, 100*r.a.cH1, 100*r.a.bH5, 100*r.a.cH5)
		n := float64(r.merge.n)
		fmt.Printf("candidate merge (n=%d): MRR %.3f Hit@1 %.1f%% Hit@3 %.1f%% Hit@5 %.1f%%\n",
			r.merge.n, r.merge.mrr/n, 100*r.merge.h1/n, 100*r.merge.h3/n, 100*r.merge.h5/n)
		for _, name := range []string{"keyword", "metadata_title", "vector_global"} {
			st, ok := r.chs[name]
			if !ok {
				continue
			}
			sn := float64(st.n)
			fmt.Printf("  channel %s (n=%d): MRR %.3f Hit@1 %.1f%%\n", name, st.n, st.mrr/sn, 100*st.h1/sn)
		}
		on := float64(r.oracle.n)
		fmt.Printf("  oracle: MRR %.3f Hit@1 %.1f%%\n", r.oracle.mrr/on, 100*r.oracle.h1/on)
		fmt.Printf("  metadata_title channel appearances: %d\n", r.metaN)
	}

	if len(results) == 2 {
		b, a := results[0], results[1]
		fmt.Printf("\n=== DELTA (meta OFF - meta ON) ===\n")
		fmt.Printf("pass rate: %d/%d -> %d/%d (%+d)\n", b.a.pass, b.a.total, a.a.pass, a.a.total, a.a.pass-b.a.pass)
		fmt.Printf("candidate MRR: %+.3f | Hit@1: %+.1fpp | Hit@5: %+.1fpp\n",
			a.a.cMRR-b.a.cMRR, 100*(a.a.cH1-b.a.cH1), 100*(a.a.cH5-b.a.cH5))
		bn, an := float64(b.merge.n), float64(a.merge.n)
		fmt.Printf("merge MRR: %+.3f | merge Hit@1: %+.1fpp\n",
			a.merge.mrr/an-b.merge.mrr/bn, 100*(a.merge.h1/an-b.merge.h1/bn))
	}

	// per-sample diffs
	fmt.Println("\n=== Per-sample candidate MRR changes ===")
	before := load("testdata/evals/rewrite/v2_24_soft_gate.json")
	after := load("testdata/evals/rewrite/v2_24_no_metadata.json")
	for _, s := range before.Samples {
		b := before.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		af := after.Artifacts.Executions[s.Name].RetrievalComparison.Candidate.ReciprocalRank
		if af != b {
			fmt.Printf("  %s: %.2f -> %.2f (%+.2f)\n", s.Name, b, af, af-b)
		}
	}
}

type doc struct {
	Samples []struct{ Name string } `json:"samples"`
	Aggregate struct {
		PassedCount int `json:"passed_count"`
		SampleCount int `json:"sample_count"`
		Metrics     map[string]any `json:"metrics"`
	} `json:"aggregate"`
	Artifacts struct {
		Executions map[string]struct {
			RetrievalComparison struct {
				Candidate struct {
					ReciprocalRank float64 `json:"reciprocalRank"`
					HitAtK         map[string]bool `json:"hitAtK"`
					Channels       []struct {
						ChannelName       string `json:"channelName"`
						FirstRelevantRank int    `json:"firstRelevantRank"`
						HitAtK            map[string]bool `json:"hitAtK"`
					} `json:"channels"`
				} `json:"candidate"`
			} `json:"retrieval_comparison"`
		} `json:"executions"`
	} `json:"artifacts"`
}

type aggStats struct {
	pass, total int
	bMRR, cMRR  float64
	bH1, cH1    float64
	bH5, cH5    float64
}

type mergeStats struct {
	n          int
	mrr, h1, h3, h5 float64
}

func load(path string) doc {
	raw, _ := os.ReadFile(path)
	start := strings.Index(string(raw), `{"suite"`)
	var d doc
	_ = json.Unmarshal(raw[start:], &d)
	return d
}

func aggregate(d doc) aggStats {
	m := d.Aggregate.Metrics
	bh, _ := m["baseline_hit_at_k"].(map[string]any)
	ch, _ := m["candidate_hit_at_k"].(map[string]any)
	return aggStats{
		pass: d.Aggregate.PassedCount, total: d.Aggregate.SampleCount,
		bMRR: num(m["baseline_mrr"]), cMRR: num(m["candidate_mrr"]),
		bH1: num(bh["1"]), cH1: num(ch["1"]),
		bH5: num(bh["5"]), cH5: num(ch["5"]),
	}
}

func channelMerge(d doc) (mergeStats, map[string]mergeStats, mergeStats, int) {
	merged := mergeStats{}
	chs := map[string]mergeStats{}
	oracle := mergeStats{}
	metaN := 0
	for _, s := range d.Samples {
		c := d.Artifacts.Executions[s.Name].RetrievalComparison.Candidate
		if len(c.Channels) == 0 {
			continue
		}
		merged.n++
		merged.mrr += c.ReciprocalRank
		merged.h1 += boolF(c.HitAtK["1"])
		merged.h3 += boolF(c.HitAtK["3"])
		merged.h5 += boolF(c.HitAtK["5"])
		best := 0.0
		bestHit1 := false
		for _, ch := range c.Channels {
			st := chs[ch.ChannelName]
			st.n++
			mrr := 0.0
			if ch.FirstRelevantRank > 0 {
				mrr = 1.0 / float64(ch.FirstRelevantRank)
			}
			st.mrr += mrr
			st.h1 += boolF(ch.HitAtK["1"])
			chs[ch.ChannelName] = st
			if ch.ChannelName == "metadata_title" {
				metaN++
			}
			if mrr > best {
				best = mrr
			}
			if ch.HitAtK["1"] {
				bestHit1 = true
			}
		}
		oracle.n++
		oracle.mrr += best
		if bestHit1 {
			oracle.h1++
		}
	}
	return merged, chs, oracle, metaN
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func boolF(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
