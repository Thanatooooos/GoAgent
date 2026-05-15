package system

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func DocumentQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return nextDecisionFromHint(nextActionDocumentQuery(result))
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeDocumentQuery(result), true
		},
	}
}

func DocumentChunkLogQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return nextDecisionFromHint(nextActionChunkLogQuery(result))
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeChunkLogQuery(result), true
		},
	}
}

func DocumentListBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Next: func(result ragcore.Result, input ragcore.WorkflowInput) ragcore.NextDecision {
			total := result.GetInt("total")
			if total == 0 && ragcore.KnowledgeBaseInsufficient(input.RetrieveResult) {
				return ragcore.NextDecision{
					Done:     false,
					Reason:   "document_list_kb_insufficient",
					Terminal: false,
					HintCalls: []ragcore.HintCall{{
						Name:      "external_evidence_workflow",
						Arguments: map[string]any{"question": input.Question},
					}},
				}
			}
			return ragcore.NextDecision{Done: true, Reason: "document_list_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, input ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			listTotal := result.GetInt("total")
			if listTotal == 0 && ragcore.KnowledgeBaseInsufficient(input.RetrieveResult) {
				return ragcore.NewObserveResult(false, "Knowledge base returned no matching documents, and retrieval results are also insufficient. Trying the external evidence workflow.", ragcore.ObserveState(
					"external_search",
					"knowledge base has limited or no relevant content",
					0.3,
					[]ragcore.HintCall{{Name: "external_evidence_workflow", Arguments: map[string]any{"question": input.Question}}},
					[]string{result.Name},
				)), true
			}
			return ragcore.NewObserveResult(true, "The document list returned results, so the agent can answer directly.", ragcore.ObserveState(
				"complete",
				strings.TrimSpace(result.Summary),
				0.7,
				nil,
				[]string{result.Name},
			)), true
		},
		RenderContext: renderDocumentListContext,
	}
}

func TaskListBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
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

func DocumentIngestionDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewDiagnosisResult(result)
			if !ok {
				return nil, fmt.Errorf("document_ingestion_diagnose result view unavailable")
			}
			return view, nil
		},
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return nextDecisionFromHint(nextActionDocumentDiagnosis(result))
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeDocumentDiagnosis(result), true
		},
		RenderContext: renderDiagnosisContext,
	}
}

func TaskIngestionDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
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

func nextDecisionFromHint(hintCall *ragcore.HintCall, done bool, reason string) ragcore.NextDecision {
	decision := ragcore.NextDecision{Done: done, Reason: reason, Terminal: done}
	if hintCall != nil {
		decision.HintCalls = []ragcore.HintCall{*hintCall}
		decision.Done = false
		decision.Terminal = false
	}
	return decision
}

func nextActionDocumentQuery(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	documentID := result.GetString("documentId")
	status := strings.ToLower(result.GetString("status"))
	processMode := strings.ToLower(result.GetString("processMode"))
	if documentID != "" && processMode == "pipeline" && (status == "failed" || status == "running") {
		return &ragcore.HintCall{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": documentID}}, false, "pipeline_document_abnormal"
	}
	return nil, true, "document_stable"
}

func nextActionChunkLogQuery(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	documentID := result.GetString("documentId")
	latestTaskID := result.GetString("latestTaskId")
	latestStatus := strings.ToLower(result.GetString("latestStatus"))
	latestError := result.GetString("latestError")
	failedLogCount := result.GetInt("failedLogCount")
	runningLogCount := result.GetInt("runningLogCount")
	isAbnormal := latestStatus == "failed" || latestStatus == "running" || failedLogCount > 0 || runningLogCount > 0 || latestError != ""
	if !isAbnormal {
		return nil, true, "chunk_log_stable"
	}
	if latestTaskID != "" {
		return &ragcore.HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "chunk_log_abnormal_with_task"
	}
	if documentID != "" {
		return &ragcore.HintCall{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": documentID}}, false, "chunk_log_abnormal_no_task"
	}
	return nil, true, "chunk_log_stable"
}

func nextActionDocumentDiagnosis(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	latestTaskID := result.GetString("latestTaskId")
	latestNodeID := result.GetString("latestNodeId")
	latestNodeError := result.GetString("latestNodeError")
	latestLogError := result.GetString("latestLogError")
	conclusion := strings.ToLower(result.GetString("conclusion"))
	if latestNodeError != "" {
		return nil, true, "node_error_found"
	}
	if latestTaskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		return &ragcore.HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": latestTaskID, "nodeId": latestNodeID}}, false, "failed_node_located"
	}
	if latestTaskID != "" && latestLogError != "" {
		return &ragcore.HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "task_level_error_only"
	}
	if latestTaskID != "" && (strings.Contains(conclusion, "still running") || strings.Contains(conclusion, "inconsistent")) {
		return &ragcore.HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "still_running_or_inconsistent"
	}
	return nil, true, "diagnosis_complete"
}

func nextActionTaskQuery(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	taskID := result.GetString("taskId")
	status := strings.ToLower(result.GetString("status"))
	if taskID == "" {
		return nil, true, "task_id_missing"
	}
	if nodeID, _, ok := LatestInterestingTaskNode(result.Data); ok {
		return &ragcore.HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": taskID, "nodeId": nodeID}}, false, "interesting_node_found"
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

func LatestInterestingTaskNode(data map[string]any) (string, string, bool) {
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

func observeDocumentDiagnosis(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionDocumentDiagnosis(result)
	conclusion := result.GetString("conclusion")
	confidence := strings.ToLower(result.GetString("confidence"))
	switch reason {
	case "node_error_found":
		return ragcore.NewObserveResult(true, "The document diagnosis already includes a node-level error, so the agent can answer directly.", ragcore.ObserveState("complete", conclusion, 1, nil, []string{result.Name}))
	case "failed_node_located":
		return ragcore.NewObserveResult(false, "The document diagnosis located the failed node, but the agent still needs the node details before answering.", ragcore.ObserveState("deep_dive", conclusion, 0.72, hintCallToSlice(hintCall), []string{result.Name}, "What is the concrete node-level error message?"))
	case "task_level_error_only":
		return ragcore.NewObserveResult(false, "The document diagnosis only has a task or chunk-log level error summary, so the next step is to inspect the task detail.", ragcore.ObserveState("deep_dive", conclusion, 0.58, hintCallToSlice(hintCall), []string{result.Name}, "Which task node actually failed?", "Is there a node-level error message?"))
	case "still_running_or_inconsistent":
		return ragcore.NewObserveResult(false, "The document diagnosis suggests the task is still running or the states are inconsistent, so the next step is to inspect the task detail.", ragcore.ObserveState("verification", conclusion, 0.45, hintCallToSlice(hintCall), []string{result.Name}, "Is the task still running or already blocked on a node?"))
	default:
		if confidence == "high" {
			return ragcore.NewObserveResult(true, "The document diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", ragcore.ObserveState("complete", conclusion, 0.9, nil, []string{result.Name}))
		}
		return ragcore.NewObserveResult(true, "The document diagnosis already gathered the main evidence needed for the final answer.", ragcore.ObserveState("complete", conclusion, 0.75, nil, []string{result.Name}))
	}
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

func observeDocumentQuery(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionDocumentQuery(result)
	documentID := result.GetString("documentId")
	status := strings.ToLower(result.GetString("status"))
	switch reason {
	case "pipeline_document_abnormal":
		return ragcore.NewObserveResult(false, "The document state suggests a pipeline failure or an in-progress task, so continue with document-level diagnosis.", ragcore.ObserveState("initial_diagnosis", fmt.Sprintf("document %s is in pipeline status %s", documentID, status), 0.35, hintCallToSlice(hintCall), []string{result.Name}, "Why did the pipeline document enter this state?"))
	default:
		return ragcore.NewObserveResult(true, "The document query already returned a stable document state, so the agent loop can stop.", ragcore.ObserveState("complete", fmt.Sprintf("document state is %s", status), 0.8, nil, []string{result.Name}))
	}
}

func observeChunkLogQuery(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionChunkLogQuery(result)
	documentID := result.GetString("documentId")
	latestTaskID := result.GetString("latestTaskId")
	latestStatus := strings.ToLower(result.GetString("latestStatus"))
	switch reason {
	case "chunk_log_abnormal_with_task":
		return ragcore.NewObserveResult(false, "The chunk log shows a failed or running ingestion record, so the next step is to inspect the task detail.", ragcore.ObserveState("deep_dive", fmt.Sprintf("task %s has abnormal chunk-log state %s", latestTaskID, latestStatus), 0.52, hintCallToSlice(hintCall), []string{result.Name}, "Which ingestion node caused the abnormal chunk-log state?"))
	case "chunk_log_abnormal_no_task":
		return ragcore.NewObserveResult(false, "The chunk log shows an abnormal ingestion record, but a task id is not available, so continue with document-level diagnosis.", ragcore.ObserveState("initial_diagnosis", fmt.Sprintf("document %s has abnormal chunk-log state %s", documentID, latestStatus), 0.4, hintCallToSlice(hintCall), []string{result.Name}, "Which ingestion task is associated with this abnormal chunk-log state?"))
	default:
		return ragcore.NewObserveResult(true, "The chunk log did not expose a new abnormal state that requires deeper drilling.", ragcore.ObserveState("complete", "chunk log is stable", 0.75, nil, []string{result.Name}))
	}
}

func observeTaskQuery(result ragcore.Result) ragcore.ObserveResult {
	hintCall, _, reason := nextActionTaskQuery(result)
	taskID := result.GetString("taskId")
	status := strings.ToLower(result.GetString("status"))
	switch reason {
	case "task_id_missing":
		return ragcore.NewObserveResult(true, "The task query result is missing taskId, so the agent loop stops here.", ragcore.ObserveState("complete", "task query result is missing taskId", 0.2, nil, []string{result.Name}))
	case "interesting_node_found":
		nodeID, nodeStatus, _ := LatestInterestingTaskNode(result.Data)
		if nodeStatus == "running" {
			return ragcore.NewObserveResult(false, "The task query exposed a running node, so the next step is to inspect that live node instead of assuming a failure.", ragcore.ObserveState("verification", fmt.Sprintf("task %s is still running at node %s", taskID, nodeID), 0.55, hintCallToSlice(hintCall), []string{result.Name}, "What is the live status of this running node?"))
		}
		return ragcore.NewObserveResult(false, "The task query already exposed a failed node, so the next step is to inspect that node directly.", ragcore.ObserveState("deep_dive", fmt.Sprintf("task %s failed at node %s", taskID, nodeID), 0.7, hintCallToSlice(hintCall), []string{result.Name}, "What is the concrete error for this failed node?"))
	case "task_abnormal":
		return ragcore.NewObserveResult(false, "The task is still failed or running, so continue with task-level diagnosis before answering.", ragcore.ObserveState("verification", fmt.Sprintf("task %s is currently %s", taskID, status), 0.45, hintCallToSlice(hintCall), []string{result.Name}, "Which node or condition explains the current task status?"))
	default:
		return ragcore.NewObserveResult(true, "The task query already returned a stable task state, so the agent loop can stop.", ragcore.ObserveState("complete", fmt.Sprintf("task %s is stable with status %s", taskID, status), 0.8, nil, []string{result.Name}))
	}
}

func hintCallToSlice(hintCall *ragcore.HintCall) []ragcore.HintCall {
	if hintCall == nil {
		return nil
	}
	return []ragcore.HintCall{*hintCall}
}

func renderDocumentListContext(result ragcore.Result) string {
	if strings.TrimSpace(result.Summary) == "" {
		return ""
	}
	items := ragcore.ReadMapItems(result.Data["items"])
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for idx, item := range items {
		documentID := strings.TrimSpace(ragcore.ReadDataString(item, "documentId"))
		name := strings.TrimSpace(ragcore.ReadDataString(item, "name"))
		status := strings.TrimSpace(ragcore.ReadDataString(item, "status"))
		processMode := strings.TrimSpace(ragcore.ReadDataString(item, "processMode"))
		if documentID == "" && name == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s) status=%s processMode=%s", idx+1, ragcore.FirstNonEmpty(name, documentID), documentID, status, processMode))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Document list:\n" + strings.Join(lines, "\n")
}

func renderTaskListContext(result ragcore.Result) string {
	items := ragcore.ReadMapItems(result.Data["items"])
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for idx, item := range items {
		taskID := strings.TrimSpace(ragcore.ReadDataString(item, "taskId"))
		pipelineID := strings.TrimSpace(ragcore.ReadDataString(item, "pipelineId"))
		status := strings.TrimSpace(ragcore.ReadDataString(item, "status"))
		sourceFileName := strings.TrimSpace(ragcore.ReadDataString(item, "sourceFileName"))
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
	if nodeID := strings.TrimSpace(result.GetString("nodeId")); nodeID != "" {
		return fmt.Sprintf("Task node detail:\nnode=%s status=%s error=%s", nodeID, result.GetString("status"), result.GetString("errorMessage"))
	}
	nodes := ragcore.ReadMapItems(result.Data["nodes"])
	if len(nodes) == 0 {
		return ""
	}
	lines := make([]string, 0, len(nodes))
	for idx, node := range nodes {
		nodeID := strings.TrimSpace(ragcore.ReadDataString(node, "nodeId"))
		status := strings.TrimSpace(ragcore.ReadDataString(node, "status"))
		nodeType := strings.TrimSpace(ragcore.ReadDataString(node, "nodeType"))
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

func renderDiagnosisContext(result ragcore.Result) string {
	view, ok := ViewDiagnosisResult(result)
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
