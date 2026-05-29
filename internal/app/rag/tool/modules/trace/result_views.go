package trace

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
)

type TraceNodeItemView struct {
	NodeID        string
	NodeType      string
	NodeName      string
	Status        string
	Summary       string
	MemoryRecall  *TraceNodeMemoryRecallSummaryView
	SessionRecall *TraceNodeSessionRecallSummaryView
}

type TraceNodeMemoryRecallSummaryView struct {
	RuleCount           int
	FactCandidateCount  int
	FactSelectedCount   int
	CandidateCount      int
	SelectedCount       int
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

type TraceNodeSessionRecallSummaryView struct {
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

type TraceNodeQueryResultView struct {
	TraceID        string
	Status         string
	ConversationID string
	TaskID         string
	ErrorMessage   string
	NodeCount      int
	Nodes          []TraceNodeItemView
}

type TraceRetrievalDiagnoseResultView = systemmod.DiagnosisResultView

func ViewTraceNodeQueryResult(result ragcore.Result) (TraceNodeQueryResultView, bool) {
	if strings.TrimSpace(result.Name) != "trace_node_query" {
		return TraceNodeQueryResultView{}, false
	}
	view := TraceNodeQueryResultView{
		TraceID:        result.GetString("traceId"),
		Status:         result.GetString("status"),
		ConversationID: result.GetString("conversationId"),
		TaskID:         result.GetString("taskId"),
		ErrorMessage:   result.GetString("errorMessage"),
		NodeCount:      result.GetInt("nodeCount"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["nodes"]) {
		entry := TraceNodeItemView{
			NodeID:   ragcore.ReadDataString(item, "nodeId"),
			NodeType: ragcore.ReadDataString(item, "nodeType"),
			NodeName: ragcore.ReadDataString(item, "nodeName"),
			Status:   ragcore.ReadDataString(item, "status"),
			Summary:  ragcore.ReadDataString(item, "summary"),
		}
		if memoryRecall := readTraceNodeMemoryRecallView(item); memoryRecall != nil {
			entry.MemoryRecall = memoryRecall
		}
		if sessionRecall := readTraceNodeSessionRecallView(item); sessionRecall != nil {
			entry.SessionRecall = sessionRecall
		}
		if entry.NodeID == "" && entry.NodeName == "" {
			continue
		}
		view.Nodes = append(view.Nodes, entry)
	}
	if view.NodeCount == 0 {
		view.NodeCount = len(view.Nodes)
	}
	return view, true
}

func readTraceNodeMemoryRecallView(item map[string]any) *TraceNodeMemoryRecallSummaryView {
	raw := ragcore.ReadDataMap(item, "memoryRecall")
	if len(raw) == 0 {
		return nil
	}
	view := &TraceNodeMemoryRecallSummaryView{
		RuleCount:           ragcore.ReadDataInt(raw, "ruleCount"),
		FactCandidateCount:  ragcore.ReadDataInt(raw, "factCandidateCount"),
		FactSelectedCount:   ragcore.ReadDataInt(raw, "factSelectedCount"),
		CandidateCount:      ragcore.ReadDataInt(raw, "candidateCount"),
		SelectedCount:       ragcore.ReadDataInt(raw, "selectedCount"),
		Truncated:           ragcore.ReadDataBool(raw, "truncated"),
		CacheEnabled:        ragcore.ReadDataBool(raw, "cacheEnabled"),
		RuleCacheLayer:      ragcore.ReadDataString(raw, "ruleCacheLayer"),
		FactCacheLayer:      ragcore.ReadDataString(raw, "factCacheLayer"),
		EmbeddingCacheLayer: ragcore.ReadDataString(raw, "embeddingCacheLayer"),
		RecomputeReason:     ragcore.ReadDataString(raw, "recomputeReason"),
		SourceCounts:        readTraceCountMap(raw, "sourceCounts"),
		ContributionCounts:  readTraceCountMap(raw, "contributionCounts"),
		TypeCounts:          readTraceCountMap(raw, "typeCounts"),
		MemoryIDs:           ragcore.ReadDataStringSlice(raw, "memoryIds"),
		RuleMemoryIDs:       ragcore.ReadDataStringSlice(raw, "ruleMemoryIds"),
		FactMemoryIDs:       ragcore.ReadDataStringSlice(raw, "factMemoryIds"),
		Summary:             ragcore.ReadDataString(raw, "summary"),
	}
	if view.CandidateCount == 0 && view.SelectedCount == 0 && view.RuleCount == 0 && view.FactCandidateCount == 0 &&
		view.FactSelectedCount == 0 && len(view.SourceCounts) == 0 && len(view.ContributionCounts) == 0 &&
		len(view.TypeCounts) == 0 && len(view.MemoryIDs) == 0 && view.RuleCacheLayer == "" &&
		view.FactCacheLayer == "" && view.EmbeddingCacheLayer == "" && view.RecomputeReason == "" && view.Summary == "" {
		return nil
	}
	return view
}

func readTraceNodeSessionRecallView(item map[string]any) *TraceNodeSessionRecallSummaryView {
	raw := ragcore.ReadDataMap(item, "sessionRecall")
	if len(raw) == 0 {
		return nil
	}
	view := &TraceNodeSessionRecallSummaryView{
		CandidateCount:         ragcore.ReadDataInt(raw, "candidateCount"),
		ExcerptCount:           ragcore.ReadDataInt(raw, "excerptCount"),
		TopScore:               readTraceFloat(raw, "topScore"),
		CacheEnabled:           ragcore.ReadDataBool(raw, "cacheEnabled"),
		CacheLayer:             ragcore.ReadDataString(raw, "cacheLayer"),
		EmbeddingCacheLayer:    ragcore.ReadDataString(raw, "embeddingCacheLayer"),
		RecallFingerprint:      ragcore.ReadDataString(raw, "recallFingerprint"),
		RecomputeReason:        ragcore.ReadDataString(raw, "recomputeReason"),
		SkippedPerMessageLimit: ragcore.ReadDataInt(raw, "skippedPerMessageLimit"),
		TruncatedBy:            ragcore.ReadDataString(raw, "truncatedBy"),
		SelectedMessageIDs:     ragcore.ReadDataStringSlice(raw, "selectedMessageIds"),
		SelectedChunkIDs:       ragcore.ReadDataStringSlice(raw, "selectedChunkIds"),
		Summary:                ragcore.ReadDataString(raw, "summary"),
	}
	if view.CandidateCount == 0 && view.ExcerptCount == 0 && view.TopScore == 0 &&
		view.CacheLayer == "" && view.EmbeddingCacheLayer == "" && view.RecallFingerprint == "" &&
		view.RecomputeReason == "" && view.TruncatedBy == "" && view.SkippedPerMessageLimit == 0 &&
		len(view.SelectedMessageIDs) == 0 && len(view.SelectedChunkIDs) == 0 && view.Summary == "" {
		return nil
	}
	return view
}

func readTraceCountMap(data map[string]any, key string) map[string]int {
	raw := ragcore.ReadDataMap(data, key)
	if len(raw) == 0 {
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

func readTraceFloat(data map[string]any, key string) float64 {
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func ViewTraceRetrievalDiagnoseResult(result ragcore.Result) (TraceRetrievalDiagnoseResultView, bool) {
	if strings.TrimSpace(result.Name) != "trace_retrieval_diagnose" {
		return TraceRetrievalDiagnoseResultView{}, false
	}
	view, ok := systemmod.ViewDiagnosisResult(result)
	if !ok {
		return TraceRetrievalDiagnoseResultView{}, false
	}
	return view, true
}
