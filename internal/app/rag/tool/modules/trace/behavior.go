package trace

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
)

type TraceNodeItemView struct {
	NodeID   string
	NodeType string
	NodeName string
	Status   string
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

func TraceRetrievalDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := systemmod.ViewDiagnosisResult(result)
			if !ok {
				return nil, fmt.Errorf("trace_retrieval_diagnose result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "trace_retrieval_diagnose_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return ragcore.NewObserveResult(true, "The retrieval diagnosis already provides enough trace-level evidence to answer directly.", ragcore.ObserveState(
				"complete",
				result.GetString("conclusion"),
				1,
				nil,
				[]string{result.Name},
			)), true
		},
	}
}

func TraceNodeQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewTraceNodeQueryResult(result)
			if !ok {
				return nil, fmt.Errorf("trace_node_query result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "trace_node_query_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return ragcore.NewObserveResult(true, "The trace node details are already available, so the agent can answer directly.", ragcore.ObserveState(
				"complete",
				ragcore.FirstNonEmpty(result.Summary, result.GetString("errorMessage")),
				1,
				nil,
				[]string{result.Name},
			)), true
		},
	}
}

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
