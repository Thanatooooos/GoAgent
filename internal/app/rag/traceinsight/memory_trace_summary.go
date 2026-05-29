package traceinsight

import (
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

type LongTermMemorySummary struct {
	Used                bool
	CandidateCount      int
	SelectedCount       int
	RuleCount           int
	FactCandidateCount  int
	FactSelectedCount   int
	Truncated           bool
	CacheEnabled        bool
	RuleCacheLayer      string
	FactCacheLayer      string
	EmbeddingCacheLayer string
	RecomputeReason     string
	SourceCounts        map[string]int
	ContributionCounts  map[string]int
	TypeCounts          map[string]int
	MemoryIDs           []string
	RuleMemoryIDs       []string
	FactMemoryIDs       []string
	Summary             string
}

type SessionRecallSummary struct {
	Used                   bool
	CandidateCount         int
	ExcerptCount           int
	TopScore               float64
	CacheEnabled           bool
	CacheLayer             string
	EmbeddingCacheLayer    string
	RecallFingerprint      string
	RecomputeReason        string
	SkippedPerMessageLimit int
	TruncatedBy            string
	SelectedMessageIDs     []string
	SelectedChunkIDs       []string
	Summary                string
}

func ParseLongTermMemoryNode(node *domain.RagTraceNode) *LongTermMemorySummary {
	if node == nil {
		return nil
	}
	return ParseLongTermMemoryPayload(strings.TrimSpace(node.NodeID), readTraceExtra(node.ExtraData))
}

func ParseSessionRecallNode(node *domain.RagTraceNode) *SessionRecallSummary {
	if node == nil {
		return nil
	}
	return ParseSessionRecallPayload(strings.TrimSpace(node.NodeID), readTraceExtra(node.ExtraData))
}

func ParseLongTermMemoryPayload(nodeID string, payload map[string]any) *LongTermMemorySummary {
	if len(payload) == 0 {
		return nil
	}
	if nodeID == "session_recall" || strings.TrimSpace(readDataString(payload, "recallFingerprint")) != "" || readDataInt(payload, "excerptCount") > 0 {
		return nil
	}
	if nodeID != "long_term_memory" &&
		readDataInt(payload, "candidateCount") == 0 &&
		readDataInt(payload, "selectedCount") == 0 &&
		readDataInt(payload, "ruleCount") == 0 &&
		readDataInt(payload, "factSelectedCount") == 0 &&
		len(readStringSlice(payload, "memoryIds")) == 0 &&
		strings.TrimSpace(readDataString(payload, "ruleCacheLayer")) == "" &&
		strings.TrimSpace(readDataString(payload, "factCacheLayer")) == "" {
		return nil
	}

	summary := &LongTermMemorySummary{
		Used:                readDataBool(payload, "used"),
		CandidateCount:      readDataInt(payload, "candidateCount"),
		SelectedCount:       readDataInt(payload, "selectedCount"),
		RuleCount:           readDataInt(payload, "ruleCount"),
		FactCandidateCount:  readDataInt(payload, "factCandidateCount"),
		FactSelectedCount:   readDataInt(payload, "factSelectedCount"),
		Truncated:           readDataBool(payload, "truncated"),
		CacheEnabled:        readDataBool(payload, "cacheEnabled"),
		RuleCacheLayer:      readDataString(payload, "ruleCacheLayer"),
		FactCacheLayer:      readDataString(payload, "factCacheLayer"),
		EmbeddingCacheLayer: readDataString(payload, "embeddingCacheLayer"),
		RecomputeReason:     readDataString(payload, "recomputeReason"),
		SourceCounts:        readCountMap(payload, "sourceCounts"),
		ContributionCounts:  readCountMap(payload, "contributionCounts"),
		TypeCounts:          readCountMap(payload, "typeCounts"),
		MemoryIDs:           readStringSlice(payload, "memoryIds"),
		RuleMemoryIDs:       readStringSlice(payload, "ruleMemoryIds"),
		FactMemoryIDs:       readStringSlice(payload, "factMemoryIds"),
	}
	if summary.CandidateCount < 0 {
		summary.CandidateCount = 0
	}
	if summary.SelectedCount < 0 {
		summary.SelectedCount = 0
	}
	if summary.RuleCount < 0 {
		summary.RuleCount = 0
	}
	if summary.FactCandidateCount < 0 {
		summary.FactCandidateCount = 0
	}
	if summary.FactSelectedCount < 0 {
		summary.FactSelectedCount = 0
	}
	if summary.isEmpty() {
		return nil
	}

	parts := make([]string, 0, 6)
	if summary.CandidateCount > 0 {
		selection := fmt.Sprintf("selected %d/%d memories", summary.SelectedCount, summary.CandidateCount)
		if summary.RuleCount > 0 || summary.FactCandidateCount > 0 || summary.FactSelectedCount > 0 {
			selection = fmt.Sprintf("%s (rules=%d facts=%d/%d)", selection, summary.RuleCount, summary.FactSelectedCount, summary.FactCandidateCount)
		}
		parts = append(parts, selection)
	} else if summary.SelectedCount > 0 {
		parts = append(parts, fmt.Sprintf("selected %d memories", summary.SelectedCount))
	}
	if cacheText := RenderMemoryCacheSummary(summary.RuleCacheLayer, summary.FactCacheLayer, summary.EmbeddingCacheLayer); cacheText != "" {
		parts = append(parts, "cache "+cacheText)
	}
	if contributionText := RenderCountSummary(summary.ContributionCounts, []string{"hybrid", "vector_only", "keyword_only"}); contributionText != "" {
		parts = append(parts, "contributions "+contributionText)
	}
	if sourceText := RenderCountSummary(summary.SourceCounts, []string{"keyword", "vector"}); sourceText != "" {
		parts = append(parts, "sources "+sourceText)
	}
	if summary.RecomputeReason != "" {
		parts = append(parts, "reason="+summary.RecomputeReason)
	}
	if summary.Truncated {
		parts = append(parts, "truncated")
	}
	summary.Summary = strings.Join(parts, ", ")
	return summary
}

func ParseSessionRecallPayload(nodeID string, payload map[string]any) *SessionRecallSummary {
	if len(payload) == 0 {
		return nil
	}
	if nodeID == "long_term_memory" || strings.TrimSpace(readDataString(payload, "ruleCacheLayer")) != "" || strings.TrimSpace(readDataString(payload, "factCacheLayer")) != "" {
		return nil
	}
	if nodeID != "session_recall" &&
		readDataInt(payload, "excerptCount") == 0 &&
		readDataInt(payload, "candidateCount") == 0 &&
		len(readMapItems(payload["selectedHits"])) == 0 &&
		strings.TrimSpace(readDataString(payload, "cacheLayer")) == "" &&
		strings.TrimSpace(readDataString(payload, "recallFingerprint")) == "" {
		return nil
	}

	selectedHits := readMapItems(payload["selectedHits"])
	messageIDs := make([]string, 0, len(selectedHits))
	chunkIDs := make([]string, 0, len(selectedHits))
	for _, item := range selectedHits {
		if messageID := strings.TrimSpace(readDataString(item, "messageId")); messageID != "" {
			messageIDs = append(messageIDs, messageID)
		}
		if chunkID := strings.TrimSpace(readDataString(item, "sourceChunkId")); chunkID != "" {
			chunkIDs = append(chunkIDs, chunkID)
		}
	}

	summary := &SessionRecallSummary{
		Used:                   readDataBool(payload, "used"),
		CandidateCount:         readDataInt(payload, "candidateCount"),
		ExcerptCount:           readDataInt(payload, "excerptCount"),
		TopScore:               readDataFloat(payload, "topScore"),
		CacheEnabled:           readDataBool(payload, "cacheEnabled"),
		CacheLayer:             readDataString(payload, "cacheLayer"),
		EmbeddingCacheLayer:    readDataString(payload, "embeddingCacheLayer"),
		RecallFingerprint:      readDataString(payload, "recallFingerprint"),
		RecomputeReason:        readDataString(payload, "recomputeReason"),
		SkippedPerMessageLimit: readDataInt(payload, "skippedPerMessageLimit"),
		TruncatedBy:            readDataString(payload, "truncatedBy"),
		SelectedMessageIDs:     messageIDs,
		SelectedChunkIDs:       chunkIDs,
	}
	if summary.CandidateCount < 0 {
		summary.CandidateCount = 0
	}
	if summary.ExcerptCount < 0 {
		summary.ExcerptCount = len(selectedHits)
	}
	if summary.ExcerptCount == 0 && len(selectedHits) > 0 {
		summary.ExcerptCount = len(selectedHits)
	}
	if summary.SkippedPerMessageLimit < 0 {
		summary.SkippedPerMessageLimit = 0
	}
	if summary.TopScore < 0 {
		summary.TopScore = 0
	}
	if summary.isEmpty() {
		return nil
	}

	parts := make([]string, 0, 6)
	if summary.ExcerptCount > 0 {
		if summary.CandidateCount > 0 {
			parts = append(parts, fmt.Sprintf("recalled %d/%d excerpts", summary.ExcerptCount, summary.CandidateCount))
		} else {
			parts = append(parts, fmt.Sprintf("recalled %d excerpts", summary.ExcerptCount))
		}
	} else if summary.CandidateCount > 0 {
		parts = append(parts, fmt.Sprintf("selected 0/%d excerpts", summary.CandidateCount))
	}
	if summary.TopScore > 0 {
		parts = append(parts, fmt.Sprintf("topScore=%.4f", summary.TopScore))
	}
	if cacheText := RenderSessionCacheSummary(summary.CacheLayer, summary.EmbeddingCacheLayer); cacheText != "" {
		parts = append(parts, "cache "+cacheText)
	}
	if summary.SkippedPerMessageLimit > 0 {
		parts = append(parts, fmt.Sprintf("perMessageSkips=%d", summary.SkippedPerMessageLimit))
	}
	if summary.TruncatedBy != "" {
		parts = append(parts, "truncatedBy="+summary.TruncatedBy)
	}
	if summary.RecomputeReason != "" {
		parts = append(parts, "reason="+summary.RecomputeReason)
	}
	summary.Summary = strings.Join(parts, ", ")
	return summary
}

func (s *LongTermMemorySummary) IsDegraded() bool {
	return s != nil && (s.RuleCacheLayer == "fallback" || s.FactCacheLayer == "fallback")
}

func (s *SessionRecallSummary) IsDegraded() bool {
	return s != nil && (s.CacheLayer == "fallback" || strings.Contains(s.RecomputeReason, "fingerprint_unavailable"))
}

func (s *LongTermMemorySummary) isEmpty() bool {
	if s == nil {
		return true
	}
	return s.CandidateCount == 0 &&
		s.SelectedCount == 0 &&
		s.RuleCount == 0 &&
		s.FactCandidateCount == 0 &&
		s.FactSelectedCount == 0 &&
		s.RuleCacheLayer == "" &&
		s.FactCacheLayer == "" &&
		s.EmbeddingCacheLayer == "" &&
		s.RecomputeReason == "" &&
		len(s.SourceCounts) == 0 &&
		len(s.ContributionCounts) == 0 &&
		len(s.TypeCounts) == 0 &&
		len(s.MemoryIDs) == 0
}

func (s *SessionRecallSummary) isEmpty() bool {
	if s == nil {
		return true
	}
	return s.CandidateCount == 0 &&
		s.ExcerptCount == 0 &&
		s.TopScore == 0 &&
		s.CacheLayer == "" &&
		s.EmbeddingCacheLayer == "" &&
		s.RecallFingerprint == "" &&
		s.RecomputeReason == "" &&
		s.SkippedPerMessageLimit == 0 &&
		s.TruncatedBy == "" &&
		len(s.SelectedMessageIDs) == 0 &&
		len(s.SelectedChunkIDs) == 0
}

func RenderMemoryCacheSummary(ruleLayer string, factLayer string, embeddingLayer string) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(ruleLayer) != "" {
		parts = append(parts, "rule="+strings.TrimSpace(ruleLayer))
	}
	if strings.TrimSpace(factLayer) != "" {
		parts = append(parts, "fact="+strings.TrimSpace(factLayer))
	}
	if strings.TrimSpace(embeddingLayer) != "" {
		parts = append(parts, "embedding="+strings.TrimSpace(embeddingLayer))
	}
	return strings.Join(parts, " ")
}

func RenderSessionCacheSummary(cacheLayer string, embeddingLayer string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(cacheLayer) != "" {
		parts = append(parts, "session="+strings.TrimSpace(cacheLayer))
	}
	if strings.TrimSpace(embeddingLayer) != "" {
		parts = append(parts, "embedding="+strings.TrimSpace(embeddingLayer))
	}
	return strings.Join(parts, " ")
}

func RenderCountSummary(counts map[string]int, keys []string) string {
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if counts[key] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
		}
	}
	return strings.Join(parts, ", ")
}

func readTraceExtra(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload
}

func readDataString(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func readDataBool(data map[string]any, key string) bool {
	value, ok := data[key]
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func readDataInt(data map[string]any, key string) int {
	value, ok := data[key]
	if !ok || value == nil {
		return -1
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return -1
	}
}

func readDataFloat(data map[string]any, key string) float64 {
	value, ok := data[key]
	if !ok || value == nil {
		return -1
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return -1
	}
}

func readStringSlice(data map[string]any, key string) []string {
	value, ok := data[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				items = append(items, text)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func readMapItems(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
		return items
	default:
		return nil
	}
}

func readCountMap(data map[string]any, key string) map[string]int {
	value, ok := data[key]
	if !ok || value == nil {
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	counts := make(map[string]int, len(raw))
	for name, value := range raw {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		switch typed := value.(type) {
		case int:
			counts[trimmed] = typed
		case int32:
			counts[trimmed] = int(typed)
		case int64:
			counts[trimmed] = int(typed)
		case float64:
			counts[trimmed] = int(typed)
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}
