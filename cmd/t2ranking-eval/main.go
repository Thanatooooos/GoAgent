package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	rageval "local/rag-project/internal/app/rag/evaluation"
	"local/rag-project/internal/framework/config"
)

type query struct {
	ID   string
	Text string
}

type qrel struct {
	QueryID   string
	PassageID string
	Relevance int
}

type documentRow struct {
	ID      string
	DocName string
}

type sampleFile struct {
	Samples []rageval.Sample `json:"samples"`
}

type hardsetPlan struct {
	Items []struct {
		QueryID    string `json:"queryId"`
		PositiveID string `json:"positiveId"`
	} `json:"items"`
}

type manualLabel struct {
	QueryID   json.RawMessage `json:"queryId"`
	PassageID json.RawMessage `json:"passageId"`
	Label     string          `json:"label"`
}

func main() {
	dataDir := flag.String("data-dir", "tmp/t2ranking-dev", "directory containing queries.dev.tsv and qrels.dev.tsv")
	kbName := flag.String("kb", "t2ranking-vector-preflight", "knowledge base name")
	limit := flag.Int("limit", 1000, "number of query rows to consider")
	output := flag.String("output", "tmp/t2ranking-dev/eval-500-semantic.json", "evaluation samples JSON output path")
	queryIDsPath := flag.String("query-ids", "", "optional file containing one selected query ID per line")
	primaryPlanPath := flag.String("primary-plan", "", "optional hard-set plan that fixes one primary positive per query")
	manualLabelsPath := flag.String("manual-labels", "", "optional JSONL audit decisions; replaces official qrels with reviewed positives")
	includeWeak := flag.Bool("include-weak", false, "include manually reviewed weak positives with relevance 1")
	topK := flag.Int("top-k", 10, "retrieval candidate count stored in generated samples")
	flag.Parse()

	queries, err := loadQueries(filepath.Join(*dataDir, "queries.dev.tsv"), *limit)
	if err != nil {
		fatalf("load queries: %v", err)
	}
	if strings.TrimSpace(*queryIDsPath) != "" {
		queries, err = filterQueries(queries, *queryIDsPath)
		if err != nil {
			fatalf("filter queries: %v", err)
		}
	}
	if err := config.LoadConfig("configs"); err != nil {
		fatalf("load config: %v", err)
	}
	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fatalf("open database: %v", err)
	}

	var kb struct{ ID string }
	if err := db.Table("t_knowledge_base").Select("id").Where("name = ?", strings.TrimSpace(*kbName)).Take(&kb).Error; err != nil {
		fatalf("find knowledge base %q: %v", *kbName, err)
	}
	var docs []documentRow
	if err := db.Table("t_knowledge_document").Select("id, doc_name").Where("kb_id = ? AND deleted = 0", kb.ID).Find(&docs).Error; err != nil {
		fatalf("list knowledge base documents: %v", err)
	}
	passageDocuments := make(map[string]string, len(docs))
	for _, doc := range docs {
		passageDocuments[strings.TrimSuffix(doc.DocName, filepath.Ext(doc.DocName))] = doc.ID
	}
	rels, err := loadQrels(filepath.Join(*dataDir, "qrels.dev.tsv"), queryIDSet(queries))
	if err != nil {
		fatalf("load qrels: %v", err)
	}
	samples := buildSamples(queries, rels, passageDocuments, kb.ID)
	if strings.TrimSpace(*primaryPlanPath) != "" {
		samples, err = applyPrimaryPlan(samples, *primaryPlanPath, passageDocuments)
		if err != nil {
			fatalf("apply primary plan: %v", err)
		}
	}
	if strings.TrimSpace(*manualLabelsPath) != "" {
		samples, err = applyManualLabels(samples, *manualLabelsPath, passageDocuments, *includeWeak)
		if err != nil {
			fatalf("apply manual labels: %v", err)
		}
	}
	if *topK <= 0 {
		fatalf("top-k must be positive")
	}
	for i := range samples {
		samples[i].TopK = *topK
	}
	data, err := json.MarshalIndent(sampleFile{Samples: samples}, "", "  ")
	if err != nil {
		fatalf("marshal samples: %v", err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fatalf("write samples: %v", err)
	}
	fmt.Printf("samples=%d knowledge_base_id=%s output=%s\n", len(samples), kb.ID, *output)
}

func applyManualLabels(samples []rageval.Sample, path string, passageDocuments map[string]string, includeWeak bool) ([]rageval.Sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	labels := map[string]manualLabel{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item manualLabel
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		queryID, err := jsonID(item.QueryID)
		if err != nil {
			return nil, fmt.Errorf("read query ID: %w", err)
		}
		passageID, err := jsonID(item.PassageID)
		if err != nil {
			return nil, fmt.Errorf("read passage ID: %w", err)
		}
		labels[queryID+"|"+passageID] = item
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]rageval.Sample, 0, len(samples))
	for _, sample := range samples {
		queryID := strings.TrimPrefix(sample.Name, "t2ranking_dev_")
		expected := map[string]int{}
		for key, item := range labels {
			if !strings.HasPrefix(key, queryID+"|") {
				continue
			}
			relevance := 0
			switch item.Label {
			case "strong_positive", "strong":
				relevance = 3
			case "weak_positive", "weak":
				if includeWeak {
					relevance = 1
				}
			}
			if relevance == 0 {
				continue
			}
			passageID, err := jsonID(item.PassageID)
			if err != nil {
				return nil, fmt.Errorf("read passage ID: %w", err)
			}
			documentID, ok := passageDocuments[passageID]
			if !ok {
				return nil, fmt.Errorf("reviewed passage %q for query %q is absent", passageID, queryID)
			}
			expected[documentID] = relevance
		}
		if len(expected) == 0 {
			continue
		}
		ids := make([]string, 0, len(expected))
		for id := range expected {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sample.ExpectedIDs = ids
		sample.ExpectedRelevance = expected
		out = append(out, sample)
	}
	return out, nil
}

func jsonID(raw json.RawMessage) (string, error) {
	var stringID string
	if err := json.Unmarshal(raw, &stringID); err == nil {
		return stringID, nil
	}
	var numberID json.Number
	if err := json.Unmarshal(raw, &numberID); err != nil {
		return "", err
	}
	return numberID.String(), nil
}

func applyPrimaryPlan(samples []rageval.Sample, path string, passageDocuments map[string]string) ([]rageval.Sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan hardsetPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	primary := map[string]string{}
	for _, item := range plan.Items {
		primary[item.QueryID] = item.PositiveID
	}
	out := make([]rageval.Sample, 0, len(samples))
	for _, sample := range samples {
		queryID := strings.TrimPrefix(sample.Name, "t2ranking_dev_")
		passageID, ok := primary[queryID]
		if !ok {
			continue
		}
		documentID, ok := passageDocuments[passageID]
		if !ok {
			return nil, fmt.Errorf("primary passage %q for query %q is absent", passageID, queryID)
		}
		sample.ExpectedIDs = []string{documentID}
		sample.ExpectedRelevance = map[string]int{documentID: 3}
		out = append(out, sample)
	}
	return out, nil
}

func filterQueries(queries []query, path string) ([]query, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	wanted := map[string]struct{}{}
	for _, id := range strings.Fields(string(data)) {
		wanted[id] = struct{}{}
	}
	out := make([]query, 0, len(wanted))
	for _, item := range queries {
		if _, ok := wanted[item.ID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func loadQueries(path string, limit int) ([]query, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
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
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			queries = append(queries, query{ID: parts[0], Text: parts[1]})
		}
	}
	return queries, scanner.Err()
}

func loadQrels(path string, wanted map[string]struct{}) ([]qrel, error) {
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
		if _, ok := wanted[parts[0]]; !ok {
			continue
		}
		var relevance int
		if _, err := fmt.Sscanf(parts[3], "%d", &relevance); err != nil {
			return nil, fmt.Errorf("parse relevance for query %s: %w", parts[0], err)
		}
		if relevance > 0 {
			result = append(result, qrel{QueryID: parts[0], PassageID: parts[2], Relevance: relevance})
		}
	}
	return result, scanner.Err()
}

func queryIDSet(queries []query) map[string]struct{} {
	result := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		result[query.ID] = struct{}{}
	}
	return result
}

func buildSamples(queries []query, rels []qrel, passageDocuments map[string]string, knowledgeBaseID string) []rageval.Sample {
	byQuery := make(map[string][]qrel)
	for _, rel := range rels {
		if _, ok := passageDocuments[rel.PassageID]; ok {
			byQuery[rel.QueryID] = append(byQuery[rel.QueryID], rel)
		}
	}
	samples := make([]rageval.Sample, 0)
	for _, query := range queries {
		expected := make(map[string]int)
		for _, rel := range byQuery[query.ID] {
			documentID := passageDocuments[rel.PassageID]
			if rel.Relevance > expected[documentID] {
				expected[documentID] = rel.Relevance
			}
		}
		if len(expected) == 0 {
			continue
		}
		ids := make([]string, 0, len(expected))
		for id := range expected {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		samples = append(samples, rageval.Sample{
			Name:              "t2ranking_dev_" + query.ID,
			Query:             query.Text,
			Tags:              []string{"t2ranking", "semantic_baseline", "parent_child"},
			Target:            rageval.TargetDocument,
			ExpectedIDs:       ids,
			ExpectedRelevance: expected,
			KnowledgeBaseIDs:  []string{knowledgeBaseID},
			SearchMode:        "semantic",
			TopK:              10,
		})
	}
	return samples
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
