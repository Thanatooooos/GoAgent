package graph

import (
	"context"
	"fmt"
	"strings"

	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
	"local/rag-project/internal/framework/log"

	"github.com/cloudwego/eino/compose"
)

// diagnosisGraphState flows through the Eino graph nodes, accumulating results.
type diagnosisGraphState struct {
	DocumentID string
	TaskID     string
	NodeID     string
	Results    []ragtool.Result
	LastError  string
}

// DiagnosisGraphTool wraps an Eino-compiled graph as a deterministic Tool.
// It chains: document_ingestion_diagnose → ingestion_task_query → ingestion_task_node_query
// without calling the LLM for each hop.
type DiagnosisGraphTool struct {
	runner compose.Runnable[*diagnosisGraphState, *diagnosisGraphState]
}

// NewDiagnosisGraphTool creates an Eino Graph that chains the 3-hop diagnosis.
// executor is used by each lambda node to invoke individual tools.
func NewDiagnosisGraphTool(executor *ragruntime.Executor) (*DiagnosisGraphTool, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor with registry is required")
	}

	graph := compose.NewGraph[*diagnosisGraphState, *diagnosisGraphState]()

	// Node 1: diagnose
	graph.AddLambdaNode("diagnose", compose.InvokableLambda(
		func(ctx context.Context, state *diagnosisGraphState) (*diagnosisGraphState, error) {
			if state.DocumentID == "" {
				state.LastError = "documentId is required"
				return state, nil
			}
			result, err := executor.Execute(ctx, ragtool.Call{
				Name:      "document_ingestion_diagnose",
				Arguments: map[string]any{"documentId": state.DocumentID},
			})
			if err != nil {
				state.LastError = err.Error()
				return state, nil
			}
			state.Results = append(state.Results, result)
			state.TaskID = strings.TrimSpace(result.GetString("latestTaskId"))
			return state, nil
		},
	))

	// Node 2: task query
	graph.AddLambdaNode("task_query", compose.InvokableLambda(
		func(ctx context.Context, state *diagnosisGraphState) (*diagnosisGraphState, error) {
			if state.TaskID == "" {
				return state, nil
			}
			result, err := executor.Execute(ctx, ragtool.Call{
				Name: "ingestion_task_query",
				Arguments: map[string]any{
					"taskId":       state.TaskID,
					"includeNodes": true,
				},
			})
			if err != nil {
				state.LastError = err.Error()
				return state, nil
			}
			state.Results = append(state.Results, result)
			if nodeID, _, ok := latestInterestingTaskNode(result.Data); ok {
				state.NodeID = nodeID
			}
			return state, nil
		},
	))

	// Node 3: node query
	graph.AddLambdaNode("node_query", compose.InvokableLambda(
		func(ctx context.Context, state *diagnosisGraphState) (*diagnosisGraphState, error) {
			if state.TaskID == "" || state.NodeID == "" {
				return state, nil
			}
			result, err := executor.Execute(ctx, ragtool.Call{
				Name: "ingestion_task_node_query",
				Arguments: map[string]any{
					"taskId": state.TaskID,
					"nodeId": state.NodeID,
				},
			})
			if err != nil {
				state.LastError = err.Error()
				return state, nil
			}
			state.Results = append(state.Results, result)
			return state, nil
		},
	))

	// Wire: START → diagnose → task_query → node_query → END
	_ = graph.AddEdge(compose.START, "diagnose")
	_ = graph.AddEdge("diagnose", "task_query")
	_ = graph.AddEdge("task_query", "node_query")
	_ = graph.AddEdge("node_query", compose.END)

	runner, err := graph.Compile(context.Background(), compose.WithGraphName("diagnosis_chain"))
	if err != nil {
		return nil, fmt.Errorf("compile diagnosis graph: %w", err)
	}

	return &DiagnosisGraphTool{runner: runner}, nil
}

func (t *DiagnosisGraphTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_root_cause_diagnosis",
		Description: "Deterministic 3-hop diagnosis chain: diagnose → task query → node query. Returns node-level root cause when available, without per-hop LLM decisions.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "documentId",
				Type:        ragtool.ParamTypeString,
				Description: "Knowledge document id to diagnose.",
				Required:    true,
			},
		},
	}
}

func (t *DiagnosisGraphTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.runner == nil {
		return ragtool.Result{
			Name:         "document_root_cause_diagnosis",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "diagnosis graph runner is not initialized",
		}, nil
	}

	documentID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "documentId"))
	if documentID == "" {
		return ragtool.Result{
			Name:         "document_root_cause_diagnosis",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "documentId is required",
		}, nil
	}

	state := &diagnosisGraphState{DocumentID: documentID}
	final, err := t.runner.Invoke(ctx, state)
	if err != nil {
		return ragtool.Result{
			Name:         "document_root_cause_diagnosis",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: err.Error(),
		}, nil
	}

	if final.LastError != "" && len(final.Results) == 0 {
		return ragtool.Result{
			Name:         "document_root_cause_diagnosis",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: final.LastError,
		}, nil
	}
	log.Infof("Call graph tool\n")

	chainLen := len(final.Results)
	return ragtool.Result{
		Name:    "document_root_cause_diagnosis",
		Status:  ragtool.CallStatusSuccess,
		Summary: buildDiagnosisGraphSummary(final),
		Data: map[string]any{
			"conclusion":     extractBestConclusion(final.Results),
			"confidence":     extractBestConfidence(final.Results),
			"diagnosisDepth": diagnosisDepthLabel(chainLen),
			"documentId":     final.DocumentID,
			"latestTaskId":   final.TaskID,
			"latestNodeId":   final.NodeID,
			"chainLength":    chainLen,
		},
	}, nil
}

func buildDiagnosisGraphSummary(state *diagnosisGraphState) string {
	if state.LastError != "" && len(state.Results) == 0 {
		return fmt.Sprintf("diagnosis chain failed: %s", state.LastError)
	}
	parts := make([]string, 0, 3)
	for _, r := range state.Results {
		parts = append(parts, fmt.Sprintf("%s=%s", r.Name, r.Status))
	}
	return fmt.Sprintf("diagnosis chain completed %d hops: %s", len(state.Results), strings.Join(parts, " → "))
}

func extractBestConclusion(results []ragtool.Result) string {
	for i := len(results) - 1; i >= 0; i-- {
		if c := strings.TrimSpace(results[i].GetString("conclusion")); c != "" {
			return c
		}
		if c := strings.TrimSpace(results[i].GetString("errorMessage")); c != "" {
			return c
		}
	}
	return ""
}

func extractBestConfidence(results []ragtool.Result) string {
	for i := len(results) - 1; i >= 0; i-- {
		if c := strings.TrimSpace(results[i].GetString("confidence")); c != "" {
			return c
		}
	}
	return "medium"
}

func diagnosisDepthLabel(chainLength int) string {
	switch {
	case chainLength >= 3:
		return "node_level"
	case chainLength >= 2:
		return "task_level"
	default:
		return "diagnose_only"
	}
}

func readStringArg(arguments map[string]any, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func latestInterestingTaskNode(data map[string]any) (string, string, bool) {
	if len(data) == 0 {
		return "", "", false
	}
	raw, ok := data["taskNodeSummary"]
	if !ok || raw == nil {
		return "", "", false
	}

	readFromMap := func(item map[string]any) (string, string, bool) {
		nodeID := strings.TrimSpace(ragcore.ReadStringArg(item, "nodeId"))
		status := strings.ToLower(strings.TrimSpace(ragcore.ReadStringArg(item, "status")))
		if nodeID == "" {
			return "", "", false
		}
		if status == "failed" || status == "running" {
			return nodeID, status, true
		}
		return "", "", false
	}

	switch typed := raw.(type) {
	case []map[string]any:
		for _, item := range typed {
			if nodeID, status, ok := readFromMap(item); ok {
				return nodeID, status, true
			}
		}
	case []any:
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if nodeID, status, ok := readFromMap(mapped); ok {
				return nodeID, status, true
			}
		}
	}
	return "", "", false
}
