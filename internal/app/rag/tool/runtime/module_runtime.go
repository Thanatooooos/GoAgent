package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

func NextDecisionWithRegistry(registry *Registry, input WorkflowInput, result Result) NextDecision {
	if registry != nil {
		if behavior, ok := registry.GetBehavior(result.Name); ok && behavior.Next != nil {
			decision := behavior.Next(result, input)
			if len(decision.HintCalls) > 0 || decision.Done || decision.Terminal || decision.Reason != "" || decision.Retryable {
				return decision
			}
		}
	}
	return NextDecision{Done: true, Reason: "terminal", Terminal: true}
}

func PlanCallsFromResultsWithRegistry(results []Result, input WorkflowInput, registry *Registry) []Call {
	if len(results) == 0 {
		return nil
	}
	latest := results[len(results)-1]
	decision := NextDecisionWithRegistry(registry, input, latest)
	if decision.Done || decision.Terminal || len(decision.HintCalls) == 0 {
		return nil
	}
	calls := make([]Call, 0, len(decision.HintCalls))
	for _, hintCall := range NormalizeHintCalls(decision.HintCalls) {
		if strings.TrimSpace(hintCall.Name) == "" {
			continue
		}
		calls = append(calls, Call{
			Name:      strings.TrimSpace(hintCall.Name),
			Arguments: CloneMap(hintCall.Arguments),
		})
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func ObserveWithRegistry(result Result, input ObserveInput) (ObserveResult, bool) {
	if input.ToolRegistry == nil {
		return ObserveResult{}, false
	}
	behavior, ok := input.ToolRegistry.GetBehavior(result.Name)
	if !ok || behavior.Observe == nil {
		return ObserveResult{}, false
	}
	return behavior.Observe(result, input)
}

func RenderContextWithRegistry(registry *Registry, results []Result) string {
	if len(results) == 0 {
		return ""
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

		detail := ""
		if registry != nil {
			if behavior, ok := registry.GetBehavior(result.Name); ok && behavior.RenderContext != nil {
				detail = strings.TrimSpace(behavior.RenderContext(result))
			}
		}
		if detail == "" {
			detail = strings.TrimSpace(renderResultContextDetail(result))
		}
		if detail != "" {
			builder.WriteString("\n")
			builder.WriteString(detail)
		}
	}
	return strings.TrimSpace(builder.String())
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
	}
	return BuildAnswerGuidance(results)
}

func renderGuidanceNotes(notes []GuidanceNote) string {
	if len(notes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(notes))
	for _, note := range notes {
		text := strings.TrimSpace(note.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
