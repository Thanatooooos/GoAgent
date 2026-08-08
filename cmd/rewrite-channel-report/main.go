package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

type rewriteCheckpoint struct {
	BaselineRetrieval  *rageval.SampleResult `json:"baseline_retrieval"`
	CandidateRetrieval *rageval.SampleResult `json:"candidate_retrieval"`
}

func main() {
	artifactDir := flag.String("artifact-dir", "testdata/evals/rewrite/runs/full48/artifacts", "rewrite checkpoint directory")
	outputPath := flag.String("output", "", "optional path to write channel report JSON")
	flag.Parse()

	baselineResults := make([]rageval.SampleResult, 0)
	candidateResults := make([]rageval.SampleResult, 0)

	entries, err := os.ReadDir(*artifactDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read artifact dir: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(*artifactDir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", entry.Name(), err)
			os.Exit(1)
		}
		var checkpoint rewriteCheckpoint
		if err := json.Unmarshal(raw, &checkpoint); err != nil {
			fmt.Fprintf(os.Stderr, "decode %s: %v\n", entry.Name(), err)
			os.Exit(1)
		}
		if checkpoint.BaselineRetrieval != nil {
			baselineResults = append(baselineResults, *checkpoint.BaselineRetrieval)
		}
		if checkpoint.CandidateRetrieval != nil {
			candidateResults = append(candidateResults, *checkpoint.CandidateRetrieval)
		}
	}

	ks := []int{1, 3, 5}
	report := rageval.BuildRewriteRetrievalChannelReport(baselineResults, candidateResults, ks)
	if report == nil {
		fmt.Fprintln(os.Stderr, "no channel metrics found in checkpoints")
		os.Exit(1)
	}

	fmt.Printf("Rewrite per-channel report from %d checkpoints\n", len(baselineResults))
	fmt.Printf("artifact-dir: %s\n\n", *artifactDir)

	printChannels("baseline", report["baseline_channels"])
	printChannels("candidate", report["candidate_channels"])

	fmt.Println("channel_mrr_uplift:")
	uplift, _ := report["channel_mrr_uplift"].(map[string]float64)
	names := make([]string, 0, len(uplift))
	for name := range uplift {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  %-16s %+.3f\n", name, uplift[name])
	}

	fmt.Println("\nchannel_hit_at_k_uplift:")
	hitUplift, _ := report["channel_hit_at_k_uplift"].(map[string]map[int]float64)
	for _, name := range names {
		perK := hitUplift[name]
		fmt.Printf("  %-16s Hit@1=%+.1fpp Hit@3=%+.1fpp Hit@5=%+.1fpp\n",
			name,
			perK[1]*100,
			perK[3]*100,
			perK[5]*100,
		)
	}

	if strings.TrimSpace(*outputPath) != "" {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal report: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*outputPath, payload, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nWrote %s\n", *outputPath)
	}
}

func printChannels(label string, raw any) {
	channels, ok := raw.([]rageval.ChannelAggregateMetrics)
	if !ok || len(channels) == 0 {
		fmt.Printf("%s_channels: none\n\n", label)
		return
	}
	fmt.Printf("%s_channels:\n", label)
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].ChannelName < channels[j].ChannelName
	})
	for _, channel := range channels {
		fmt.Printf("  %-16s n=%3d MRR=%.3f Hit@1=%.1f%% Hit@3=%.1f%% Hit@5=%.1f%% unique=%d overlap=%d\n",
			channel.ChannelName,
			channel.SampleCount,
			channel.MRR,
			channel.HitRateAtK[1]*100,
			channel.HitRateAtK[3]*100,
			channel.HitRateAtK[5]*100,
			channel.UniqueHitCount,
			channel.OverlapHitCount,
		)
	}
	fmt.Println()
}
