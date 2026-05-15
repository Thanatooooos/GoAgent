package tool

import (
	"regexp"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

// This file provides thin forwarding wrappers for symbols whose canonical
// implementations live in the core or runtime packages. These wrappers exist
// so that package-internal tests can invoke them without importing those
// packages directly.

// Pattern variables for ID extraction (forwarded from core).
var (
	documentIDPattern = ragcore.DocumentIDPattern
	taskIDPattern     = ragcore.TaskIDPattern
	traceIDPattern    = ragcore.TraceIDPattern
)

func firstMatchedID(pattern *regexp.Regexp, text string) string {
	return ragcore.FirstMatchedID(pattern, text)
}

func containsAny(text string, keywords ...string) bool {
	return ragcore.ContainsAny(text, keywords...)
}

func callKey(call Call) string {
	return ragcore.CallKey(call)
}

const defaultMaxIterations = ragruntime.DefaultMaxIterations

func planCallsFromHint(agentState string, defs []Definition) []Call {
	return ragruntime.PlanCallsFromHint(agentState, defs)
}

func planCallsFromHintCalls(hintCalls []HintCall, defs []Definition) []Call {
	return ragruntime.PlanCallsFromHintCalls(hintCalls, defs)
}

func planCallsFromResults(results []Result) []Call {
	return ragruntime.PlanCallsFromResults(results)
}

func planWithBaseRules(input WorkflowInput, maxCalls int) []Call {
	return ragruntime.PlanWithBaseRules(input, maxCalls)
}

func observeDocumentDiagnosis(result Result) ObserveResult {
	return ragruntime.ObserveDocumentDiagnosis(result)
}

func observeTaskDiagnosis(result Result) ObserveResult {
	return ragruntime.ObserveTaskDiagnosis(result)
}

func observeTaskQuery(result Result) ObserveResult {
	return ragruntime.ObserveTaskQuery(result)
}

func observeWebFetch(result Result) ObserveResult {
	return ragruntime.ObserveWebFetch(result)
}
