package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const collectionURL = "https://huggingface.co/datasets/THUIR/T2Ranking/resolve/main/data/collection.tsv"

type query struct {
	ID   string
	Text string
}

type qrel struct {
	QueryID   string
	PassageID string
	Relevance int
}

type sample struct {
	Name              string         `json:"name"`
	Query             string         `json:"query"`
	Tags              []string       `json:"tags"`
	Target            string         `json:"target"`
	ExpectedIDs       []string       `json:"expectedIds"`
	ExpectedRelevance map[string]int `json:"expectedRelevance"`
	SearchMode        string         `json:"searchMode"`
	TopK              int            `json:"topK"`
}

type sampleFile struct {
	Samples []sample `json:"samples"`
}

func main() {
	dataDir := flag.String("data-dir", "tmp/t2ranking-dev", "directory containing queries.dev.tsv and qrels.dev.tsv")
	outputDir := flag.String("output-dir", "tmp/t2ranking-dev/subset", "directory for the extracted corpus and evaluation samples")
	limit := flag.Int("limit", 1000, "number of dev queries to include")
	markdownDir := flag.String("markdown-dir", "", "optional directory for one Markdown file per extracted passage")
	markdownLimit := flag.Int("markdown-limit", 0, "maximum number of Markdown files to write; 0 writes every extracted passage")
	markdownOffset := flag.Int("markdown-offset", 0, "number of valid corpus passages to skip before writing Markdown files")
	corpusInput := flag.String("corpus-input", "", "existing corpus.tsv to convert to Markdown without downloading")
	flag.Parse()
	if *limit <= 0 {
		fatalf("limit must be positive")
	}
	if strings.TrimSpace(*corpusInput) != "" {
		if strings.TrimSpace(*markdownDir) == "" {
			fatalf("markdown-dir is required with corpus-input")
		}
		written, err := writeMarkdownCorpus(*corpusInput, *markdownDir, *markdownLimit, *markdownOffset)
		if err != nil {
			fatalf("write markdown corpus: %v", err)
		}
		fmt.Printf("markdown_passages=%d directory=%s\n", written, *markdownDir)
		return
	}

	queries, err := loadQueries(filepath.Join(*dataDir, "queries.dev.tsv"), *limit)
	if err != nil {
		fatalf("load queries: %v", err)
	}
	rels, err := loadQrels(filepath.Join(*dataDir, "qrels.dev.tsv"), queryIDs(queries))
	if err != nil {
		fatalf("load qrels: %v", err)
	}
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fatalf("create output directory: %v", err)
	}

	passageIDs := passageIDs(rels)
	passageCount := len(passageIDs)
	corpusPath := filepath.Join(*outputDir, "corpus.tsv")
	count, err := streamCorpus(context.Background(), collectionURL, passageIDs, corpusPath)
	if err != nil {
		fatalf("extract corpus: %v", err)
	}
	if count != passageCount {
		fatalf("extracted %d of %d requested passages", count, passageCount)
	}
	if err := writeSamples(filepath.Join(*outputDir, "samples.json"), queries, rels); err != nil {
		fatalf("write samples: %v", err)
	}
	if strings.TrimSpace(*markdownDir) != "" {
		written, err := writeMarkdownCorpus(corpusPath, *markdownDir, *markdownLimit, *markdownOffset)
		if err != nil {
			fatalf("write markdown corpus: %v", err)
		}
		fmt.Printf("markdown_passages=%d directory=%s\n", written, *markdownDir)
	}
	fmt.Printf("queries=%d passages=%d corpus=%s samples=%s\n", len(queries), count, corpusPath, filepath.Join(*outputDir, "samples.json"))
}

func loadQueries(path string, limit int) ([]query, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("missing header")
	}
	queries := make([]query, 0, limit)
	for scanner.Scan() && len(queries) < limit {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			continue
		}
		queries = append(queries, query{ID: parts[0], Text: parts[1]})
	}
	return queries, scanner.Err()
}

func loadQrels(path string, selected map[string]struct{}) ([]qrel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("missing header")
	}
	result := make([]qrel, 0)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) != 4 {
			continue
		}
		if _, ok := selected[parts[0]]; !ok {
			continue
		}
		relevance, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("parse relevance for query %s: %w", parts[0], err)
		}
		result = append(result, qrel{QueryID: parts[0], PassageID: parts[2], Relevance: relevance})
	}
	return result, scanner.Err()
}

func queryIDs(queries []query) map[string]struct{} {
	result := make(map[string]struct{}, len(queries))
	for _, item := range queries {
		result[item.ID] = struct{}{}
	}
	return result
}

func passageIDs(rels []qrel) map[string]struct{} {
	result := make(map[string]struct{}, len(rels))
	for _, item := range rels {
		result[item.PassageID] = struct{}{}
	}
	return result
}

func streamCorpus(ctx context.Context, url string, wanted map[string]struct{}, outputPath string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(response.Body)
	buffer := make([]byte, 0, 1024*1024)
	scanner.Buffer(buffer, 8*1024*1024)
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		if _, ok := wanted[parts[0]]; !ok {
			continue
		}
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return count, err
		}
		delete(wanted, parts[0])
		count++
	}
	if err := scanner.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func writeSamples(path string, queries []query, rels []qrel) error {
	byQuery := make(map[string][]qrel, len(queries))
	for _, rel := range rels {
		byQuery[rel.QueryID] = append(byQuery[rel.QueryID], rel)
	}
	samples := make([]sample, 0, len(queries))
	for _, item := range queries {
		relevant := byQuery[item.ID]
		expected := make([]string, 0, len(relevant))
		grades := make(map[string]int, len(relevant))
		for _, rel := range relevant {
			if rel.Relevance <= 0 {
				continue
			}
			expected = append(expected, rel.PassageID)
			grades[rel.PassageID] = rel.Relevance
		}
		if len(expected) == 0 {
			continue
		}
		sort.Strings(expected)
		samples = append(samples, sample{
			Name:              "t2ranking_dev_" + item.ID,
			Query:             item.Text,
			Tags:              []string{"t2ranking", "confusion", "hard_negative"},
			Target:            "chunk",
			ExpectedIDs:       expected,
			ExpectedRelevance: grades,
			SearchMode:        "hybrid",
			TopK:              10,
		})
	}
	data, err := json.MarshalIndent(sampleFile{Samples: samples}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeMarkdownCorpus(corpusPath, outputDir string, limit, offset int) (int, error) {
	if limit < 0 || offset < 0 {
		return 0, fmt.Errorf("markdown limit and offset must not be negative")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, err
	}
	file, err := os.Open(corpusPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	written := 0
	skipped := 0
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		if limit > 0 && written >= limit {
			break
		}
		path := filepath.Join(outputDir, strings.TrimSpace(parts[0])+".md")
		content := "<!-- t2ranking-pid: " + strings.TrimSpace(parts[0]) + " -->\n\n" + strings.TrimSpace(parts[1]) + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return written, err
		}
		written++
	}
	if err := scanner.Err(); err != nil {
		return written, err
	}
	return written, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
