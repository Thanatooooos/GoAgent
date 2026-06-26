// analyze_eval extracts a comprehensive multi-dimensional report from an
// eval-runner suite output.
//
// Usage:
//
//	go run ./scripts/analyze_eval.go testdata/evals/rewrite/final_48.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type evalOutput struct {
	Suite      string `json:"suite"`
	RunMeta    struct {
		RunAt       string `json:"run_at"`
		SampleSetID string `json:"sample_set_id"`
	} `json:"run_metadata"`
	Samples   []sampleResult   `json:"samples"`
	Aggregate aggregateSection `json:"aggregate"`
	Artifacts struct {
		Executions map[string]sampleArtifact `json:"executions"`
	} `json:"artifacts"`
}

type sampleResult struct {
	Name    string   `json:"name"`
	Tags    []string `json:"tags"`
	Passed  bool     `json:"passed"`
	Scores  map[string]any
}

type aggregateSection struct {
	PassRate            float64 `json:"pass_rate"`
	CriticalFailureRate float64 `json:"critical_failure_rate"`
	ByTag               []tagAggregate
	Metrics             map[string]any `json:"metrics"`
}

type tagAggregate struct {
	Tag     string  `json:"tag"`
	PassRate float64 `json:"passRate"`
	Count    int     `json:"count"`
}

type sampleArtifact struct {
	Query               string                         `json:"query"`
	RewrittenQuery      string                         `json:"rewritten_query"`
	SubQuestions        []string                       `json:"sub_questions"`
	NeedRetrieval       bool                           `json:"need_retrieval"`
	RetrievalComparison *retrievalComparisonArtifact   `json:"retrieval_comparison,omitempty"`
	RetrievalExpectation map[string]any                `json:"retrieval_expectation,omitempty"`
}

type retrievalComparisonArtifact struct {
	Baseline    sideArtifact `json:"baseline"`
	Candidate   sideArtifact `json:"candidate"`
	Passed      bool         `json:"passed"`
	BaselinePipeline map[string]any `json:"baseline_pipeline,omitempty"`
	CandidatePipeline map[string]any `json:"candidate_pipeline,omitempty"`
}

type sideArtifact struct {
	Channels         []channelArtifact `json:"channels"`
	ExpectedIDs      []string          `json:"expectedIds"`
	FirstRelevantRank int              `json:"firstRelevantRank"`
	HitAtK           map[string]bool   `json:"hitAtK"`
	ReciprocalRank   float64           `json:"reciprocalRank"`
}

type channelArtifact struct {
	ChannelName      string         `json:"channelName"`
	RetrievedCount   float64        `json:"retrievedCount"`
	FirstRelevantRank int           `json:"firstRelevantRank"`
	HitAtK           map[string]bool `json:"hitAtK"`
	OverlapHitCount  float64        `json:"overlapHitCount"`
	UniqueHitCount   float64        `json:"uniqueHitCount"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/analyze_eval.go <eval-output.json>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	var out evalOutput
	if err := json.Unmarshal(data, &out); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("  EVALUATION ANALYSIS: %s\n", out.RunMeta.SampleSetID)
	fmt.Printf("  Run at: %s | Samples: %d\n", out.RunMeta.RunAt, len(out.Samples))
	fmt.Println(strings.Repeat("=", 72))

	// ── 1. Overall: Baseline vs Candidate ──
	printOverall(&out)

	// ── 2. Per-channel breakdown ──
	printChannelBreakdown(&out)

	// ── 3. RRF → Rerank pipeline ──
	printPipelineAnalysis(&out)

	// ── 4. Per-tag pass rate ──
	printTagBreakdown(&out)
}

func printOverall(out *evalOutput) {
	m := out.Aggregate.Metrics
	fmt.Println("\n── 1. OVERALL: Baseline vs Candidate ──")
	fmt.Printf("%-20s %10s %10s %10s\n", "Metric", "Baseline", "Candidate", "Uplift")
	fmt.Println(strings.Repeat("-", 52))
	for _, kv := range []struct{ label, base, cand string }{
		{"MRR", "baseline_mrr", "candidate_mrr"},
		{"Hit@1", "baseline_hit_at_k.1", "candidate_hit_at_k.1"},
		{"Hit@3", "baseline_hit_at_k.3", "candidate_hit_at_k.3"},
		{"Hit@5", "baseline_hit_at_k.5", "candidate_hit_at_k.5"},
		{"Recall@1", "baseline_recall_at_k.1", "candidate_recall_at_k.1"},
		{"Recall@3", "baseline_recall_at_k.3", "candidate_recall_at_k.3"},
		{"Recall@5", "baseline_recall_at_k.5", "candidate_recall_at_k.5"},
		{"NDCG@1", "baseline_ndcg_at_k.1", "candidate_ndcg_at_k.1"},
		{"NDCG@3", "baseline_ndcg_at_k.3", "candidate_ndcg_at_k.3"},
		{"NDCG@5", "baseline_ndcg_at_k.5", "candidate_ndcg_at_k.5"},
	} {
		bv := nestedFloat(m, kv.base)
		cv := nestedFloat(m, kv.cand)
		fmt.Printf("%-20s %10.4f %10.4f %+10.4f\n", kv.label, bv, cv, cv-bv)
	}
	fmt.Printf("\nMRR uplift: %+.4f\n", nestedFloat(m, "mrr_uplift"))
	fmt.Printf("Regression count: %.0f\n", nestedFloat(m, "retrieval_regression_count"))
}

func printChannelBreakdown(out *evalOutput) {
	fmt.Println("\n── 2. PER-CHANNEL BREAKDOWN ──")

	type chanStat struct {
		name       string
		samples    int
		totalChunks float64
		hit1        int
		hit3        int
		hit5        int
	}

	for _, sideName := range []string{"baseline", "candidate"} {
		stats := map[string]*chanStat{}
		for _, art := range out.Artifacts.Executions {
			if art.RetrievalComparison == nil {
				continue
			}
			var side sideArtifact
			if sideName == "baseline" {
				side = art.RetrievalComparison.Baseline
			} else {
				side = art.RetrievalComparison.Candidate
			}
			for _, ch := range side.Channels {
				name := ch.ChannelName
				if stats[name] == nil {
					stats[name] = &chanStat{name: name}
				}
				s := stats[name]
				s.samples++
				s.totalChunks += ch.RetrievedCount
				if ch.HitAtK["1"] {
					s.hit1++
				}
				if ch.HitAtK["3"] {
					s.hit3++
				}
				if ch.HitAtK["5"] {
					s.hit5++
				}
			}
		}

		fmt.Printf("\n  [%s]\n", strings.ToUpper(sideName))
		fmt.Printf("  %-20s %8s %8s %8s %8s %8s\n", "Channel", "Samples", "AvgChunk", "Hit@1%", "Hit@3%", "Hit@5%")
		fmt.Println("  " + strings.Repeat("-", 64))
		keys := make([]string, 0, len(stats))
		for k := range stats {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := stats[k]
			fmt.Printf("  %-20s %8d %8.1f %7.1f%% %7.1f%% %7.1f%%\n",
				s.name, s.samples, s.totalChunks/float64(s.samples),
				percent(s.hit1, s.samples), percent(s.hit3, s.samples), percent(s.hit5, s.samples))
		}
	}
}

func printPipelineAnalysis(out *evalOutput) {
	fmt.Println("\n── 3. RRF → RERANK PIPELINE ──")

	type pipeStat struct {
		samples       int
		rerankApplied int
		preCount      float64
		finalCount    float64
		idsChanged    int // pre != final
	}
	stats := map[string]*pipeStat{"baseline": {}, "candidate": {}}

	for _, art := range out.Artifacts.Executions {
		if art.RetrievalComparison == nil {
			continue
		}
		for _, sideName := range []string{"baseline", "candidate"} {
			var pipeline map[string]any
			if sideName == "baseline" {
				pipeline = art.RetrievalComparison.BaselinePipeline
			} else {
				pipeline = art.RetrievalComparison.CandidatePipeline
			}
			if pipeline == nil {
				continue
			}
			s := stats[sideName]
			s.samples++
			if applied, ok := pipeline["rerank_applied"].(bool); ok && applied {
				s.rerankApplied++
			}
			preIDs, _ := pipeline["pre_rerank_chunk_ids"].([]any)
			finalIDs, _ := pipeline["final_chunk_ids"].([]any)
			s.preCount += float64(len(preIDs))
			s.finalCount += float64(len(finalIDs))

			// Check if rerank changed the order
			if len(preIDs) > 0 && len(finalIDs) > 0 {
				pre0 := fmt.Sprint(preIDs[0])
				final0 := fmt.Sprint(finalIDs[0])
				if pre0 != final0 {
					s.idsChanged++
				}
			}
		}
	}

	for _, sideName := range []string{"baseline", "candidate"} {
		s := stats[sideName]
		if s.samples == 0 {
			continue
		}
		fmt.Printf("\n  [%s]\n", strings.ToUpper(sideName))
		fmt.Printf("  Samples with pipeline trace: %d\n", s.samples)
		fmt.Printf("  Rerank applied:              %d (%.1f%%)\n", s.rerankApplied, percent(s.rerankApplied, s.samples))
		fmt.Printf("  Avg pre-rerank (RRF) chunks: %.1f\n", s.preCount/float64(s.samples))
		fmt.Printf("  Avg final chunks:            %.1f\n", s.finalCount/float64(s.samples))
		fmt.Printf("  Samples where rerank changed top-1: %d (%.1f%%)\n", s.idsChanged, percent(s.idsChanged, s.samples))

		// Show a concrete example
		if s.idsChanged > 0 {
			for _, art := range out.Artifacts.Executions {
				if art.RetrievalComparison == nil {
					continue
				}
				var pipeline map[string]any
				if sideName == "baseline" {
					pipeline = art.RetrievalComparison.BaselinePipeline
				} else {
					pipeline = art.RetrievalComparison.CandidatePipeline
				}
				preIDs, _ := pipeline["pre_rerank_chunk_ids"].([]any)
				finalIDs, _ := pipeline["final_chunk_ids"].([]any)
				if len(preIDs) > 0 && len(finalIDs) > 0 && fmt.Sprint(preIDs[0]) != fmt.Sprint(finalIDs[0]) {
					fmt.Printf("\n  Example (rerank changed top-1):\n")
					fmt.Printf("    Query: %s\n", art.Query)
					fmt.Printf("    Pre-rerank top-3:  %v\n", truncIDs(preIDs, 3))
					fmt.Printf("    Post-rerank top-3: %v\n", truncIDs(finalIDs, 3))
					break
				}
			}
		}
	}
}

func printTagBreakdown(out *evalOutput) {
	fmt.Println("\n── 4. PER-TAG PASS RATE ──")
	fmt.Printf("  %-20s %8s %8s\n", "Tag", "Samples", "Pass%")
	fmt.Println("  " + strings.Repeat("-", 40))
	for _, t := range out.Aggregate.ByTag {
		fmt.Printf("  %-20s %8d %7.1f%%\n", t.Tag, t.Count, t.PassRate*100)
	}
}

func nestedFloat(m map[string]any, key string) float64 {
	parts := strings.Split(key, ".")
	if len(parts) == 2 {
		if inner, ok := m[parts[0]].(map[string]any); ok {
			if v, ok := inner[parts[1]].(float64); ok {
				return v
			}
		}
	}
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func percent(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

func truncIDs(ids []any, n int) []string {
	out := make([]string, 0, n)
	for i, id := range ids {
		if i >= n {
			break
		}
		out = append(out, fmt.Sprint(id))
	}
	return out
}
