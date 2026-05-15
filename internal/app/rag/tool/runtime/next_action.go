package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

var nextActionRegistry *Registry

// SetNextActionRegistry provides a Registry for nextAction lookups.
// Set once during assembly; nextAction uses it to resolve tool behaviors
// instead of the old hardcoded switch.
func SetNextActionRegistry(r *Registry) {
	nextActionRegistry = r
}

// nextAction returns the next tool to call based on the latest tool result.
func nextAction(result Result) (hintCall *HintCall, done bool, reason string) {
	if nextActionRegistry != nil {
		if behavior, ok := nextActionRegistry.GetBehavior(result.Name); ok && behavior.Next != nil {
			return nextActionFromDecision(behavior.Next(result, WorkflowInput{}))
		}
	}
	return nil, true, "terminal"
}

func nextActionFromDecision(decision NextDecision) (hintCall *HintCall, done bool, reason string) {
	hints := NormalizeHintCalls(decision.HintCalls)
	if len(hints) > 0 {
		return &hints[0], false, strings.TrimSpace(decision.Reason)
	}
	if decision.Done || decision.Terminal || strings.TrimSpace(decision.Reason) != "" {
		return nil, decision.Done || decision.Terminal, strings.TrimSpace(decision.Reason)
	}
	return nil, true, "terminal"
}

func nextActionDocumentQuery(result Result) (*HintCall, bool, string) {
	documentID := result.GetString("documentId")
	status := strings.ToLower(result.GetString("status"))
	processMode := strings.ToLower(result.GetString("processMode"))
	if documentID != "" && processMode == "pipeline" && (status == "failed" || status == "running") {
		return &HintCall{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": documentID}}, false, "pipeline_document_abnormal"
	}
	return nil, true, "document_stable"
}

func nextActionChunkLogQuery(result Result) (*HintCall, bool, string) {
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
		return &HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "chunk_log_abnormal_with_task"
	}
	if documentID != "" {
		return &HintCall{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": documentID}}, false, "chunk_log_abnormal_no_task"
	}
	return nil, true, "chunk_log_stable"
}

func nextActionDocumentDiagnosis(result Result) (*HintCall, bool, string) {
	latestTaskID := result.GetString("latestTaskId")
	latestNodeID := result.GetString("latestNodeId")
	latestNodeError := result.GetString("latestNodeError")
	latestLogError := result.GetString("latestLogError")
	conclusion := strings.ToLower(result.GetString("conclusion"))

	if latestNodeError != "" {
		return nil, true, "node_error_found"
	}
	if latestTaskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		return &HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": latestTaskID, "nodeId": latestNodeID}}, false, "failed_node_located"
	}
	if latestTaskID != "" && latestLogError != "" {
		return &HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "task_level_error_only"
	}
	if latestTaskID != "" && (strings.Contains(conclusion, "still running") || strings.Contains(conclusion, "inconsistent")) {
		return &HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": latestTaskID, "includeNodes": true}}, false, "still_running_or_inconsistent"
	}
	return nil, true, "diagnosis_complete"
}

func nextActionTaskQuery(result Result) (*HintCall, bool, string) {
	taskID := result.GetString("taskId")
	status := strings.ToLower(result.GetString("status"))
	if taskID == "" {
		return nil, true, "task_id_missing"
	}
	if nodeID, _, ok := latestInterestingTaskNode(result.Data); ok {
		return &HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": taskID, "nodeId": nodeID}}, false, "interesting_node_found"
	}
	if status == "failed" || status == "running" {
		return &HintCall{Name: "task_ingestion_diagnose", Arguments: map[string]any{"taskId": taskID}}, false, "task_abnormal"
	}
	return nil, true, "task_stable"
}

func nextActionWebSearch(result Result) (*HintCall, bool, string) {
	view, ok := webmod.ViewWebSearchResult(result)
	results := result.GetStringSlice("urls")
	if len(results) == 0 && ok {
		results = view.FetchableURLs(3)
	}
	if len(results) == 0 && ok {
		results = view.URLs(3)
	}
	if len(results) == 0 {
		results = extractURLsFromWebSearchData(result.Data)
	}
	if len(results) == 0 {
		return nil, true, "web_search_no_results"
	}
	if len(results) > 3 {
		results = results[:3]
	}
	return &HintCall{
		Name: "web_fetch",
		Arguments: map[string]any{
			"urls": results,
		},
	}, false, "web_search_has_results"
}

func extractURLsFromWebSearchData(data map[string]any) []string {
	view, _ := webmod.ViewWebSearchResult(Result{Name: "web_search", Data: data})
	urls := view.FetchableURLs(0)
	if len(urls) > 0 {
		return urls
	}
	return view.URLs(0)
}

func hintCallToSlice(hintCall *HintCall) []HintCall {
	if hintCall == nil {
		return nil
	}
	return []HintCall{*hintCall}
}

func nextActionTaskDiagnosis(result Result) (*HintCall, bool, string) {
	taskID := result.GetString("taskId")
	latestNodeID := result.GetString("latestNodeId")
	latestNodeError := result.GetString("latestNodeError")
	conclusion := strings.ToLower(result.GetString("conclusion"))

	if latestNodeError != "" {
		return nil, true, "node_error_found"
	}
	if taskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		return &HintCall{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": taskID, "nodeId": latestNodeID}}, false, "failed_node_located"
	}
	if taskID != "" && strings.Contains(conclusion, "still running") {
		return &HintCall{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": taskID, "includeNodes": true}}, false, "still_running"
	}
	return nil, true, "diagnosis_complete"
}
