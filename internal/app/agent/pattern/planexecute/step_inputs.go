package planexecute

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func prepareStepInputs(session *agentruntime.RuntimeSession, step *agentstate.PlanStep) {
	if step == nil {
		return
	}
	switch step.CapabilityName {
	case agentcapability.NameWebSearch:
		query := resolveSearchStepQuery(session, *step)
		if strings.TrimSpace(query) == "" {
			return
		}
		step.Query = query
		ensureStepInput(step)["query"] = query
	case agentcapability.NameWebFetch:
		urls := resolveFetchStepURLs(session, *step)
		if len(urls) == 0 {
			return
		}
		step.URLs = append([]string(nil), urls...)
		ensureStepInput(step)["urls"] = append([]string(nil), urls...)
	case agentcapability.NameExternalEvidenceCollect:
		query := resolveExternalEvidenceQuery(session, *step)
		if strings.TrimSpace(query) == "" {
			return
		}
		step.Query = query
		ensureStepInput(step)["query"] = query
	}
}

func ensureStepInput(step *agentstate.PlanStep) map[string]any {
	if step.CapabilityInput == nil {
		step.CapabilityInput = make(map[string]any, 1)
	}
	return step.CapabilityInput
}

func resolveSearchStepQuery(session *agentruntime.RuntimeSession, step agentstate.PlanStep) string {
	base := firstNonEmpty(explicitStepQuery(step), normalizeQuery(session))
	context := artifactContextForStep(session, step)
	return composeQueryWithContext(base, context)
}

func resolveFetchStepURLs(session *agentruntime.RuntimeSession, step agentstate.PlanStep) []string {
	if len(step.URLs) > 0 {
		return append([]string(nil), step.URLs...)
	}
	if raw := toStringSlice(step.CapabilityInput["urls"]); len(raw) > 0 {
		return raw
	}
	return selectedFetchURLs(session)
}

func resolveExternalEvidenceQuery(session *agentruntime.RuntimeSession, step agentstate.PlanStep) string {
	base := firstNonEmpty(explicitStepQuery(step), normalizeQuery(session))
	context := artifactContextForStep(session, step)
	return composeQueryWithContext(base, context)
}

func explicitStepQuery(step agentstate.PlanStep) string {
	if trimmed := strings.TrimSpace(step.Query); trimmed != "" {
		return trimmed
	}
	if query, ok := step.CapabilityInput["query"].(string); ok {
		return strings.TrimSpace(query)
	}
	return ""
}

func artifactContextForStep(session *agentruntime.RuntimeSession, step agentstate.PlanStep) string {
	if session == nil || len(step.Consumes) == 0 {
		return ""
	}
	artifacts := lastStepArtifacts(session)
	if len(artifacts) == 0 {
		return ""
	}
	return firstRelevantArtifactContext(artifacts, step.Consumes)
}

func firstRelevantArtifactContext(items []agentstate.PlanStepArtifact, consumes []string) string {
	for _, consume := range consumes {
		switch strings.TrimSpace(consume) {
		case artifactKindStructuredOutput:
			if value := firstNonEmptyStructuredArtifactValue(items); value != "" {
				return value
			}
		case artifactKindEvidenceRefs:
			if value := firstArtifactValue(items, artifactKindEvidenceRefs); value != "" {
				return value
			}
		case artifactKindFetchResults:
			if value := firstArtifactValue(items, artifactKindFetchResults); value != "" {
				return value
			}
		case artifactKindSearchResults, artifactKindURLs:
			if urls := artifactURLs(items); len(urls) > 0 {
				return urls[0]
			}
		}
	}
	return ""
}

func composeQueryWithContext(base, context string) string {
	base = strings.TrimSpace(base)
	context = strings.TrimSpace(context)
	switch {
	case base == "":
		return context
	case context == "":
		return base
	case strings.Contains(base, context):
		return base
	default:
		return base + "\ncontext: " + context
	}
}
