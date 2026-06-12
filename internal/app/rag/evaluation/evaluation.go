package evaluation

import (
	"fmt"
	"math"
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
	Name              string                       `json:"name"`
	Query             string                       `json:"query"`
	UserID            string                       `json:"userId,omitempty"`
	Tags              []string                     `json:"tags,omitempty"`
	Target            Target                       `json:"target"`
	ExpectedIDs       []string                     `json:"expectedIds"`
	Retrieved         []RetrievedItem              `json:"retrieved"`
	ChannelRetrieved  map[string][]RetrievedItem   `json:"channelRetrieved,omitempty"`
	KnowledgeBaseIDs  []string                     `json:"knowledgeBaseIds,omitempty"`
	SearchMode        string                       `json:"searchMode,omitempty"`
	TopK              int                          `json:"topK,omitempty"`
	ChunkStrategy     string                       `json:"chunkStrategy,omitempty"`
	ExpectedRelevance map[string]int               `json:"expectedRelevance,omitempty"`
}

type SampleResult struct {
	Name              string                `json:"name"`
	Query             string                `json:"query"`
	Tags              []string              `json:"tags,omitempty"`
	Target            Target                `json:"target"`
	ExpectedIDs       []string              `json:"expectedIds"`
	RetrievedCount    int                   `json:"retrievedCount"`
	FirstRelevantRank int                   `json:"firstRelevantRank,omitempty"`
	ReciprocalRank    float64               `json:"reciprocalRank"`
	HitAtK            map[int]bool          `json:"hitAtK"`
	RecallAtK         map[int]float64       `json:"recallAtK"`
	NDCGAtK           map[int]float64       `json:"ndcgAtK"`
	Channels          []ChannelSampleResult `json:"channels,omitempty"`
}

type AggregateMetrics struct {
	SampleCount      int             `json:"sampleCount"`
	HitRateAtK       map[int]float64 `json:"hitRateAtK"`
	AverageRecallAtK map[int]float64 `json:"averageRecallAtK"`
	AverageNDCGAtK   map[int]float64 `json:"averageNdcgAtK"`
	MRR              float64         `json:"mrr"`
}

type TagSummary struct {
	Tag     string           `json:"tag"`
	Metrics AggregateMetrics `json:"metrics"`
}

type Summary struct {
	Ks       []int                     `json:"ks"`
	Overall  AggregateMetrics          `json:"overall"`
	Channels []ChannelAggregateMetrics `json:"channels,omitempty"`
	ByTag    []TagSummary              `json:"byTag"`
	Samples  []SampleResult            `json:"samples"`
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
		channelResults, err := evaluateChannelSample(sample, normalizedKs)
		if err != nil {
			return Summary{}, err
		}
		result.Channels = channelResults
		results = append(results, result)
	}

	byTag := buildTagSummaries(results, normalizedKs)
	return Summary{
		Ks:       normalizedKs,
		Overall:  aggregate(results, normalizedKs),
		Channels: aggregateChannelMetrics(results, normalizedKs),
		ByTag:    byTag,
		Samples:  results,
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
	relevance := normalizeRelevance(sample.ExpectedRelevance, sample.ExpectedIDs)
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

	ndcgAtK := computeNDCG(rankedIDs, relevance, ks)

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
		NDCGAtK:           ndcgAtK,
	}, nil
}

func aggregate(results []SampleResult, ks []int) AggregateMetrics {
	metrics := AggregateMetrics{
		SampleCount:      len(results),
		HitRateAtK:       make(map[int]float64, len(ks)),
		AverageRecallAtK: make(map[int]float64, len(ks)),
		AverageNDCGAtK:   make(map[int]float64, len(ks)),
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
			metrics.AverageNDCGAtK[k] += result.NDCGAtK[k]
		}
	}

	total := float64(len(results))
	metrics.MRR /= total
	for _, k := range ks {
		metrics.HitRateAtK[k] /= total
		metrics.AverageRecallAtK[k] /= total
		metrics.AverageNDCGAtK[k] /= total
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

// normalizeRelevance builds a grade map from ExpectedRelevance, falling back to
// binary relevance from ExpectedIDs (present=1, absent=0).
func normalizeRelevance(provided map[string]int, expectedIDs []string) map[string]int {
	if len(provided) > 0 {
		result := make(map[string]int, len(provided))
		for id, grade := range provided {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if grade < 0 {
				grade = 0
			}
			result[id] = grade
		}
		// ensure all expected IDs have at least grade 1
		for _, id := range expectedIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := result[id]; !ok {
				result[id] = 1
			}
		}
		return result
	}
	result := make(map[string]int, len(expectedIDs))
	for _, id := range expectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			result[id] = 1
		}
	}
	return result
}

// computeNDCG computes NDCG@K for every k in ks.
func computeNDCG(rankedIDs []string, relevance map[string]int, ks []int) map[int]float64 {
	result := make(map[int]float64, len(ks))
	if len(rankedIDs) == 0 || len(relevance) == 0 {
		for _, k := range ks {
			result[k] = 0
		}
		return result
	}

	// ideal DCG: sort all expected items by grade descending, truncate to top K
	for _, k := range ks {
		idealGrades := make([]int, 0, len(relevance))
		for _, grade := range relevance {
			if grade > 0 {
				idealGrades = append(idealGrades, grade)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(idealGrades)))
		idcg := dcgAtK(idealGrades, k)

		gains := make([]int, min(k, len(rankedIDs)))
		for i := 0; i < len(gains); i++ {
			if grade, ok := relevance[rankedIDs[i]]; ok {
				gains[i] = grade
			}
		}
		dcg := dcgAtK(gains, k)
		if idcg == 0 {
			result[k] = 0
		} else {
			result[k] = dcg / idcg
		}
	}
	return result
}

// dcgAtK computes DCG@K = Σ(i=1 to min(K,len(gains))) (2^gains[i-1] - 1) / log2(i + 1).
func dcgAtK(gains []int, k int) float64 {
	limit := k
	if limit > len(gains) {
		limit = len(gains)
	}
	var total float64
	for i := 0; i < limit; i++ {
		if gains[i] <= 0 {
			continue
		}
		numerator := math.Pow(2, float64(gains[i])) - 1
		denominator := math.Log2(float64(i + 2)) // i+2 because index is 0-based, rank is 1-based
		total += numerator / denominator
	}
	return total
}
