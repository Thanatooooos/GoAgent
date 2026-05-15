package runtime

import (
	"context"
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
	"local/rag-project/internal/framework/log"
)

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
	if observation, handled := observeWithRegistry(latest, input); handled {
		return observation, nil
	}
	depth := latest.GetString("diagnosisDepth")
	switch depth {
	case "node_level":
		return newObserveResult(true, "The diagnosis chain reached node-level evidence. The agent can answer with high confidence.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.95,
			nil,
			[]string{latest.Name},
		)), nil
	case "task_level":
		return newObserveResult(true, "The diagnosis chain reached task-level evidence but not a specific node. The agent can answer with moderate confidence.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.75,
			nil,
			[]string{latest.Name},
		)), nil
	default:
		log.Infof("[observer] no module behavior for %q, falling back to generic completion", strings.TrimSpace(latest.Name))
		return newObserveResult(true, "The current tool result is already sufficient as supporting context, so the agent loop stops here.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.6,
			nil,
			[]string{latest.Name},
		)), nil
	}
}

func ObserveDocumentDiagnosis(result Result) ObserveResult {
	hintCall, _, reason := nextAction(result)
	conclusion := result.GetString("conclusion")
	confidence := strings.ToLower(result.GetString("confidence"))

	switch reason {
	case "node_error_found":
		return newObserveResult(true, "The document diagnosis already includes a node-level error, so the agent can answer directly.", observeState(
			"complete", conclusion, 1, nil, []string{result.Name},
		))
	case "failed_node_located":
		return newObserveResult(false, "The document diagnosis located the failed node, but the agent still needs the node details before answering.", observeState(
			"deep_dive", conclusion, 0.72, hintCallToSlice(hintCall), []string{result.Name},
			"What is the concrete node-level error message?",
		))
	case "task_level_error_only":
		return newObserveResult(false, "The document diagnosis only has a task or chunk-log level error summary, so the next step is to inspect the task detail.", observeState(
			"deep_dive", conclusion, 0.58, hintCallToSlice(hintCall), []string{result.Name},
			"Which task node actually failed?",
			"Is there a node-level error message?",
		))
	case "still_running_or_inconsistent":
		return newObserveResult(false, "The document diagnosis suggests the task is still running or the states are inconsistent, so the next step is to inspect the task detail.", observeState(
			"verification", conclusion, 0.45, hintCallToSlice(hintCall), []string{result.Name},
			"Is the task still running or already blocked on a node?",
		))
	default:
		if confidence == "high" {
			return newObserveResult(true, "The document diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", observeState(
				"complete", conclusion, 0.9, nil, []string{result.Name},
			))
		}
		return newObserveResult(true, "The document diagnosis already gathered the main evidence needed for the final answer.", observeState(
			"complete", conclusion, 0.75, nil, []string{result.Name},
		))
	}
}

func ObserveTaskDiagnosis(result Result) ObserveResult {
	hintCall, _, reason := nextAction(result)
	conclusion := result.GetString("conclusion")
	confidence := strings.ToLower(result.GetString("confidence"))

	switch reason {
	case "node_error_found":
		return newObserveResult(true, "The task diagnosis already includes the node error, so the agent can answer directly.", observeState(
			"complete", conclusion, 1, nil, []string{result.Name},
		))
	case "failed_node_located":
		return newObserveResult(false, "The task diagnosis located the failed node, but the agent still needs the node detail before answering.", observeState(
			"deep_dive", conclusion, 0.72, hintCallToSlice(hintCall), []string{result.Name},
			"What is the concrete node-level error message?",
		))
	case "still_running":
		return newObserveResult(false, "The task diagnosis shows the task is still running, so the next step is to inspect the live task detail.", observeState(
			"verification", conclusion, 0.45, hintCallToSlice(hintCall), []string{result.Name},
			"Which node is still running right now?",
		))
	default:
		if confidence == "high" {
			return newObserveResult(true, "The task diagnosis already reached a high-confidence conclusion, so the agent can answer directly.", observeState(
				"complete", conclusion, 0.9, nil, []string{result.Name},
			))
		}
		return newObserveResult(true, "The task diagnosis already gathered the main evidence needed for the final answer.", observeState(
			"complete", conclusion, 0.75, nil, []string{result.Name},
		))
	}
}

func observeDocumentQuery(result Result) ObserveResult {
	hintCall, _, reason := nextAction(result)
	documentID := result.GetString("documentId")
	status := strings.ToLower(result.GetString("status"))

	switch reason {
	case "pipeline_document_abnormal":
		return newObserveResult(false, "The document state suggests a pipeline failure or an in-progress task, so continue with document-level diagnosis.", observeState(
			"initial_diagnosis",
			fmt.Sprintf("document %s is in pipeline status %s", documentID, status),
			0.35, hintCallToSlice(hintCall), []string{result.Name},
			"Why did the pipeline document enter this state?",
		))
	default:
		return newObserveResult(true, "The document query already returned a stable document state, so the agent loop can stop.", observeState(
			"complete", fmt.Sprintf("document state is %s", status), 0.8, nil, []string{result.Name},
		))
	}
}

func observeChunkLogQuery(result Result) ObserveResult {
	hintCall, _, reason := nextAction(result)
	documentID := result.GetString("documentId")
	latestTaskID := result.GetString("latestTaskId")
	latestStatus := strings.ToLower(result.GetString("latestStatus"))

	switch reason {
	case "chunk_log_abnormal_with_task":
		return newObserveResult(false, "The chunk log shows a failed or running ingestion record, so the next step is to inspect the task detail.", observeState(
			"deep_dive",
			fmt.Sprintf("task %s has abnormal chunk-log state %s", latestTaskID, latestStatus),
			0.52, hintCallToSlice(hintCall), []string{result.Name},
			"Which ingestion node caused the abnormal chunk-log state?",
		))
	case "chunk_log_abnormal_no_task":
		return newObserveResult(false, "The chunk log shows an abnormal ingestion record, but a task id is not available, so continue with document-level diagnosis.", observeState(
			"initial_diagnosis",
			fmt.Sprintf("document %s has abnormal chunk-log state %s", documentID, latestStatus),
			0.4, hintCallToSlice(hintCall), []string{result.Name},
			"Which ingestion task is associated with this abnormal chunk-log state?",
		))
	default:
		return newObserveResult(true, "The chunk log did not expose a new abnormal state that requires deeper drilling.", observeState(
			"complete", "chunk log is stable", 0.75, nil, []string{result.Name},
		))
	}
}

func ObserveTaskQuery(result Result) ObserveResult {
	hintCall, _, reason := nextAction(result)
	taskID := result.GetString("taskId")
	status := strings.ToLower(result.GetString("status"))

	switch reason {
	case "task_id_missing":
		return newObserveResult(true, "The task query result is missing taskId, so the agent loop stops here.", observeState(
			"complete", "task query result is missing taskId", 0.2, nil, []string{result.Name},
		))
	case "interesting_node_found":
		nodeID, nodeStatus, _ := latestInterestingTaskNode(result.Data)
		if nodeStatus == "running" {
			return newObserveResult(false, "The task query exposed a running node, so the next step is to inspect that live node instead of assuming a failure.", observeState(
				"verification",
				fmt.Sprintf("task %s is still running at node %s", taskID, nodeID),
				0.55, hintCallToSlice(hintCall), []string{result.Name},
				"What is the live status of this running node?",
			))
		}
		return newObserveResult(false, "The task query already exposed a failed node, so the next step is to inspect that node directly.", observeState(
			"deep_dive",
			fmt.Sprintf("task %s failed at node %s", taskID, nodeID),
			0.7, hintCallToSlice(hintCall), []string{result.Name},
			"What is the concrete error for this failed node?",
		))
	case "task_abnormal":
		return newObserveResult(false, "The task is still failed or running, so continue with task-level diagnosis before answering.", observeState(
			"verification",
			fmt.Sprintf("task %s is currently %s", taskID, status),
			0.45, hintCallToSlice(hintCall), []string{result.Name},
			"Which node or condition explains the current task status?",
		))
	default:
		return newObserveResult(true, "The task query already returned a stable task state, so the agent loop can stop.", observeState(
			"complete", fmt.Sprintf("task %s is stable with status %s", taskID, status), 0.8, nil, []string{result.Name},
		))
	}
}

func observeWebSearch(result Result, input ObserveInput) ObserveResult {
	hintCall, done, reason := nextActionWebSearch(result)
	view, _ := webmod.ViewWebSearchResult(result)
	if done || hintCall == nil {
		return newObserveResult(true, "Web search completed. The results are sufficient to answer with source attribution.", observeState(
			"complete",
			result.Summary,
			0.75,
			nil,
			[]string{result.Name},
		))
	}
	switch reason {
	case "web_search_has_results":
		resultCount := result.GetInt("resultCount")
		if view.ResultCount > resultCount {
			resultCount = view.ResultCount
		}
		return newObserveResult(false, "Web search found results. Fetch content from the top results for more detail before answering.", observeState(
			"fetching",
			fmt.Sprintf("found %d web results", resultCount),
			0.45,
			[]HintCall{*hintCall},
			[]string{result.Name},
			"What does each result page actually say?",
		))
	default:
		return newObserveResult(true, "Web search completed. The results are sufficient to answer with source attribution.", observeState(
			"complete",
			result.Summary,
			0.75,
			nil,
			[]string{result.Name},
		))
	}
}

func ObserveWebFetch(result Result) ObserveResult {
	view, _ := webmod.ViewWebFetchResult(result)
	text := strings.TrimSpace(view.ReadableText())
	wasTruncated := view.AnyPageTruncated()
	if text == "" {
		return newObserveResult(true, "Web page fetched but no readable text was extracted. Answer with the available search snippets.", observeState(
			"complete",
			result.Summary,
			0.55,
			nil,
			[]string{result.Name},
		))
	}
	confidence := 0.8
	if wasTruncated {
		confidence = 0.7
	}
	return newObserveResult(true, "Web page content has been fetched. The agent can now synthesize an answer using both knowledge base context and web sources with proper attribution.", observeState(
		"complete",
		result.Summary,
		confidence,
		nil,
		[]string{result.Name},
	))
}

func webFetchWasTruncated(result Result) bool {
	view, ok := webmod.ViewWebFetchResult(result)
	if !ok {
		return false
	}
	return view.AnyPageTruncated()
}

func lastNonThinkResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(results[idx].Name) != "think" {
			return results[idx], true
		}
	}
	return Result{}, false
}
