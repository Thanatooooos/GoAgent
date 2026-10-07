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
	"time"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
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
	executedOutputPath := flag.String("executed-output", "", "write executed samples with retrieved IDs and pipeline traces to a JSON file")
	rerankModel := flag.String("rerank-model", "", "optional rerank model override, e.g. qwen3-reranker-8b or rerank-noop")
	vectorTopKMultiplier := flag.Int("vector-topk-multiplier", 0, "optional override for rag.search.channels.vector-global.top-k-multiplier")
	searchModeOverride := flag.String("search-mode", "", "optional retrieval mode override: semantic, keyword, hybrid, auto")
	rewrite := flag.Bool("rewrite", false, "run query rewrite before retrieval (uses LLM API when enabled in config)")
	disableRerank := flag.Bool("disable-rerank", false, "disable the configured reranker when executing retrieval")
	queryVectorCachePath := flag.String("query-vector-cache", "", "optional JSON cache for original query embeddings used with -execute")
	perSampleTimeout := flag.Duration("per-sample-timeout", 0, "optional timeout for one retrieval sample, e.g. 30s; zero disables it")
	traceExecution := flag.Bool("trace-execution", false, "print per-sample retrieval start, completion, and elapsed time")
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/retrieve-eval -input <samples.json> [-execute] [-rewrite] [-k 1,3,5] [-json] [-output result.json]")
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
		aiRuntime := infraai.NewRuntime()
		if *disableRerank {
			aiRuntime.Rerank = nil
		}
		var queryVectorCache *queryVectorCache
		if strings.TrimSpace(*queryVectorCachePath) != "" {
			queryVectorCache, err = loadQueryVectorCache(strings.TrimSpace(*queryVectorCachePath), config.Get().AI.Embedding.DefaultModel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "load query vector cache failed: %v\n", err)
				os.Exit(1)
			}
			aiRuntime.Embedding = &cachedEmbeddingService{inner: aiRuntime.Embedding, cache: queryVectorCache}
		}
		runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{AIRuntime: aiRuntime})
		if err != nil {
			fmt.Fprintf(os.Stderr, "build rag runtime failed: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = runtime.Close() }()

		if err := executeSamples(context.Background(), runtime, samples, executeOptions{
			searchModeOverride: strings.TrimSpace(*searchModeOverride),
			useRewrite:         *rewrite,
			perSampleTimeout:   *perSampleTimeout,
			traceExecution:     *traceExecution,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "execute retrieval samples failed: %v\n", err)
			os.Exit(1)
		}
		if queryVectorCache != nil {
			fmt.Fprintf(os.Stderr, "query vector cache: hits=%d misses=%d entries=%d path=%s\n", queryVectorCache.hits, queryVectorCache.misses, queryVectorCache.entryCount(), *queryVectorCachePath)
		}
		if strings.TrimSpace(*executedOutputPath) != "" {
			data, err := json.MarshalIndent(sampleFile{Samples: samples}, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "marshal executed samples failed: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(strings.TrimSpace(*executedOutputPath), append(data, '\n'), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "write executed samples failed: %v\n", err)
				os.Exit(1)
			}
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

type executeOptions struct {
	searchModeOverride string
	useRewrite         bool
	perSampleTimeout   time.Duration
	traceExecution     bool
}

func executeSamples(ctx context.Context, runtime *ragbootstrap.Runtime, samples []rageval.Sample, opts executeOptions) error {
	if runtime == nil || runtime.Retrieve == nil {
		return fmt.Errorf("rag retrieve runtime is required")
	}
	if opts.useRewrite && runtime.Rewrite == nil {
		return fmt.Errorf("rewrite is enabled but query rewrite service is unavailable (check rag.query-rewrite.enabled in config)")
	}

	cfg := config.Get()
	subQuestionOptions := ragretrieve.SubQuestionOptions{
		ParallelEnabled: true,
		MaxConcurrency:  2,
	}
	if cfg != nil {
		subQuestionOptions.ParallelEnabled = cfg.Rag.Retrieve.ParallelSubquestions.Enabled
		subQuestionOptions.MaxConcurrency = cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency
	}

	execCfg := rageval.ExecuteConfig{
		Retrieve:           runtime.Retrieve,
		Rewrite:            runtime.Rewrite,
		UseRewrite:         opts.useRewrite,
		SearchModeOverride: opts.searchModeOverride,
		SubQuestionOptions: subQuestionOptions,
	}

	for i := range samples {
		sampleCtx := ctx
		cancel := func() {}
		if opts.perSampleTimeout > 0 {
			sampleCtx, cancel = context.WithTimeout(ctx, opts.perSampleTimeout)
		}
		startedAt := time.Now()
		if opts.traceExecution {
			fmt.Fprintf(os.Stderr, "retrieve sample=%s started\n", samples[i].Name)
		}
		err := rageval.ExecuteSample(sampleCtx, &samples[i], execCfg)
		elapsed := time.Since(startedAt)
		cancel()
		if opts.traceExecution {
			fmt.Fprintf(os.Stderr, "retrieve sample=%s elapsed=%s err=%v\n", samples[i].Name, elapsed.Round(time.Millisecond), err)
		}
		if err != nil {
			return fmt.Errorf("retrieve sample %q after %s: %w", samples[i].Name, elapsed.Round(time.Millisecond), err)
		}
	}
	return nil
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
