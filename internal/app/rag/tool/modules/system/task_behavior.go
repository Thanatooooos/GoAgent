package system

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func TaskListBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`The task list answered the question directly:
Current result: task_list returns several matching tasks.
Return: {"done":true,"reasoning":"The task list already answers the user's request directly.","state":{"phase":"complete","hypothesis":"the matching task list is sufficient","confidence":0.7,"openQuestions":[],"checkedTools":["task_list"],"nextHintCalls":[]}}`,
		},
		Next: func(result ragcore.Result, input ragcore.WorkflowInput) ragcore.NextDecision {
			total := result.GetInt("total")
			if total == 0 && ragcore.KnowledgeBaseInsufficient(input.RetrieveResult) {
				return ragcore.NextDecision{
					Done:     false,
					Reason:   "task_list_kb_insufficient",
					Terminal: false,
					HintCalls: []ragcore.HintCall{{
						Name:      "external_evidence_workflow",
						Arguments: map[string]any{"question": input.Question},
					}},
				}
			}
			return ragcore.NextDecision{Done: true, Reason: "task_list_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, input ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			listTotal := result.GetInt("total")
			if listTotal == 0 && ragcore.KnowledgeBaseInsufficient(input.RetrieveResult) {
				return ragcore.NewObserveResult(false, "Knowledge base returned no matching tasks, and retrieval results are also insufficient. Trying the external evidence workflow.", ragcore.ObserveState(
					"external_search",
					"knowledge base has limited or no relevant content",
					0.3,
					[]ragcore.HintCall{{Name: "external_evidence_workflow", Arguments: map[string]any{"question": input.Question}}},
					[]string{result.Name},
				)), true
			}
			return ragcore.NewObserveResult(true, "The task list returned results, so the agent can answer directly.", ragcore.ObserveState(
				"complete",
				strings.TrimSpace(result.Summary),
				0.7,
				nil,
				[]string{result.Name},
			)), true
		},
		RenderContext: renderTaskListContext,
	}
}

func IngestionTaskQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`Task query shows a failed node summary:
Current result: ingestion_task_query says task-1 status=failed and taskNodeSummary includes indexer(status=failed).
Return: {"done":false,"reasoning":"The task summary already points to a failed node. Inspect that node directly next.","state":{"phase":"deep_dive","hypothesis":"task task-1 failed at node indexer","confidence":0.7,"openQuestions":["What is the concrete error for this failed node?"],"checkedTools":["ingestion_task_query"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}]}}`,
		},
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return nextDecisionFromHint(nextActionTaskQuery(result))
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeTaskQuery(result), true
		},
	}
}

func IngestionTaskNodeQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`Task node detail is already enough to answer:
Current result: ingestion_task_node_query says node indexer failed with a concrete error message.
Return: {"done":true,"reasoning":"Node-level task evidence is already available.","state":{"phase":"complete","hypothesis":"the task failed at the returned node for the captured reason","confidence":1.0,"openQuestions":[],"checkedTools":["ingestion_task_node_query"],"nextHintCalls":[]}}`,
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "ingestion_task_node_query_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return ragcore.NewObserveResult(true, "The task node details are already available, so the agent can answer directly.", ragcore.ObserveState(
				"complete",
				result.GetString("errorMessage"),
				1,
				nil,
				[]string{result.Name},
			)), true
		},
		RenderContext: renderTaskNodeContext,
	}
}

func TaskIngestionDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`Task diagnosis says the task is still running:
Current result: task_ingestion_diagnose says task-1 is still running and no node-level error is available.
Return: {"done":false,"reasoning":"The task is still running, so verify the live task detail instead of assuming a final failure.","state":{"phase":"verification","hypothesis":"the task is still in progress rather than failed at a confirmed node","confidence":0.45,"openQuestions":["Which node is still running right now?"],"checkedTools":["task_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}`,
		},
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewDiagnosisResult(result)
			if !ok {
				return nil, fmt.Errorf("task_ingestion_diagnose result view unavailable")
			}
			return view, nil
		},
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return nextDecisionFromHint(nextActionTaskDiagnosis(result))
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeTaskDiagnosis(result), true
		},
		RenderContext: renderDiagnosisContext,
	}
}

func nextActionTaskQuery(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	view, ok := ViewIngestionTaskQueryResult(result)
	if !ok {
		return nil, true, "task_result_unavailable"
	}
	taskID := view.TaskID
	status := strings.ToLower(view.Status)
	if taskID == "" {
		return nil, true, "task_id_missing"
	}
	if node, found := view.LatestInterestingNode(); found {
		return &ragcore.HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": taskID, "nodeId": node.NodeID}}, false, "interesting_node_found"
	}
	if status == "failed" || status == "running" {
		return &ragcore.HintCall{Name: "task_ingestion_diagnose", Arguments: map[string]any{"taskId": taskID}}, false, "task_abnormal"
	}
	return nil, true, "task_stable"
}

func nextActionTaskDiagnosis(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	taskID := result.GetString("taskId")
	latestNodeID := result.GetString("latestNodeId")
	latestNodeError := result.GetString("latestNodeError")
	conclusion := strings.ToLower(result.GetString("conclusion"))
	if latestNodeError != "" {
		return nil, true, "node_error_found"
	}
	if taskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		return &ragcore.HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": taskID, "nodeId": latestNodeID}}, false, "failed_node_located"
	}
	if taskID != "" && strings.Contains(conclusion, "still running") {
		return &ragcore.HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": taskID, "includeNodes": true}}, false, "still_running"
	}
	return nil, true, "diagnosis_complete"
}

func observeTaskDiagnosis(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionTaskDiagnosis(result)
	conclusion := result.GetString("conclusion")
	confidence := strings.ToLower(result.GetString("confidence"))
	switch reason {
	case "node_error_found":
		return ragcore.NewObserveResult(true, "The task diagnosis already includes the node error, so the agent can answer directly.", ragcore.ObserveState("complete", conclusion, 1, nil, []string{result.Name}))
	case "failed_node_located":
		return ragcore.NewObserveResult(false, "The task diagnosis located the failed node, but the agent still needs the node detail before answering.", ragcore.ObserveState("deep_dive", conclusion, 0.72, hintCallToSlice(hintCall), []string{result.Name}, "What is the concrete node-level error message?"))
	case "still_running":
		return ragcore.NewObserveResult(false, "The task diagnosis shows the task is still running, so the next step is to inspect the live task detail.", ragcore.ObserveState("verification", conclusion, 0.45, hintCallToSlice(hintCall), []string{result.Name}, "Which node is still running right now?"))
	default:
		if confidence == "high" {
			return ragcore.NewObserveResult(true, "The task diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", ragcore.ObserveState("complete", conclusion, 0.9, nil, []string{result.Name}))
		}
		return ragcore.NewObserveResult(true, "The task diagnosis already gathered the main evidence needed for the final answer.", ragcore.ObserveState("complete", conclusion, 0.75, nil, []string{result.Name}))
	}
}

func observeTaskQuery(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionTaskQuery(result)
	view, ok := ViewIngestionTaskQueryResult(result)
	if !ok {
		return ragcore.NewObserveResult(true, "The task query result could not be decoded into a structured view, so the agent loop stops here.", ragcore.ObserveState("complete", "task query result view unavailable", 0.2, nil, []string{result.Name}))
	}
	taskID := view.TaskID
	status := strings.ToLower(view.Status)
	switch reason {
	case "task_id_missing":
		return ragcore.NewObserveResult(true, "The task query result is missing taskId, so the agent loop stops here.", ragcore.ObserveState("complete", "task query result is missing taskId", 0.2, nil, []string{result.Name}))
	case "interesting_node_found":
		node, found := view.LatestInterestingNode()
		if !found {
			return ragcore.NewObserveResult(true, "The task query no longer exposes a structured interesting node, so the agent loop stops here.", ragcore.ObserveState("complete", fmt.Sprintf("task %s is stable with status %s", taskID, status), 0.4, nil, []string{result.Name}))
		}
		if strings.EqualFold(strings.TrimSpace(node.Status), "running") {
			return ragcore.NewObserveResult(false, "The task query exposed a running node, so the next step is to inspect that live node instead of assuming a failure.", ragcore.ObserveState("verification", fmt.Sprintf("task %s is still running at node %s", taskID, node.NodeID), 0.55, hintCallToSlice(hintCall), []string{result.Name}, "What is the live status of this running node?"))
		}
		return ragcore.NewObserveResult(false, "The task query already exposed a failed node, so the next step is to inspect that node directly.", ragcore.ObserveState("deep_dive", fmt.Sprintf("task %s failed at node %s", taskID, node.NodeID), 0.7, hintCallToSlice(hintCall), []string{result.Name}, "What is the concrete error for this failed node?"))
	case "task_abnormal":
		return ragcore.NewObserveResult(false, "The task is still failed or running, so continue with task-level diagnosis before answering.", ragcore.ObserveState("verification", fmt.Sprintf("task %s is currently %s", taskID, status), 0.45, hintCallToSlice(hintCall), []string{result.Name}, "Which node or condition explains the current task status?"))
	default:
		return ragcore.NewObserveResult(true, "The task query already returned a stable task state, so the agent loop can stop.", ragcore.ObserveState("complete", fmt.Sprintf("task %s is stable with status %s", taskID, status), 0.8, nil, []string{result.Name}))
	}
}

func renderTaskListContext(result ragcore.Result) string {
	view, ok := ViewTaskListResult(result)
	if !ok || len(view.Items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(view.Items))
	for idx, item := range view.Items {
		taskID := strings.TrimSpace(item.TaskID)
		pipelineID := strings.TrimSpace(item.PipelineID)
		status := strings.TrimSpace(item.Status)
		sourceFileName := strings.TrimSpace(item.SourceFileName)
		if taskID == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s pipeline=%s status=%s source=%s", idx+1, taskID, pipelineID, status, sourceFileName))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Task list:\n" + strings.Join(lines, "\n")
}

func renderTaskNodeContext(result ragcore.Result) string {
	view, ok := ViewIngestionTaskNodeQueryResult(result)
	if !ok {
		return ""
	}
	if nodeID := strings.TrimSpace(view.NodeID); nodeID != "" {
		return fmt.Sprintf("Task node detail:\nnode=%s status=%s error=%s", nodeID, view.Status, view.ErrorMessage)
	}
	if len(view.Nodes) == 0 {
		return ""
	}
	lines := make([]string, 0, len(view.Nodes))
	for idx, node := range view.Nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		status := strings.TrimSpace(node.Status)
		nodeType := strings.TrimSpace(node.NodeType)
		if nodeID == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s type=%s status=%s", idx+1, nodeID, nodeType, status))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Task nodes:\n" + strings.Join(lines, "\n")
}
