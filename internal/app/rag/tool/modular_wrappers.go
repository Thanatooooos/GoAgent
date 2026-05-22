package tool

import (
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

func BuildAnswerGuidance(results []Result) string {
	return ragruntime.BuildAnswerGuidance(results)
}

func BuildAnswerGuidanceWithRegistry(registry *Registry, results []Result) string {
	return ragruntime.BuildAnswerGuidanceWithRegistry(registry, results)
}

func RenderContext(results []Result) string {
	return ragruntime.RenderContext(results)
}

func RenderContextWithRegistry(registry *Registry, results []Result) string {
	return ragruntime.RenderContextWithRegistry(registry, results)
}

func ToCallSummaries(results []Result) []CallSummary {
	return ragruntime.ToCallSummaries(results)
}

func SummarizeResultDataForLLM(data map[string]any) string {
	return ragcore.SummarizeResultDataForLLM(data)
}

func TruncateForLog(raw string) string {
	return ragruntime.TruncateForLog(raw)
}

func KnowledgeBaseInsufficient(retrieveResult ragretrieve.Result) bool {
	return ragcore.KnowledgeBaseInsufficient(retrieveResult)
}

func nextDecisionWithRegistry(registry *Registry, input WorkflowInput, result Result) NextDecision {
	return ragruntime.NextDecisionWithRegistry(registry, input, result)
}

func planCallsFromResultsWithRegistry(results []Result, input WorkflowInput, registry *Registry) []Call {
	return ragruntime.PlanCallsFromResultsWithRegistry(results, input, registry)
}

func observeWithRegistry(result Result, input ObserveInput) (ObserveResult, bool) {
	return ragruntime.ObserveWithRegistry(result, input)
}
