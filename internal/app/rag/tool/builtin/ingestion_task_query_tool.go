package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type ingestionTaskGetter interface {
	Get(ctx context.Context, id string) (ingestiondomain.Task, error)
	ListNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

type IngestionTaskQueryTool struct {
	service ingestionTaskGetter
}

func NewIngestionTaskQueryTool(service ingestionTaskGetter) *IngestionTaskQueryTool {
	return &IngestionTaskQueryTool{service: service}
}

func (t *IngestionTaskQueryTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "ingestion_task_query",
		Description: "Query an ingestion task by taskId and optionally include node summaries.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "taskId",
				Type:        ragtool.ParamTypeString,
				Description: "Ingestion task id.",
				Required:    true,
			},
			{
				Name:        "includeNodes",
				Type:        ragtool.ParamTypeBoolean,
				Description: "Whether to include task node statuses.",
				Required:    false,
			},
		},
	}
}

func (t *IngestionTaskQueryTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "ingestion_task_query", Status: ragtool.CallStatusFailed, ErrorMessage: "ingestion task service is required"}, fmt.Errorf("ingestion task service is required")
	}
	taskID := strings.TrimSpace(readStringArg(call.Arguments, "taskId"))
	if taskID == "" {
		return ragtool.Result{Name: "ingestion_task_query", Status: ragtool.CallStatusFailed, ErrorMessage: "taskId is required"}, fmt.Errorf("taskId is required")
	}

	task, err := t.service.Get(ctx, taskID)
	if err != nil {
		return ragtool.Result{Name: "ingestion_task_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	includeNodes := readBoolArg(call.Arguments, "includeNodes")
	data := map[string]any{
		"taskId":          task.ID,
		"pipelineId":      task.PipelineID,
		"status":          task.Status,
		"sourceType":      task.SourceType,
		"sourceLocation":  task.SourceLocation,
		"sourceFileName":  task.SourceFileName,
		"metadata":        task.Metadata,
		"startedAt":       task.StartedAt,
		"completedAt":     task.CompletedAt,
		"errorMessage":    task.ErrorMessage,
		"chunkCount":      task.ChunkCount,
		"taskNodeCount":   0,
		"taskNodeSummary": []map[string]any{},
	}

	summary := fmt.Sprintf(
		"ingestion task %s status=%s pipelineId=%s sourceType=%s",
		task.ID,
		strings.TrimSpace(task.Status),
		strings.TrimSpace(task.PipelineID),
		strings.TrimSpace(task.SourceType),
	)

	if includeNodes {
		nodes, err := t.service.ListNodes(ctx, taskID)
		if err != nil {
			return ragtool.Result{Name: "ingestion_task_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
		}
		nodeSummary := make([]map[string]any, 0, len(nodes))
		for _, node := range nodes {
			nodeSummary = append(nodeSummary, map[string]any{
				"nodeId":   node.NodeID,
				"nodeType": node.NodeType,
				"status":   node.Status,
			})
		}
		data["taskNodeCount"] = len(nodes)
		data["taskNodeSummary"] = nodeSummary
		summary = fmt.Sprintf("%s nodes=%d", summary, len(nodes))
		if interesting := summarizeInterestingNodes(nodeSummary); interesting != "" {
			summary = fmt.Sprintf("%s interestingNodes=[%s]", summary, interesting)
		}
	}

	return ragtool.Result{
		Name:    "ingestion_task_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data:    data,
	}, nil
}

func summarizeInterestingNodes(nodes []map[string]any) string {
	if len(nodes) == 0 {
		return ""
	}
	items := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeID := strings.TrimSpace(readStringArg(node, "nodeId"))
		status := strings.ToLower(strings.TrimSpace(readStringArg(node, "status")))
		if nodeID == "" {
			continue
		}
		if status != ingestiondomain.TaskStatusFailed && status != ingestiondomain.TaskStatusRunning {
			continue
		}
		nodeType := strings.TrimSpace(readStringArg(node, "nodeType"))
		if nodeType != "" {
			items = append(items, fmt.Sprintf("%s(status=%s,type=%s)", nodeID, status, nodeType))
			continue
		}
		items = append(items, fmt.Sprintf("%s(status=%s)", nodeID, status))
	}
	return strings.Join(items, ", ")
}

var _ ingestionTaskGetter = (*ingestionservice.TaskService)(nil)
