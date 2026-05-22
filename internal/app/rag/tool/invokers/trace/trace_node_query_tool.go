package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ragdomain "local/rag-project/internal/app/rag/domain"
	ragport "local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type TraceNodeQueryTool struct {
	runRepo  ragport.RagTraceRunRepository
	nodeRepo ragport.RagTraceNodeRepository
}

func NewTraceNodeQueryTool(runRepo ragport.RagTraceRunRepository, nodeRepo ragport.RagTraceNodeRepository) *TraceNodeQueryTool {
	return &TraceNodeQueryTool{
		runRepo:  runRepo,
		nodeRepo: nodeRepo,
	}
}

func (t *TraceNodeQueryTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "trace_node_query",
		Description: "Query a trace and its nodes by traceId.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "traceId",
				Type:        ragtool.ParamTypeString,
				Description: "Trace id.",
				Required:    true,
			},
		},
	}
}

func (t *TraceNodeQueryTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.runRepo == nil || t.nodeRepo == nil {
		return ragtool.Result{Name: "trace_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: "trace repositories are required"}, fmt.Errorf("trace repositories are required")
	}
	traceID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "traceId"))
	if traceID == "" {
		return ragtool.Result{Name: "trace_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: "traceId is required"}, fmt.Errorf("traceId is required")
	}

	run, err := t.runRepo.GetByTraceID(ctx, traceID)
	if err != nil {
		return ragtool.Result{Name: "trace_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}
	nodes, err := t.nodeRepo.ListByTraceID(ctx, traceID)
	if err != nil {
		return ragtool.Result{Name: "trace_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	nodeItems := summarizeTraceNodes(nodes)
	summary := fmt.Sprintf(
		"trace %s status=%s conversationId=%s nodes=%d",
		run.TraceID,
		strings.TrimSpace(run.Status),
		strings.TrimSpace(run.ConversationID),
		len(nodes),
	)

	return ragtool.Result{
		Name:    "trace_node_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: map[string]any{
			"traceId":        run.TraceID,
			"status":         run.Status,
			"conversationId": run.ConversationID,
			"taskId":         run.TaskID,
			"errorMessage":   run.ErrorMessage,
			"nodeCount":      len(nodes),
			"nodes":          nodeItems,
		},
	}, nil
}

func summarizeTraceNodes(nodes []ragdomain.RagTraceNode) []map[string]any {
	if len(nodes) == 0 {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		item := map[string]any{
			"nodeId":   node.NodeID,
			"nodeType": node.NodeType,
			"nodeName": node.NodeName,
			"status":   node.Status,
		}
		if summary, memoryRecall := summarizeTraceNodeExtra(node); summary != "" {
			item["summary"] = summary
			if memoryRecall != nil {
				item["memoryRecall"] = memoryRecall
			}
		}
		items = append(items, item)
	}
	return items
}

func summarizeTraceNodeExtra(node ragdomain.RagTraceNode) (string, map[string]any) {
	extra := strings.TrimSpace(node.ExtraData)
	if extra == "" {
		return "", nil
	}

	payload := map[string]any{}
	if err := json.Unmarshal([]byte(extra), &payload); err != nil || len(payload) == 0 {
		return "", nil
	}

	if summary, details := summarizeMemoryRecallExtra(payload); summary != "" {
		return summary, details
	}
	return "", nil
}

func summarizeMemoryRecallExtra(payload map[string]any) (string, map[string]any) {
	selectedCount := readTraceNodeInt(payload, "selectedCount")
	candidateCount := readTraceNodeInt(payload, "candidateCount")
	if selectedCount == 0 && candidateCount == 0 && len(readTraceNodeCountMap(payload, "sourceCounts")) == 0 && len(readTraceNodeCountMap(payload, "contributionCounts")) == 0 {
		return "", nil
	}

	sourceCounts := readTraceNodeCountMap(payload, "sourceCounts")
	contributionCounts := readTraceNodeCountMap(payload, "contributionCounts")
	memoryIDs := ragcore.ReadDataStringSlice(payload, "memoryIds")
	truncated := ragcore.ReadDataBool(payload, "truncated")

	parts := make([]string, 0, 4)
	if candidateCount > 0 {
		parts = append(parts, fmt.Sprintf("selected %d/%d memories", selectedCount, candidateCount))
	} else {
		parts = append(parts, fmt.Sprintf("selected %d memories", selectedCount))
	}
	if contributionText := renderTraceCountSummary(contributionCounts, []string{"hybrid", "vector_only", "keyword_only"}); contributionText != "" {
		parts = append(parts, "contributions "+contributionText)
	}
	if sourceText := renderTraceCountSummary(sourceCounts, []string{"keyword", "vector"}); sourceText != "" {
		parts = append(parts, "sources "+sourceText)
	}
	if truncated {
		parts = append(parts, "truncated")
	}

	details := map[string]any{
		"candidateCount":     candidateCount,
		"selectedCount":      selectedCount,
		"truncated":          truncated,
		"sourceCounts":       sourceCounts,
		"contributionCounts": contributionCounts,
		"memoryIds":          memoryIDs,
		"summary":            strings.Join(parts, ", "),
	}
	return details["summary"].(string), details
}

func renderTraceCountSummary(counts map[string]int, keys []string) string {
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

func readTraceNodeCountMap(payload map[string]any, key string) map[string]int {
	raw := ragcore.ReadDataMap(payload, key)
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

func readTraceNodeInt(payload map[string]any, key string) int {
	return ragcore.ReadDataInt(payload, key)
}
