package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
)

type sampleFile struct {
	Samples []rageval.Sample `json:"samples"`
}

func main() {
	inputPath := flag.String("input", "", "path to a JSON file containing retrieval evaluation samples")
	kValues := flag.String("k", "1,3,5", "comma-separated K values, e.g. 1,3,5")
	execute := flag.Bool("execute", false, "execute real retrieve requests for each sample before evaluation")
	configDir := flag.String("config-dir", "configs", "config directory used with -execute")
	jsonOutput := flag.Bool("json", false, "print evaluation summary as JSON")
	outputPath := flag.String("output", "", "write evaluation summary to a file instead of stdout")
	rerankModel := flag.String("rerank-model", "", "optional rerank model override, e.g. qwen3-rerank or rerank-noop")
	vectorTopKMultiplier := flag.Int("vector-topk-multiplier", 0, "optional override for rag.search.channels.vector-global.top-k-multiplier")
	searchModeOverride := flag.String("search-mode", "", "optional retrieval mode override: semantic, keyword, hybrid, auto")
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/retrieve-eval -input <samples.json> [-k 1,3,5] [-json] [-output result.json]")
		os.Exit(1)
	}

	ks, err := parseKs(*kValues)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ks failed: %v\n", err)
		os.Exit(1)
	}

	samples, err := loadSamples(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load samples failed: %v\n", err)
		os.Exit(1)
	}
	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "no samples found")
		os.Exit(1)
	}

	if *execute {
		applyExperimentOverrides(strings.TrimSpace(*rerankModel), *vectorTopKMultiplier)
		if err := config.LoadConfig(*configDir); err != nil {
			fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
			os.Exit(1)
		}
		runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "build rag runtime failed: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = runtime.Close() }()

		if err := executeSamples(context.Background(), runtime, samples, strings.TrimSpace(*searchModeOverride)); err != nil {
			fmt.Fprintf(os.Stderr, "execute retrieval samples failed: %v\n", err)
			os.Exit(1)
		}
	}

	summary, err := rageval.Evaluate(samples, ks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evaluate samples failed: %v\n", err)
		os.Exit(1)
	}

	if err := emitSummary(summary, *jsonOutput, strings.TrimSpace(*outputPath)); err != nil {
		fmt.Fprintf(os.Stderr, "emit summary failed: %v\n", err)
		os.Exit(1)
	}
}

func executeSamples(ctx context.Context, runtime *ragbootstrap.Runtime, samples []rageval.Sample, searchModeOverride string) error {
	if runtime == nil || runtime.Retrieve == nil {
		return fmt.Errorf("rag retrieve runtime is required")
	}

	for i := range samples {
		searchMode := strings.TrimSpace(samples[i].SearchMode)
		if searchModeOverride != "" {
			searchMode = searchModeOverride
		}
		request := ragretrieve.Request{
			UserID:           strings.TrimSpace(samples[i].UserID),
			Query:            strings.TrimSpace(samples[i].Query),
			KnowledgeBaseIDs: append([]string(nil), samples[i].KnowledgeBaseIDs...),
			SearchMode:       searchMode,
			TopK:             samples[i].TopK,
		}
		result, err := runtime.Retrieve.Retrieve(ctx, request)
		if err != nil {
			return fmt.Errorf("execute sample %q: %w", samples[i].Name, err)
		}

		samples[i].Retrieved = retrievedItemsFromChunks(result.Chunks)
		samples[i].ChannelRetrieved = channelRetrievedFromResult(result)
	}
	return nil
}

func retrievedItemsFromChunks(chunks []convention.RetrievedChunk) []rageval.RetrievedItem {
	retrieved := make([]rageval.RetrievedItem, 0, len(chunks))
	for _, chunk := range chunks {
		retrieved = append(retrieved, rageval.RetrievedItem{
			ChunkID:    chunk.ID,
			DocumentID: chunk.DocumentID,
			Metadata:   chunk.Metadata,
			Score:      float64(chunk.Score),
		})
	}
	return retrieved
}

func channelRetrievedFromResult(result ragretrieve.Result) map[string][]rageval.RetrievedItem {
	if len(result.ChannelRetrieved) == 0 {
		return nil
	}
	channelRetrieved := make(map[string][]rageval.RetrievedItem, len(result.ChannelRetrieved))
	for channel, chunks := range result.ChannelRetrieved {
		channelRetrieved[channel] = retrievedItemsFromChunks(chunks)
	}
	return channelRetrieved
}

func loadSamples(path string) ([]rageval.Sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wrapped sampleFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Samples) > 0 {
		return wrapped.Samples, nil
	}

	var plain []rageval.Sample
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	return plain, nil
}

func parseKs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid k value %q", part)
		}
		result = append(result, k)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one k value is required")
	}
	slices.Sort(result)
	return result, nil
}

func applyExperimentOverrides(rerankModel string, vectorTopKMultiplier int) {
	if rerankModel != "" {
		_ = os.Setenv("AI_RERANK_DEFAULT_MODEL", rerankModel)
	}
	if vectorTopKMultiplier > 0 {
		_ = os.Setenv("RAG_SEARCH_CHANNELS_VECTOR_GLOBAL_TOP_K_MULTIPLIER", strconv.Itoa(vectorTopKMultiplier))
	}
}

func emitSummary(summary rageval.Summary, jsonOutput bool, outputPath string) error {
	var (
		data []byte
		err  error
	)
	if jsonOutput {
		data, err = marshalSummaryJSON(summary)
	} else {
		data = []byte(renderSummaryText(summary))
	}
	if err != nil {
		return err
	}

	if outputPath == "" {
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "summary written to %s\n", outputPath)
	return nil
}

func marshalSummaryJSON(summary rageval.Summary) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderSummaryText(summary rageval.Summary) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "samples=%d mrr=%.4f\n", summary.Overall.SampleCount, summary.Overall.MRR)
	for _, k := range summary.Ks {
		fmt.Fprintf(&buf, "hit@%d=%.4f recall@%d=%.4f ndcg@%d=%.4f\n",
			k, summary.Overall.HitRateAtK[k],
			k, summary.Overall.AverageRecallAtK[k],
			k, summary.Overall.AverageNDCGAtK[k])
	}
	fmt.Fprintln(&buf)

	if len(summary.ByTag) > 0 {
		fmt.Fprintln(&buf, "by_tag:")
		for _, item := range summary.ByTag {
			fmt.Fprintf(&buf, "- %s samples=%d mrr=%.4f\n", item.Tag, item.Metrics.SampleCount, item.Metrics.MRR)
			for _, k := range summary.Ks {
				fmt.Fprintf(&buf, "  hit@%d=%.4f recall@%d=%.4f ndcg@%d=%.4f\n",
					k, item.Metrics.HitRateAtK[k],
					k, item.Metrics.AverageRecallAtK[k],
					k, item.Metrics.AverageNDCGAtK[k])
			}
		}
		fmt.Fprintln(&buf)
	}

	if len(summary.Channels) > 0 {
		fmt.Fprintln(&buf, "channels:")
		for _, channel := range summary.Channels {
			fmt.Fprintf(&buf, "- %s samples=%d unique_hits=%d overlap_hits=%d avg_first_relevant_rank=%.2f\n",
				channel.ChannelName,
				channel.SampleCount,
				channel.UniqueHitCount,
				channel.OverlapHitCount,
				channel.AverageFirstRelevantRank,
			)
			for _, k := range summary.Ks {
				fmt.Fprintf(&buf, "  channel_hit@%d=%.4f\n", k, channel.HitRateAtK[k])
			}
		}
		fmt.Fprintln(&buf)
	}

	fmt.Fprintln(&buf, "samples_detail:")
	for _, sample := range summary.Samples {
		fmt.Fprintf(&buf, "- %s target=%s rr=%.4f firstRelevantRank=%d\n", sample.Name, sample.Target, sample.ReciprocalRank, sample.FirstRelevantRank)
		for _, k := range summary.Ks {
			fmt.Fprintf(&buf, "  hit@%d=%t recall@%d=%.4f ndcg@%d=%.4f\n",
				k, sample.HitAtK[k],
				k, sample.RecallAtK[k],
				k, sample.NDCGAtK[k])
		}
		for _, channel := range sample.Channels {
			fmt.Fprintf(&buf, "  channel=%s firstRelevantRank=%d uniqueHits=%d overlapHits=%d\n",
				channel.ChannelName,
				channel.FirstRelevantRank,
				channel.UniqueHitCount,
				channel.OverlapHitCount,
			)
			for _, k := range summary.Ks {
				fmt.Fprintf(&buf, "    channel_hit@%d=%t\n", k, channel.HitAtK[k])
			}
		}
	}
	return buf.String()
}
