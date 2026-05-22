package trace

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func TraceRetrievalDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewTraceRetrievalDiagnoseResult(result)
			if !ok {
				return nil, fmt.Errorf("trace_retrieval_diagnose result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "trace_retrieval_diagnose_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			view, ok := ViewTraceRetrievalDiagnoseResult(result)
			if !ok {
				return ragcore.ObserveResult{}, false
			}
			return ragcore.NewObserveResult(true, "The retrieval diagnosis already provides enough trace-level evidence to answer directly.", ragcore.ObserveState(
				"complete",
				ragcore.FirstNonEmpty(view.Conclusion, result.Summary, result.ErrorMessage),
				1,
				nil,
				[]string{result.Name},
			)), true
		},
		RenderContext: renderTraceRetrievalDiagnoseContext,
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
		RenderContext: renderTraceNodeQueryContext,
	}
}

func renderTraceNodeQueryContext(result ragcore.Result) string {
	view, ok := ViewTraceNodeQueryResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, len(view.Nodes)+2)
	if view.ErrorMessage != "" {
		lines = append(lines, "Trace error: "+view.ErrorMessage)
	}
	for idx, node := range view.Nodes {
		label := ragcore.FirstNonEmpty(strings.TrimSpace(node.NodeName), strings.TrimSpace(node.NodeID))
		if label == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s type=%s status=%s", idx+1, label, strings.TrimSpace(node.NodeType), strings.TrimSpace(node.Status)))
		if summary := renderTraceNodeSummary(node); summary != "" {
			lines = append(lines, "   "+summary)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Trace nodes:\n" + strings.Join(lines, "\n")
}

func renderTraceNodeSummary(node TraceNodeItemView) string {
	if node.MemoryRecall != nil {
		if summary := strings.TrimSpace(node.MemoryRecall.Summary); summary != "" {
			return "memory recall: " + summary
		}
	}
	if summary := strings.TrimSpace(node.Summary); summary != "" {
		return "summary: " + summary
	}
	return ""
}

func renderTraceRetrievalDiagnoseContext(result ragcore.Result) string {
	view, ok := ViewTraceRetrievalDiagnoseResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, 4)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if len(view.Facts) > 0 {
		lines = append(lines, "Facts:\n- "+strings.Join(view.Facts, "\n- "))
	}
	if len(view.NextActions) > 0 {
		lines = append(lines, "Suggested next actions:\n- "+strings.Join(view.NextActions, "\n- "))
	}
	return strings.Join(lines, "\n")
}
