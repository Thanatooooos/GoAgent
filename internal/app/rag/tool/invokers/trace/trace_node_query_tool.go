package builtin

import (
	"context"
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
		items = append(items, map[string]any{
			"nodeId":   node.NodeID,
			"nodeType": node.NodeType,
			"nodeName": node.NodeName,
			"status":   node.Status,
		})
	}
	return items
}
