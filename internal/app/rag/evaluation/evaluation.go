package evaluation

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type Target string

const (
	TargetChunk          Target = "chunk"
	TargetDocument       Target = "document"
	TargetDocumentName   Target = "document_name"
	TargetSourceFileName Target = "source_file_name"
	TargetSection        Target = "section"
)

type RetrievedItem struct {
	ChunkID    string         `json:"chunkId"`
	DocumentID string         `json:"documentId"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Score      float64        `json:"score,omitempty"`
}

type Sample struct {
	Name             string          `json:"name"`
	Query            string          `json:"query"`
	Tags             []string        `json:"tags,omitempty"`
	Target           Target          `json:"target"`
	ExpectedIDs      []string        `json:"expectedIds"`
	Retrieved        []RetrievedItem `json:"retrieved"`
	KnowledgeBaseIDs []string        `json:"knowledgeBaseIds,omitempty"`
	SearchMode       string          `json:"searchMode,omitempty"`
	TopK             int             `json:"topK,omitempty"`
}

type SampleResult struct {
	Name              string           `json:"name"`
	Query             string           `json:"query"`
	Tags              []string         `json:"tags,omitempty"`
	Target            Target           `json:"target"`
	ExpectedIDs       []string         `json:"expectedIds"`
	RetrievedCount    int              `json:"retrievedCount"`
	FirstRelevantRank int              `json:"firstRelevantRank,omitempty"`
	ReciprocalRank    float64          `json:"reciprocalRank"`
	HitAtK            map[int]bool     `json:"hitAtK"`
	RecallAtK         map[int]float64  `json:"recallAtK"`
}

type AggregateMetrics struct {
	SampleCount     int              `json:"sampleCount"`
	HitRateAtK      map[int]float64  `json:"hitRateAtK"`
	AverageRecallAtK map[int]float64 `json:"averageRecallAtK"`
	MRR             float64          `json:"mrr"`
}

type TagSummary struct {
	Tag     string           `json:"tag"`
	Metrics AggregateMetrics `json:"metrics"`
}

type Summary struct {
	Ks      []int           `json:"ks"`
	Overall AggregateMetrics `json:"overall"`
	ByTag   []TagSummary    `json:"byTag"`
	Samples []SampleResult  `json:"samples"`
}

func Evaluate(samples []Sample, ks []int) (Summary, error) {
	normalizedKs, err := normalizeKs(ks)
	if err != nil {
		return Summary{}, err
	}

	results := make([]SampleResult, 0, len(samples))
	for _, sample := range samples {
		result, err := evaluateSample(sample, normalizedKs)
		if err != nil {
			return Summary{}, err
		}
		results = append(results, result)
	}

	byTag := buildTagSummaries(results, normalizedKs)
	return Summary{
		Ks:      normalizedKs,
		Overall: aggregate(results, normalizedKs),
		ByTag:   byTag,
		Samples: results,
	}, nil
}

func evaluateSample(sample Sample, ks []int) (SampleResult, error) {
	name := strings.TrimSpace(sample.Name)
	if name == "" {
		return SampleResult{}, fmt.Errorf("evaluation sample name is required")
	}
	target := normalizeTarget(sample.Target)
	if target == "" {
		return SampleResult{}, fmt.Errorf("evaluation sample %q target must be chunk or document", name)
	}

	expectedSet := map[string]struct{}{}
	for _, id := range sample.ExpectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			expectedSet[id] = struct{}{}
		}
	}
	if len(expectedSet) == 0 {
		return SampleResult{}, fmt.Errorf("evaluation sample %q expectedIds is required", name)
	}

	rankedIDs := make([]string, 0, len(sample.Retrieved))
	for _, item := range sample.Retrieved {
		id := extractTargetID(item, target)
		if strings.TrimSpace(id) != "" {
			rankedIDs = append(rankedIDs, strings.TrimSpace(id))
		}
	}

	firstRelevantRank := 0
	matchedByRank := make(map[int]int, len(ks))
	seen := map[string]struct{}{}
	matchedCount := 0
	for index, id := range rankedIDs {
		if _, ok := expectedSet[id]; !ok {
			continue
		}
		if firstRelevantRank == 0 {
			firstRelevantRank = index + 1
		}
		if _, duplicated := seen[id]; duplicated {
			continue
		}
		seen[id] = struct{}{}
		matchedCount++
		for _, k := range ks {
			if index+1 <= k {
				matchedByRank[k]++
			}
		}
	}

	hitAtK := make(map[int]bool, len(ks))
	recallAtK := make(map[int]float64, len(ks))
	expectedTotal := float64(len(expectedSet))
	for _, k := range ks {
		hitAtK[k] = matchedByRank[k] > 0
		recallAtK[k] = float64(matchedByRank[k]) / expectedTotal
	}

	reciprocalRank := 0.0
	if firstRelevantRank > 0 {
		reciprocalRank = 1.0 / float64(firstRelevantRank)
	}

	return SampleResult{
		Name:              name,
		Query:             strings.TrimSpace(sample.Query),
		Tags:              normalizeTags(sample.Tags),
		Target:            target,
		ExpectedIDs:       sortedKeys(expectedSet),
		RetrievedCount:    len(rankedIDs),
		FirstRelevantRank: firstRelevantRank,
		ReciprocalRank:    reciprocalRank,
		HitAtK:            hitAtK,
		RecallAtK:         recallAtK,
	}, nil
}

func aggregate(results []SampleResult, ks []int) AggregateMetrics {
	metrics := AggregateMetrics{
		SampleCount:      len(results),
		HitRateAtK:       make(map[int]float64, len(ks)),
		AverageRecallAtK: make(map[int]float64, len(ks)),
	}
	if len(results) == 0 {
		return metrics
	}

	for _, result := range results {
		metrics.MRR += result.ReciprocalRank
		for _, k := range ks {
			if result.HitAtK[k] {
				metrics.HitRateAtK[k]++
			}
			metrics.AverageRecallAtK[k] += result.RecallAtK[k]
		}
	}

	total := float64(len(results))
	metrics.MRR /= total
	for _, k := range ks {
		metrics.HitRateAtK[k] /= total
		metrics.AverageRecallAtK[k] /= total
	}
	return metrics
}

func buildTagSummaries(results []SampleResult, ks []int) []TagSummary {
	grouped := map[string][]SampleResult{}
	for _, result := range results {
		for _, tag := range result.Tags {
			grouped[tag] = append(grouped[tag], result)
		}
	}

	tags := make([]string, 0, len(grouped))
	for tag := range grouped {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	summaries := make([]TagSummary, 0, len(tags))
	for _, tag := range tags {
		summaries = append(summaries, TagSummary{
			Tag:     tag,
			Metrics: aggregate(grouped[tag], ks),
		})
	}
	return summaries
}

func normalizeKs(ks []int) ([]int, error) {
	if len(ks) == 0 {
		return []int{1, 3, 5}, nil
	}
	unique := make(map[int]struct{}, len(ks))
	result := make([]int, 0, len(ks))
	for _, k := range ks {
		if k <= 0 {
			return nil, fmt.Errorf("k must be positive: %d", k)
		}
		if _, ok := unique[k]; ok {
			continue
		}
		unique[k] = struct{}{}
		result = append(result, k)
	}
	sort.Ints(result)
	return result, nil
}

func normalizeTarget(target Target) Target {
	switch Target(strings.ToLower(strings.TrimSpace(string(target)))) {
	case TargetChunk:
		return TargetChunk
	case TargetDocument:
		return TargetDocument
	case TargetDocumentName:
		return TargetDocumentName
	case TargetSourceFileName:
		return TargetSourceFileName
	case TargetSection:
		return TargetSection
	default:
		return ""
	}
}

func extractTargetID(item RetrievedItem, target Target) string {
	switch target {
	case TargetChunk:
		return item.ChunkID
	case TargetDocument:
		return item.DocumentID
	case TargetDocumentName:
		return metadataString(item.Metadata, "document_name")
	case TargetSourceFileName:
		return metadataString(item.Metadata, "source_file_name")
	case TargetSection:
		return metadataString(item.Metadata, "section")
	default:
		return ""
	}
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	slices.Sort(result)
	return result
}

func sortedKeys(items map[string]struct{}) []string {
	result := make([]string, 0, len(items))
	for key := range items {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
