package tool

import (
	"context"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
)

// Observer decides whether the agent loop has enough evidence to stop.
type Observer interface {
	Observe(ctx context.Context, input ObserveInput) (ObserveResult, error)
}

// ObserveInput describes the context available to the observe phase.
type ObserveInput struct {
	Question         string
	Round            int
	Results          []Result
	RoundResults     []Result
	PreviousState    AgentState
	MaxIterations    int
	ReachedMaxLoop   bool
	ToolDefinitions  []Definition
	KnowledgeBaseIDs []string
	RewriteResult    ragrewrite.Result
	RetrieveResult   ragretrieve.Result
}

// ObserveResult describes the decision produced by the observe phase.
type ObserveResult struct {
	Done          bool
	Reasoning     string
	NextHintCalls []HintCall
	NextHint      string
	Confidence    float64
	State         AgentState
}

func readHintArg(arguments map[string]any, key string) string {
	if value := strings.TrimSpace(readStringArg(arguments, key)); value != "" {
		return value
	}
	if readBoolArg(arguments, key) {
		return "true"
	}
	return ""
}

func newObserveResult(done bool, reasoning string, state AgentState) ObserveResult {
	normalized := state.Normalize()
	return ObserveResult{
		Done:          done,
		Reasoning:     strings.TrimSpace(reasoning),
		NextHintCalls: append([]HintCall(nil), normalized.NextHintCalls...),
		NextHint:      normalized.NextHint,
		Confidence:    normalized.Confidence,
		State:         normalized,
	}
}

func observeState(phase string, hypothesis string, confidence float64, nextHintCalls []HintCall, checkedTools []string, openQuestions ...string) AgentState {
	return AgentState{
		Phase:         phase,
		Hypothesis:    hypothesis,
		Confidence:    confidence,
		OpenQuestions: openQuestions,
		CheckedTools:  checkedTools,
		NextHintCalls: nextHintCalls,
	}.Normalize()
}

// RuleObserver is the lightweight V1 observer built on top of tool results.
type RuleObserver struct{}

func NewRuleObserver() *RuleObserver {
	return &RuleObserver{}
}

func (o *RuleObserver) Observe(_ context.Context, input ObserveInput) (ObserveResult, error) {
	if len(input.RoundResults) == 0 {
		return newObserveResult(true, "No new tool results were produced in this round, so the agent loop stops here.", AgentState{
			Phase:        "complete",
			CheckedTools: input.PreviousState.CheckedTools,
		}), nil
	}
	if input.ReachedMaxLoop {
		return newObserveResult(true, fmt.Sprintf("The agent loop already reached the maximum of %d iterations, so it must answer with the current evidence.", input.MaxIterations), AgentState{
			Phase:         "complete",
			Confidence:    input.PreviousState.Confidence,
			Hypothesis:    input.PreviousState.Hypothesis,
			CheckedTools:  input.PreviousState.CheckedTools,
			NextHintCalls: input.PreviousState.NextHintCalls,
			NextHint:      input.PreviousState.NextHint,
		}), nil
	}

	latest, ok := lastNonThinkResult(input.RoundResults)
	if !ok {
		return newObserveResult(true, "All tool results in this round were think calls; no diagnostic evidence to evaluate.", AgentState{
			Phase:        "complete",
			CheckedTools: input.PreviousState.CheckedTools,
		}), nil
	}
	switch strings.TrimSpace(latest.Name) {
	case "document_ingestion_diagnose":
		return observeDocumentDiagnosis(latest), nil
	case "task_ingestion_diagnose":
		return observeTaskDiagnosis(latest), nil
	case "trace_retrieval_diagnose":
		return newObserveResult(true, "The retrieval diagnosis already provides enough trace-level evidence to answer directly.", observeState(
			"complete",
			readDataString(latest.Data, "conclusion"),
			1,
			nil,
			[]string{latest.Name},
		)), nil
	case "document_query":
		return observeDocumentQuery(latest), nil
	case "document_chunk_log_query":
		return observeChunkLogQuery(latest), nil
	case "ingestion_task_query":
		return observeTaskQuery(latest), nil
	case "ingestion_task_node_query":
		return newObserveResult(true, "The task node details are already available, so the agent can answer directly.", observeState(
			"complete",
			readDataString(latest.Data, "errorMessage"),
			1,
			nil,
			[]string{latest.Name},
		)), nil
	case "trace_node_query":
		return newObserveResult(true, "The trace node details are already available, so the agent can answer directly.", observeState(
			"complete",
			readDataString(latest.Data, "summary"),
			1,
			nil,
			[]string{latest.Name},
		)), nil
	default:
		return newObserveResult(true, "The current tool result is already sufficient as supporting context, so the agent loop stops here.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.6,
			nil,
			[]string{latest.Name},
		)), nil
	}
}

func observeDocumentDiagnosis(result Result) ObserveResult {
	latestTaskID := readDataString(result.Data, "latestTaskId")
	latestNodeID := readDataString(result.Data, "latestNodeId")
	latestNodeError := readDataString(result.Data, "latestNodeError")
	latestLogError := readDataString(result.Data, "latestLogError")
	conclusion := strings.ToLower(readDataString(result.Data, "conclusion"))
	confidence := strings.ToLower(readDataString(result.Data, "confidence"))

	if latestNodeError != "" {
		return newObserveResult(true, "The document diagnosis already includes a node-level error, so the agent can answer directly.", observeState(
			"complete",
			readDataString(result.Data, "conclusion"),
			1,
			nil,
			[]string{result.Name},
		))
	}
	if latestTaskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		nextHintCalls := buildHintCall("ingestion_task_node_query", map[string]any{
			"taskId": latestTaskID,
			"nodeId": latestNodeID,
		})
		return newObserveResult(false, "The document diagnosis located the failed node, but the agent still needs the node details before answering.", observeState(
			"deep_dive",
			readDataString(result.Data, "conclusion"),
			0.72,
			nextHintCalls,
			[]string{result.Name},
			"What is the concrete node-level error message?",
		))
	}
	if latestTaskID != "" && latestLogError != "" {
		nextHintCalls := buildHintCall("ingestion_task_query", map[string]any{
			"taskId":       latestTaskID,
			"includeNodes": true,
		})
		return newObserveResult(false, "The document diagnosis only has a task or chunk-log level error summary, so the next step is to inspect the task detail.", observeState(
			"deep_dive",
			readDataString(result.Data, "conclusion"),
			0.58,
			nextHintCalls,
			[]string{result.Name},
			"Which task node actually failed?",
			"Is there a node-level error message?",
		))
	}
	if latestTaskID != "" && (strings.Contains(conclusion, "still running") || strings.Contains(conclusion, "inconsistent")) {
		nextHintCalls := buildHintCall("ingestion_task_query", map[string]any{
			"taskId":       latestTaskID,
			"includeNodes": true,
		})
		return newObserveResult(false, "The document diagnosis suggests the task is still running or the states are inconsistent, so the next step is to inspect the task detail.", observeState(
			"verification",
			readDataString(result.Data, "conclusion"),
			0.45,
			nextHintCalls,
			[]string{result.Name},
			"Is the task still running or already blocked on a node?",
		))
	}
	if confidence == "high" {
		return newObserveResult(true, "The document diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", observeState(
			"complete",
			readDataString(result.Data, "conclusion"),
			0.9,
			nil,
			[]string{result.Name},
		))
	}
	return newObserveResult(true, "The document diagnosis already gathered the main evidence needed for the final answer.", observeState(
		"complete",
		readDataString(result.Data, "conclusion"),
		0.75,
		nil,
		[]string{result.Name},
	))
}

func observeTaskDiagnosis(result Result) ObserveResult {
	taskID := readDataString(result.Data, "taskId")
	latestNodeID := readDataString(result.Data, "latestNodeId")
	latestNodeError := readDataString(result.Data, "latestNodeError")
	conclusion := strings.ToLower(readDataString(result.Data, "conclusion"))
	confidence := strings.ToLower(readDataString(result.Data, "confidence"))

	if latestNodeError != "" {
		return newObserveResult(true, "The task diagnosis already includes the node error, so the agent can answer directly.", observeState(
			"complete",
			readDataString(result.Data, "conclusion"),
			1,
			nil,
			[]string{result.Name},
		))
	}
	if taskID != "" && latestNodeID != "" && strings.Contains(conclusion, "failed at node") {
		nextHintCalls := buildHintCall("ingestion_task_node_query", map[string]any{
			"taskId": taskID,
			"nodeId": latestNodeID,
		})
		return newObserveResult(false, "The task diagnosis located the failed node, but the agent still needs the node detail before answering.", observeState(
			"deep_dive",
			readDataString(result.Data, "conclusion"),
			0.72,
			nextHintCalls,
			[]string{result.Name},
			"What is the concrete node-level error message?",
		))
	}
	if taskID != "" && strings.Contains(conclusion, "still running") {
		nextHintCalls := buildHintCall("ingestion_task_query", map[string]any{
			"taskId":       taskID,
			"includeNodes": true,
		})
		return newObserveResult(false, "The task diagnosis shows the task is still running, so the next step is to inspect the live task detail.", observeState(
			"verification",
			readDataString(result.Data, "conclusion"),
			0.45,
			nextHintCalls,
			[]string{result.Name},
			"Which node is still running right now?",
		))
	}
	if confidence == "high" {
		return newObserveResult(true, "The task diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", observeState(
			"complete",
			readDataString(result.Data, "conclusion"),
			0.9,
			nil,
			[]string{result.Name},
		))
	}
	return newObserveResult(true, "The task diagnosis already gathered the main evidence needed for the final answer.", observeState(
		"complete",
		readDataString(result.Data, "conclusion"),
		0.75,
		nil,
		[]string{result.Name},
	))
}

func observeDocumentQuery(result Result) ObserveResult {
	documentID := readDataString(result.Data, "documentId")
	status := strings.ToLower(readDataString(result.Data, "status"))
	processMode := strings.ToLower(readDataString(result.Data, "processMode"))
	if documentID != "" && processMode == "pipeline" && (status == "failed" || status == "running") {
		nextHintCalls := buildHintCall("document_ingestion_diagnose", map[string]any{
			"documentId": documentID,
		})
		return newObserveResult(false, "The document state suggests a pipeline failure or an in-progress task, so continue with document-level diagnosis.", observeState(
			"initial_diagnosis",
			fmt.Sprintf("document %s is in pipeline status %s", documentID, status),
			0.35,
			nextHintCalls,
			[]string{result.Name},
			"Why did the pipeline document enter this state?",
		))
	}
	return newObserveResult(true, "The document query already returned a stable document state, so the agent loop can stop.", observeState(
		"complete",
		fmt.Sprintf("document state is %s", status),
		0.8,
		nil,
		[]string{result.Name},
	))
}

func observeChunkLogQuery(result Result) ObserveResult {
	documentID := readDataString(result.Data, "documentId")
	latestTaskID := readDataString(result.Data, "latestTaskId")
	failedLogCount := readDataInt(result.Data, "failedLogCount")
	latestStatus := strings.ToLower(readDataString(result.Data, "latestStatus"))
	if documentID != "" && (failedLogCount > 0 || latestStatus == "failed" || latestStatus == "running") {
		if latestTaskID != "" {
			nextHintCalls := buildHintCall("ingestion_task_query", map[string]any{
				"taskId":       latestTaskID,
				"includeNodes": true,
			})
			return newObserveResult(false, "The chunk log shows a failed or running ingestion record, so the next step is to inspect the task detail.", observeState(
				"deep_dive",
				fmt.Sprintf("task %s has abnormal chunk-log state %s", latestTaskID, latestStatus),
				0.52,
				nextHintCalls,
				[]string{result.Name},
				"Which ingestion node caused the abnormal chunk-log state?",
			))
		}
		nextHintCalls := buildHintCall("document_ingestion_diagnose", map[string]any{
			"documentId": documentID,
		})
		return newObserveResult(false, "The chunk log shows an abnormal ingestion record, but a task id is not available, so continue with document-level diagnosis.", observeState(
			"initial_diagnosis",
			fmt.Sprintf("document %s has abnormal chunk-log state %s", documentID, latestStatus),
			0.4,
			nextHintCalls,
			[]string{result.Name},
			"Which ingestion task is associated with this abnormal chunk-log state?",
		))
	}
	return newObserveResult(true, "The chunk log did not expose a new abnormal state that requires deeper drilling.", observeState(
		"complete",
		"chunk log is stable",
		0.75,
		nil,
		[]string{result.Name},
	))
}

func observeTaskQuery(result Result) ObserveResult {
	taskID := readDataString(result.Data, "taskId")
	status := strings.ToLower(readDataString(result.Data, "status"))
	if taskID == "" {
		return newObserveResult(true, "The task query result is missing taskId, so the agent loop stops here.", observeState(
			"complete",
			"task query result is missing taskId",
			0.2,
			nil,
			[]string{result.Name},
		))
	}
	if nodeID, nodeStatus, ok := latestInterestingTaskNode(result.Data); ok {
		nextHintCalls := buildHintCall("ingestion_task_node_query", map[string]any{
			"taskId": taskID,
			"nodeId": nodeID,
		})
		if nodeStatus == "running" {
			return newObserveResult(false, "The task query exposed a running node, so the next step is to inspect that live node instead of assuming a failure.", observeState(
				"verification",
				fmt.Sprintf("task %s is still running at node %s", taskID, nodeID),
				0.55,
				nextHintCalls,
				[]string{result.Name},
				"What is the live status of this running node?",
			))
		}
		return newObserveResult(false, "The task query already exposed a failed node, so the next step is to inspect that node directly.", observeState(
			"deep_dive",
			fmt.Sprintf("task %s failed at node %s", taskID, nodeID),
			0.7,
			nextHintCalls,
			[]string{result.Name},
			"What is the concrete error for this failed node?",
		))
	}
	if status == "failed" || status == "running" {
		nextHintCalls := buildHintCall("task_ingestion_diagnose", map[string]any{
			"taskId": taskID,
		})
		return newObserveResult(false, "The task is still failed or running, so continue with task-level diagnosis before answering.", observeState(
			"verification",
			fmt.Sprintf("task %s is currently %s", taskID, status),
			0.45,
			nextHintCalls,
			[]string{result.Name},
			"Which node or condition explains the current task status?",
		))
	}
	return newObserveResult(true, "The task query already returned a stable task state, so the agent loop can stop.", observeState(
		"complete",
		fmt.Sprintf("task %s is stable with status %s", taskID, status),
		0.8,
		nil,
		[]string{result.Name},
	))
}

func readDataInt(data map[string]any, key string) int {
	if len(data) == 0 {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func lastNonThinkResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(results[idx].Name) != "think" {
			return results[idx], true
		}
	}
	return Result{}, false
}
