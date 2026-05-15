package tool

import (
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

// Public utility functions.

func BuildAnswerGuidance(results []Result) string {
	return ragruntime.BuildAnswerGuidance(results)
}

func BuildAnswerGuidanceWithRegistry(registry *Registry, results []Result) string {
	if registry != nil {
		for idx := len(results) - 1; idx >= 0; idx-- {
			behavior, ok := registry.GetBehavior(results[idx].Name)
			if !ok || behavior.BuildGuidance == nil {
				continue
			}
			notes := behavior.BuildGuidance(results[idx], GuidanceInput{AllResults: append([]Result(nil), results...)})
			if text := renderGuidanceNotes(notes); text != "" {
				return text
			}
		}
		return ""
	}
	return BuildAnswerGuidance(results)
}

func RenderContext(results []Result) string {
	return ragruntime.RenderContext(results)
}

func RenderContextWithRegistry(registry *Registry, results []Result) string {
	if registry == nil {
		return RenderContext(results)
	}
	var builder strings.Builder
	for _, result := range results {
		name := strings.TrimSpace(result.Name)
		if name == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("### ")
		builder.WriteString(name)
		builder.WriteString("\n")
		if summary := strings.TrimSpace(result.Summary); summary != "" {
			builder.WriteString(summary)
		} else if result.Successful() {
			builder.WriteString("tool executed successfully")
		} else {
			builder.WriteString(strings.TrimSpace(result.ErrorMessage))
		}
		if behavior, ok := registry.GetBehavior(result.Name); ok && behavior.RenderContext != nil {
			if detail := strings.TrimSpace(behavior.RenderContext(result)); detail != "" {
				builder.WriteString("\n")
				builder.WriteString(detail)
			}
		}
	}
	return strings.TrimSpace(builder.String())
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

// nextAction is the legacy fallback when no registry behavior has a Next function.

func nextAction(result Result) (hintCall *HintCall, done bool, reason string) {
	behavior := inferLegacyBehavior(result.Name)
	if behavior.Next == nil {
		return nil, true, "terminal"
	}
	return nextActionFromDecision(behavior.Next(result, WorkflowInput{}))
}

func nextActionFromDecision(decision NextDecision) (hintCall *HintCall, done bool, reason string) {
	hints := ragcore.NormalizeHintCalls(decision.HintCalls)
	if len(hints) > 0 {
		return &hints[0], false, strings.TrimSpace(decision.Reason)
	}
	if decision.Done || decision.Terminal || strings.TrimSpace(decision.Reason) != "" {
		return nil, decision.Done || decision.Terminal, strings.TrimSpace(decision.Reason)
	}
	return nil, true, "terminal"
}

// Registry-aware decision helpers.

func nextDecisionWithRegistry(registry *Registry, input WorkflowInput, result Result) NextDecision {
	if registry != nil {
		if behavior, ok := registry.GetBehavior(result.Name); ok && behavior.Next != nil {
			decision := behavior.Next(result, input)
			if len(decision.HintCalls) > 0 || decision.Done || decision.Terminal || decision.Reason != "" || decision.Retryable {
				return decision
			}
		}
	}
	hintCall, done, reason := nextAction(result)
	decision := NextDecision{
		Done:     done,
		Reason:   reason,
		Terminal: done,
	}
	if hintCall != nil {
		decision.HintCalls = []HintCall{*hintCall}
		decision.Done = false
		decision.Terminal = false
	}
	return decision
}

func planCallsFromResultsWithRegistry(results []Result, input WorkflowInput, registry *Registry) []Call {
	if len(results) == 0 {
		return nil
	}
	latest := results[len(results)-1]
	decision := nextDecisionWithRegistry(registry, input, latest)
	if decision.Done || decision.Terminal || len(decision.HintCalls) == 0 {
		return nil
	}
	calls := make([]Call, 0, len(decision.HintCalls))
	for _, hintCall := range ragcore.NormalizeHintCalls(decision.HintCalls) {
		if strings.TrimSpace(hintCall.Name) == "" {
			continue
		}
		calls = append(calls, Call{
			Name:      strings.TrimSpace(hintCall.Name),
			Arguments: ragcore.CloneMap(hintCall.Arguments),
		})
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func observeWithRegistry(result Result, input ObserveInput) (ObserveResult, bool) {
	if input.ToolRegistry == nil {
		return ObserveResult{}, false
	}
	behavior, ok := input.ToolRegistry.GetBehavior(result.Name)
	if !ok || behavior.Observe == nil {
		return ObserveResult{}, false
	}
	return behavior.Observe(result, input)
}

// Internal helpers.

func runtimeGuidanceNotes(results []Result) []GuidanceNote {
	text := strings.TrimSpace(ragruntime.BuildAnswerGuidance(results))
	if text == "" {
		return nil
	}
	return []GuidanceNote{{Text: text}}
}
