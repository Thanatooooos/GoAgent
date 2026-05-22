package system

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func DocumentQueryBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`The document is in a pipeline abnormal state:
Current result: document_query says document doc-1 is processMode=pipeline and status=failed.
Return: {"done":false,"reasoning":"The document state suggests a pipeline failure or in-progress task, so document-level diagnosis is needed next.","state":{"phase":"initial_diagnosis","hypothesis":"document doc-1 entered an abnormal pipeline state","confidence":0.35,"openQuestions":["Why did the pipeline document enter this state?"],"checkedTools":["document_query"],"nextHintCalls":[{"name":"document_ingestion_diagnose","arguments":{"documentId":"doc-1"}}]}}`,
		},
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
		ObserverExamples: []string{
			`Chunk log shows an abnormal ingestion record with a task id:
Current result: document_chunk_log_query says latestTaskId=task-1 and latestStatus=failed.
Return: {"done":false,"reasoning":"The chunk log is abnormal and already points to a task. Inspect the task detail next.","state":{"phase":"deep_dive","hypothesis":"task task-1 contains the more precise ingestion failure evidence","confidence":0.52,"openQuestions":["Which ingestion node caused the abnormal chunk-log state?"],"checkedTools":["document_chunk_log_query"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}`,
		},
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
		ObserverExamples: []string{
			`The document list answered the question directly:
Current result: document_list returns several matching documents.
Return: {"done":true,"reasoning":"The document list already answers the user's request directly.","state":{"phase":"complete","hypothesis":"the matching document list is sufficient","confidence":0.7,"openQuestions":[],"checkedTools":["document_list"],"nextHintCalls":[]}}`,
		},
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

func DocumentIngestionDiagnoseBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		ObserverExamples: []string{
			`The document diagnosis found the failed node, but not the node detail:
Current result: document_ingestion_diagnose returns latestTaskId=task-1, latestNodeId=indexer, conclusion="failed at node indexer", and latestNodeError is empty.
Return: {"done":false,"reasoning":"The failed node is known, but the concrete node-level error message is still missing.","state":{"phase":"deep_dive","hypothesis":"indexer failed but its concrete error is still unknown","confidence":0.72,"openQuestions":["What is the concrete node-level error message?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}]}}`,
		},
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

func renderDocumentListContext(result ragcore.Result) string {
	view, ok := ViewDocumentListResult(result)
	if !ok || strings.TrimSpace(result.Summary) == "" || len(view.Items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(view.Items))
	for idx, item := range view.Items {
		documentID := strings.TrimSpace(item.DocumentID)
		name := strings.TrimSpace(item.Name)
		status := strings.TrimSpace(item.Status)
		processMode := strings.TrimSpace(item.ProcessMode)
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
