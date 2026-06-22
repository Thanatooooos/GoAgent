//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

const evalKnowledgeBaseID = "23848386738319617"

func main() {
	os.Exit(run())
}

func run() int {
	if err := config.LoadConfig("configs"); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	ctx := context.Background()
	runtime, err := ragbootstrap.NewRuntime(ctx, ragbootstrap.RuntimeOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime: %v\n", err)
		return 1
	}
	defer func() { _ = runtime.Close() }()
	if runtime.Retrieve == nil || runtime.Rewrite == nil {
		fmt.Fprintln(os.Stderr, "retrieve or rewrite unavailable")
		return 1
	}

	raw, err := rageval.LoadRawSampleFile("testdata/evals/rewrite/samples_batch1.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load samples: %v\n", err)
		return 1
	}
	rawSamples, err := rageval.ExtractSampleArray(json.RawMessage(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract samples: %v\n", err)
		return 1
	}
	samples, err := rageval.ParseRewriteSamples(rawSamples)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse samples: %v\n", err)
		return 1
	}

	subOpts := ragretrieve.SubQuestionOptions{ParallelEnabled: true, MaxConcurrency: 2}
	if cfg := config.Get(); cfg != nil {
		subOpts = ragretrieve.SubQuestionOptions{
			ParallelEnabled: cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
			MaxConcurrency:  cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
		}
	}

	for _, sample := range samples {
		if !strings.HasPrefix(sample.Name, "coref_") {
			continue
		}
		if err := rageval.ExecuteRewriteSample(ctx, &sample, runtime.Rewrite); err != nil {
			fmt.Fprintf(os.Stderr, "rewrite %s: %v\n", sample.Name, err)
			return 1
		}

		target, err := targetFromExpectation(sample.RetrievalExpectation.Target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "target %s: %v\n", sample.Name, err)
			return 1
		}
		topK := sample.RetrievalExpectation.TopK
		if topK <= 0 {
			topK = ragretrieve.DefaultTopK
		}

		baseline := rageval.Sample{
			Name:             sample.Name + ":baseline",
			Query:            sample.Query,
			Target:           target,
			ExpectedIDs:      append([]string(nil), sample.RetrievalExpectation.ExpectedIDs...),
			KnowledgeBaseIDs: []string{evalKnowledgeBaseID},
			SearchMode:       sample.RetrievalExpectation.SearchMode,
			TopK:             topK,
		}
		if err := rageval.ExecuteSample(ctx, &baseline, rageval.ExecuteConfig{Retrieve: runtime.Retrieve}); err != nil {
			fmt.Fprintf(os.Stderr, "baseline %s: %v\n", sample.Name, err)
			return 1
		}

		candidate, err := executeCandidate(ctx, sample, runtime.Retrieve, target, subOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "candidate %s: %v\n", sample.Name, err)
			return 1
		}

		printComparison(sample, baseline, candidate)
	}
	return 0
}

func targetFromExpectation(raw string) (rageval.Target, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "chunk":
		return rageval.TargetChunk, nil
	default:
		return "", fmt.Errorf("unsupported target %q", raw)
	}
}

func executeCandidate(ctx context.Context, sample rageval.RewriteSample, retrieve ragretrieve.Service, target rageval.Target, subOpts ragretrieve.SubQuestionOptions) (rageval.Sample, error) {
	topK := sample.RetrievalExpectation.TopK
	if topK <= 0 {
		topK = ragretrieve.DefaultTopK
	}
	request := ragretrieve.Request{
		Query:            strings.TrimSpace(sample.Query),
		KnowledgeBaseIDs: []string{evalKnowledgeBaseID},
		SearchMode:       strings.TrimSpace(sample.RetrievalExpectation.SearchMode),
		TopK:             topK,
	}
	subQuestions := ragretrieve.BuildRetrieveSubQuestions(sample.Query, sample.SubQuestions)
	executor := ragretrieve.NewSubQuestionExecutor(retrieve, subOpts)
	result, executionMode, _, err := executor.RetrieveMerged(ctx, request, subQuestions, topK)
	if err != nil {
		return rageval.Sample{}, err
	}
	retrieved := make([]rageval.RetrievedItem, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		retrieved = append(retrieved, rageval.RetrievedItem{
			ChunkID:    chunk.ID,
			DocumentID: chunk.DocumentID,
			Score:      float64(chunk.Score),
		})
	}
	return rageval.Sample{
		Name:             sample.Name + ":candidate",
		Query:            sample.Query,
		Target:           target,
		ExpectedIDs:      append([]string(nil), sample.RetrievalExpectation.ExpectedIDs...),
		KnowledgeBaseIDs: []string{evalKnowledgeBaseID},
		Retrieved:        retrieved,
		SearchMode:     sample.RetrievalExpectation.SearchMode,
		TopK:           topK,
		RewrittenQuery: sample.RewrittenQuery,
		SubQuestions:   append([]string(nil), sample.SubQuestions...),
		ExecutionMode:  executionMode,
	}, nil
}

func printComparison(sample rageval.RewriteSample, baseline, candidate rageval.Sample) {
	expected := sample.RetrievalExpectation.ExpectedIDs
	critical := sample.RetrievalExpectation.CriticalExpectedIDs
	fmt.Println(strings.Repeat("=", 72))
	fmt.Println(sample.Name)
	fmt.Printf("  query:      %q\n", sample.Query)
	fmt.Printf("  rewritten:  %q\n", sample.RewrittenQuery)
	fmt.Printf("  subs:       %v\n", sample.SubQuestions)
	fmt.Printf("  mode/top_k: %s / %d\n", sample.RetrievalExpectation.SearchMode, sample.RetrievalExpectation.TopK)
	fmt.Printf("  expected:   %v\n", expected)
	fmt.Printf("  critical:   %v\n", critical)
	if candidate.ExecutionMode != "" {
		fmt.Printf("  candidate execution: %s\n", candidate.ExecutionMode)
	}
	fmt.Printf("  sub_questions used for retrieve: %v\n",
		ragretrieve.BuildRetrieveSubQuestions(sample.Query, sample.SubQuestions))

	fmt.Println("  --- baseline (original query) ---")
	printRetrieved(baseline.Retrieved, expected, critical)
	fmt.Println("  --- candidate (rewrite path) ---")
	printRetrieved(candidate.Retrieved, expected, critical)

	fmt.Printf("  baseline hit expected:  %t\n", hitsExpected(baseline.Retrieved, expected))
	fmt.Printf("  candidate hit expected: %t\n", hitsExpected(candidate.Retrieved, expected))
	fmt.Printf("  baseline hit critical:  %t\n", hitsExpected(baseline.Retrieved, critical))
	fmt.Printf("  candidate hit critical: %t\n", hitsExpected(candidate.Retrieved, critical))
	fmt.Println()
}

func printRetrieved(items []rageval.RetrievedItem, expected, critical []string) {
	if len(items) == 0 {
		fmt.Println("    (empty)")
		return
	}
	exp := toSet(expected)
	crit := toSet(critical)
	for i, item := range items {
		if i >= 10 {
			fmt.Printf("    ... %d more\n", len(items)-i)
			break
		}
		mark := ""
		if exp[item.ChunkID] {
			mark = " EXPECTED"
		}
		if crit[item.ChunkID] {
			mark += " CRITICAL"
		}
		fmt.Printf("    %2d. chunk=%s doc=%s score=%.4f%s\n", i+1, item.ChunkID, item.DocumentID, item.Score, mark)
	}
}

func hitsExpected(items []rageval.RetrievedItem, ids []string) bool {
	want := toSet(ids)
	for _, item := range items {
		if want[item.ChunkID] {
			return true
		}
	}
	return len(want) == 0
}

func toSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	return out
}
