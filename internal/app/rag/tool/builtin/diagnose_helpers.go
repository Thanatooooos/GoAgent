package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ragdomain "local/rag-project/internal/app/rag/domain"
)

type ingestionNodeStats struct {
	Total         int
	SuccessCount  int
	FailedCount   int
	RunningCount  int
	PendingCount  int
	LastNodeID    string
	LastStatus    string
	FailedNodeID  string
	FailedError   string
	RunningNodeID string
}

func summarizeIngestionNodes(nodes []ingestiondomain.TaskNode) ingestionNodeStats {
	stats := ingestionNodeStats{Total: len(nodes)}
	for _, node := range nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		status := strings.TrimSpace(node.Status)
		if nodeID != "" {
			stats.LastNodeID = nodeID
		}
		if status != "" {
			stats.LastStatus = status
		}
		switch status {
		case ingestiondomain.TaskStatusSuccess:
			stats.SuccessCount++
		case ingestiondomain.TaskStatusFailed:
			stats.FailedCount++
			if stats.FailedNodeID == "" {
				stats.FailedNodeID = nodeID
				stats.FailedError = strings.TrimSpace(node.ErrorMessage)
			}
		case ingestiondomain.TaskStatusRunning:
			stats.RunningCount++
			if stats.RunningNodeID == "" {
				stats.RunningNodeID = nodeID
			}
		case ingestiondomain.TaskStatusPending:
			stats.PendingCount++
		}
	}
	return stats
}

func appendNodeStatsEvidence(evidence []string, stats ingestionNodeStats, prefix string) []string {
	evidence = append(evidence, fmt.Sprintf("%s.total=%d", prefix, stats.Total))
	evidence = append(evidence, fmt.Sprintf("%s.success=%d", prefix, stats.SuccessCount))
	evidence = append(evidence, fmt.Sprintf("%s.failed=%d", prefix, stats.FailedCount))
	evidence = append(evidence, fmt.Sprintf("%s.running=%d", prefix, stats.RunningCount))
	if stats.PendingCount > 0 {
		evidence = append(evidence, fmt.Sprintf("%s.pending=%d", prefix, stats.PendingCount))
	}
	if stats.LastNodeID != "" {
		evidence = append(evidence, fmt.Sprintf("%s.lastNode=%s", prefix, stats.LastNodeID))
	}
	if stats.LastStatus != "" {
		evidence = append(evidence, fmt.Sprintf("%s.lastStatus=%s", prefix, stats.LastStatus))
	}
	return evidence
}

func hasInconsistentIngestionState(taskStatus string, logStatus string, documentStatus string) bool {
	taskStatus = strings.TrimSpace(taskStatus)
	logStatus = strings.TrimSpace(logStatus)
	documentStatus = strings.TrimSpace(documentStatus)

	if taskStatus == ingestiondomain.TaskStatusFailed && (logStatus == "success" || documentStatus == "success") {
		return true
	}
	if taskStatus == ingestiondomain.TaskStatusSuccess && (logStatus == "failed" || documentStatus == "failed") {
		return true
	}
	if logStatus == "failed" && documentStatus == "success" {
		return true
	}
	if logStatus == "success" && documentStatus == "failed" {
		return true
	}
	if taskStatus == ingestiondomain.TaskStatusRunning && (logStatus == "failed" || documentStatus == "failed") {
		return true
	}
	return false
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

func readTraceExtraString(raw string, key string) string {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
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

func readTraceExtraInt(raw string, key string) int {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return -1
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return -1
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return -1
	}
}

func readTraceExtraFloat(raw string, key string) float64 {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return -1
	}
	value, ok := payload[key]
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

func readTraceExtraBool(raw string, key string) bool {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func readTraceExtraStringSlice(raw string, key string) []string {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func findToolWorkflowNode(nodes []ragdomain.RagTraceNode) *ragdomain.RagTraceNode {
	return findTraceNode(nodes, "tool_workflow")
}
