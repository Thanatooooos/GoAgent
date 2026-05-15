package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type ingestionTaskNodeGetter interface {
	GetNode(ctx context.Context, taskID string, nodeID string) (ingestiondomain.TaskNode, error)
	ListNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

type IngestionTaskNodeQueryTool struct {
	service ingestionTaskNodeGetter
}

func NewIngestionTaskNodeQueryTool(service ingestionTaskNodeGetter) *IngestionTaskNodeQueryTool {
	return &IngestionTaskNodeQueryTool{service: service}
}

func (t *IngestionTaskNodeQueryTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "ingestion_task_node_query",
		Description: "Query ingestion task node details by taskId and optional nodeId. Use after ingestion_task_query to drill into specific nodes.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "taskId",
				Type:        ragtool.ParamTypeString,
				Description: "Ingestion task id.",
				Required:    true,
			},
			{
				Name:        "nodeId",
				Type:        ragtool.ParamTypeString,
				Description: "Optional node id to query a specific node; if omitted, all nodes of the task are returned.",
				Required:    false,
			},
		},
	}
}

func (t *IngestionTaskNodeQueryTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "ingestion_task_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: "ingestion task node service is required"}, fmt.Errorf("ingestion task node service is required")
	}
	taskID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "taskId"))
	if taskID == "" {
		return ragtool.Result{Name: "ingestion_task_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: "taskId is required"}, fmt.Errorf("taskId is required")
	}

	nodeID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "nodeId"))
	if nodeID != "" {
		return t.querySingleNode(ctx, taskID, nodeID)
	}
	return t.queryAllNodes(ctx, taskID)
}

func (t *IngestionTaskNodeQueryTool) querySingleNode(ctx context.Context, taskID string, nodeID string) (ragtool.Result, error) {
	node, err := t.service.GetNode(ctx, taskID, nodeID)
	if err != nil {
		return ragtool.Result{Name: "ingestion_task_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}
	if strings.TrimSpace(node.NodeID) == "" {
		return ragtool.Result{Name: "ingestion_task_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: fmt.Sprintf("task node not found: taskId=%s nodeId=%s", taskID, nodeID)}, fmt.Errorf("task node not found")
	}

	summary := fmt.Sprintf(
		"task=%s node=%s type=%s order=%d status=%s",
		taskID,
		node.NodeID,
		node.NodeType,
		node.NodeOrder,
		strings.TrimSpace(node.Status),
	)
	if node.ErrorMessage != "" {
		summary = fmt.Sprintf("%s error=%s", summary, node.ErrorMessage)
	}
	if node.DurationMs > 0 {
		summary = fmt.Sprintf("%s duration=%dms", summary, node.DurationMs)
	}

	data := map[string]any{
		"taskId":       taskID,
		"nodeId":       node.NodeID,
		"nodeType":     node.NodeType,
		"nodeOrder":    node.NodeOrder,
		"status":       node.Status,
		"durationMs":   node.DurationMs,
		"message":      node.Message,
		"errorMessage": node.ErrorMessage,
		"output":       node.Output,
	}

	return ragtool.Result{
		Name:    "ingestion_task_node_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data:    data,
	}, nil
}

func (t *IngestionTaskNodeQueryTool) queryAllNodes(ctx context.Context, taskID string) (ragtool.Result, error) {
	nodes, err := t.service.ListNodes(ctx, taskID)
	if err != nil {
		return ragtool.Result{Name: "ingestion_task_node_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	nodeItems := make([]map[string]any, 0, len(nodes))
	failedNodes := make([]string, 0)
	runningNodes := make([]string, 0)

	for _, node := range nodes {
		nodeItems = append(nodeItems, map[string]any{
			"nodeId":       node.NodeID,
			"nodeType":     node.NodeType,
			"nodeOrder":    node.NodeOrder,
			"status":       node.Status,
			"durationMs":   node.DurationMs,
			"errorMessage": node.ErrorMessage,
		})
		switch strings.TrimSpace(node.Status) {
		case ingestiondomain.TaskStatusFailed:
			failedNodes = append(failedNodes, fmt.Sprintf("%s(%s)", node.NodeID, node.ErrorMessage))
		case ingestiondomain.TaskStatusRunning:
			runningNodes = append(runningNodes, node.NodeID)
		}
	}

	summary := fmt.Sprintf("task=%s totalNodes=%d", taskID, len(nodes))
	if len(failedNodes) > 0 {
		summary = fmt.Sprintf("%s failed=[%s]", summary, strings.Join(failedNodes, ", "))
	}
	if len(runningNodes) > 0 {
		summary = fmt.Sprintf("%s running=[%s]", summary, strings.Join(runningNodes, ", "))
	}

	return ragtool.Result{
		Name:    "ingestion_task_node_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: map[string]any{
			"taskId":    taskID,
			"nodeCount": len(nodes),
			"nodes":     nodeItems,
		},
	}, nil
}

var _ ingestionTaskNodeGetter = (*ingestionservice.TaskService)(nil)
