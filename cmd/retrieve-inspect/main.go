package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

type sampleFile struct {
	Meta    map[string]any   `json:"meta,omitempty"`
	Samples []rageval.Sample `json:"samples"`
}

func main() {
	samplesPath := flag.String("samples", "", "path to resolved eval samples JSON")
	resultsPath := flag.String("results", "", "path to retrieve-eval results JSON")
	configDir := flag.String("config-dir", "configs", "config directory")
	worst := flag.Int("worst", 8, "number of worst samples to inspect")
	nameFilter := flag.String("name", "", "inspect a single sample by exact name")
	namesFile := flag.String("names-file", "", "inspect samples listed in a text file, one name per line")
	outputDir := flag.String("output-dir", "", "optional directory to write per-sample inspection reports")
	rerankModel := flag.String("rerank-model", "", "optional rerank model override, e.g. qwen3-rerank or rerank-noop")
	vectorTopKMultiplier := flag.Int("vector-topk-multiplier", 0, "optional override for rag.search.channels.vector-global.top-k-multiplier")
	searchModeOverride := flag.String("search-mode", "", "optional retrieval mode override: semantic, keyword, hybrid, auto")
	flag.Parse()

	if strings.TrimSpace(*samplesPath) == "" || strings.TrimSpace(*resultsPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/retrieve-inspect -samples <resolved.json> -results <results.json> [-worst 8] [-name t2_123] [-names-file names.txt] [-output-dir dir]")
		os.Exit(1)
	}

	sampleFile, err := loadSampleFile(*samplesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load samples failed: %v\n", err)
		os.Exit(1)
	}
	if len(sampleFile.Samples) == 0 {
		fmt.Fprintln(os.Stderr, "no samples found")
		os.Exit(1)
	}

	resultSummary, err := loadSummary(*resultsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load results failed: %v\n", err)
		os.Exit(1)
	}
	if len(resultSummary.Samples) == 0 {
		fmt.Fprintln(os.Stderr, "no result samples found")
		os.Exit(1)
	}

	sampleByName := make(map[string]rageval.Sample, len(sampleFile.Samples))
	for _, sample := range sampleFile.Samples {
		sampleByName[strings.TrimSpace(sample.Name)] = sample
	}

	selected, err := selectSamples(resultSummary.Samples, strings.TrimSpace(*nameFilter), strings.TrimSpace(*namesFile), *worst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "select samples failed: %v\n", err)
		os.Exit(1)
	}
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "no matching samples selected")
		os.Exit(1)
	}

	outputRoot := strings.TrimSpace(*outputDir)
	if outputRoot != "" {
		if err := os.MkdirAll(outputRoot, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create output dir failed: %v\n", err)
			os.Exit(1)
		}
	}

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

	for idx, resultSample := range selected {
		sourceSample, ok := sampleByName[strings.TrimSpace(resultSample.Name)]
		if !ok {
			fmt.Printf("\n[%d] %s\n", idx+1, resultSample.Name)
			fmt.Println("missing source sample in resolved input, skipped")
			continue
		}

		request := ragretrieve.Request{
			Query:            strings.TrimSpace(sourceSample.Query),
			KnowledgeBaseIDs: append([]string(nil), sourceSample.KnowledgeBaseIDs...),
			SearchMode:       resolveSearchMode(strings.TrimSpace(sourceSample.SearchMode), strings.TrimSpace(*searchModeOverride)),
			TopK:             sourceSample.TopK,
		}
		retrieveResult, err := runtime.Retrieve.Retrieve(context.Background(), request)
		if err != nil {
			report := fmt.Sprintf("\n[%d] %s\nquery=%s\nretrieve failed: %v\n", idx+1, resultSample.Name, sourceSample.Query, err)
			if outputRoot != "" {
				writeReport(outputRoot, idx+1, resultSample.Name, report)
				continue
			}
			fmt.Print(report)
			continue
		}

		report := renderInspection(idx+1, resultSample, sourceSample, retrieveResult)
		if outputRoot != "" {
			writeReport(outputRoot, idx+1, resultSample.Name, report)
			continue
		}
		fmt.Print(report)
	}
}

func loadSampleFile(path string) (sampleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sampleFile{}, err
	}
	var file sampleFile
	if err := json.Unmarshal(data, &file); err != nil {
		return sampleFile{}, err
	}
	return file, nil
}

func loadSummary(path string) (rageval.Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rageval.Summary{}, err
	}
	var summary rageval.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return rageval.Summary{}, err
	}
	return summary, nil
}

func selectSamples(samples []rageval.SampleResult, name string, namesFile string, worst int) ([]rageval.SampleResult, error) {
	if name != "" && namesFile != "" {
		return nil, errors.New("use either -name or -names-file, not both")
	}

	if name != "" {
		for _, sample := range samples {
			if strings.TrimSpace(sample.Name) == name {
				return []rageval.SampleResult{sample}, nil
			}
		}
		return nil, nil
	}

	if namesFile != "" {
		names, err := loadNames(namesFile)
		if err != nil {
			return nil, err
		}
		sampleByName := make(map[string]rageval.SampleResult, len(samples))
		for _, sample := range samples {
			sampleByName[strings.TrimSpace(sample.Name)] = sample
		}
		selected := make([]rageval.SampleResult, 0, len(names))
		for _, item := range names {
			sample, ok := sampleByName[item]
			if !ok {
				continue
			}
			selected = append(selected, sample)
		}
		return selected, nil
	}

	ordered := append([]rageval.SampleResult(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool {
		return compareSampleSeverity(ordered[i], ordered[j])
	})
	if worst > 0 && len(ordered) > worst {
		ordered = ordered[:worst]
	}
	return ordered, nil
}

func compareSampleSeverity(a, b rageval.SampleResult) bool {
	rankA := severityRank(a.FirstRelevantRank)
	rankB := severityRank(b.FirstRelevantRank)
	if rankA != rankB {
		return rankA > rankB
	}

	recallA := metricValue(a.RecallAtK, 10)
	recallB := metricValue(b.RecallAtK, 10)
	if recallA != recallB {
		return recallA < recallB
	}

	if len(a.ExpectedIDs) != len(b.ExpectedIDs) {
		return len(a.ExpectedIDs) < len(b.ExpectedIDs)
	}
	return strings.TrimSpace(a.Query) < strings.TrimSpace(b.Query)
}

func severityRank(rank int) int {
	switch {
	case rank <= 0:
		return 1000
	default:
		return 100 - rank
	}
}

func metricValue(items map[int]float64, key int) float64 {
	if value, ok := items[key]; ok {
		return value
	}
	return 0
}

func renderInspection(index int, resultSample rageval.SampleResult, sourceSample rageval.Sample, retrieveResult ragretrieve.Result) string {
	expected := make(map[string]struct{}, len(sourceSample.ExpectedIDs))
	for _, id := range sourceSample.ExpectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			expected[id] = struct{}{}
		}
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "\n[%d] %s\n", index, resultSample.Name)
	fmt.Fprintf(&out, "query=%s\n", strings.TrimSpace(sourceSample.Query))
	fmt.Fprintf(
		&out,
		"expected=%d firstRelevantRank=%d mrr=%.4f hit@1=%t recall@10=%.4f ndcg@10=%.4f\n",
		len(sourceSample.ExpectedIDs),
		resultSample.FirstRelevantRank,
		resultSample.ReciprocalRank,
		resultSample.HitAtK[1],
		resultSample.RecallAtK[10],
		resultSample.NDCGAtK[10],
	)
	fmt.Fprintf(&out, "channels=%s\n", strings.Join(retrieveResult.SearchChannels, ","))
	fmt.Fprintln(&out, "channel_stats:")
	for _, stat := range retrieveResult.ChannelStats {
		fmt.Fprintf(&out, "  - %s chunks=%d latencyMs=%d", stat.Name, stat.ChunkCount, stat.LatencyMs)
		if strings.TrimSpace(stat.Error) != "" {
			fmt.Fprintf(&out, " error=%s", strings.TrimSpace(stat.Error))
		}
		if metadataText := formatMetadata(stat.Metadata); metadataText != "" {
			fmt.Fprintf(&out, " metadata=%s", metadataText)
		}
		fmt.Fprintln(&out)
	}

	fmt.Fprintln(&out, "top_chunks:")
	for i, chunk := range retrieveResult.Chunks {
		marker := " "
		if _, ok := expected[strings.TrimSpace(chunk.ID)]; ok {
			marker = "*"
		}
		docName := metadataString(chunk.Metadata, "document_name")
		section := metadataString(chunk.Metadata, "section")
		preview := oneLine(chunk.Text, 88)
		fmt.Fprintf(&out, "  %s#%d score=%.4f chunk=%s", marker, i+1, chunk.Score, chunk.ID)
		if docName != "" {
			fmt.Fprintf(&out, " doc=%s", docName)
		}
		if section != "" {
			fmt.Fprintf(&out, " section=%s", section)
		}
		fmt.Fprintln(&out)
		fmt.Fprintf(&out, "     %s\n", preview)
	}
	return out.String()
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func oneLine(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " "))
	runes := []rune(text)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return text
}

func loadNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	names := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func writeReport(outputDir string, index int, name string, report string) {
	filename := fmt.Sprintf("%02d_%s.txt", index, sanitizeName(name))
	path := filepath.Join(outputDir, filename)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(report)+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write report failed for %s: %v\n", name, err)
	}
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "sample"
	}
	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}

func applyExperimentOverrides(rerankModel string, vectorTopKMultiplier int) {
	if rerankModel != "" {
		_ = os.Setenv("AI_RERANK_DEFAULT_MODEL", rerankModel)
	}
	if vectorTopKMultiplier > 0 {
		_ = os.Setenv("RAG_SEARCH_CHANNELS_VECTOR_GLOBAL_TOP_K_MULTIPLIER", fmt.Sprintf("%d", vectorTopKMultiplier))
	}
}

func formatMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(data)
}

func resolveSearchMode(sampleMode string, overrideMode string) string {
	if strings.TrimSpace(overrideMode) != "" {
		return strings.TrimSpace(overrideMode)
	}
	return strings.TrimSpace(sampleMode)
}
