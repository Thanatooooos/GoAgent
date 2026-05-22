package trace

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
)

type TraceNodeItemView struct {
	NodeID       string
	NodeType     string
	NodeName     string
	Status       string
	Summary      string
	MemoryRecall *TraceNodeMemoryRecallSummaryView
}

type TraceNodeMemoryRecallSummaryView struct {
	CandidateCount     int
	SelectedCount      int
	Truncated          bool
	SourceCounts       map[string]int
	ContributionCounts map[string]int
	MemoryIDs          []string
	Summary            string
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
		CandidateCount:     ragcore.ReadDataInt(raw, "candidateCount"),
		SelectedCount:      ragcore.ReadDataInt(raw, "selectedCount"),
		Truncated:          ragcore.ReadDataBool(raw, "truncated"),
		SourceCounts:       readTraceCountMap(raw, "sourceCounts"),
		ContributionCounts: readTraceCountMap(raw, "contributionCounts"),
		MemoryIDs:          ragcore.ReadDataStringSlice(raw, "memoryIds"),
		Summary:            ragcore.ReadDataString(raw, "summary"),
	}
	if view.CandidateCount == 0 && view.SelectedCount == 0 && len(view.SourceCounts) == 0 && len(view.ContributionCounts) == 0 && len(view.MemoryIDs) == 0 && view.Summary == "" {
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
