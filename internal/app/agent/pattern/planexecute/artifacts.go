package planexecute

import (
	"strconv"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	artifactKindURLs             = "url_list"
	artifactKindSearchResults    = "search_results"
	artifactKindFetchResults     = "fetch_results"
	artifactKindEvidenceRefs     = "evidence_refs"
	artifactKindStructuredOutput = "structured_output"
)

func lastStepArtifacts(session *agentruntime.RuntimeSession) []agentstate.PlanStepArtifact {
	if session == nil {
		return nil
	}
	return append([]agentstate.PlanStepArtifact(nil), session.Snapshot.Plan.LastStepResult.Artifacts...)
}

func artifactsByKind(items []agentstate.PlanStepArtifact, kind string) []agentstate.PlanStepArtifact {
	target := strings.TrimSpace(kind)
	if target == "" || len(items) == 0 {
		return nil
	}
	filtered := make([]agentstate.PlanStepArtifact, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Kind) == target || strings.TrimSpace(item.Name) == target {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func artifactStringValues(items []agentstate.PlanStepArtifact, kind string) []string {
	filtered := artifactsByKind(items, kind)
	if len(filtered) == 0 {
		return nil
	}
	values := make([]string, 0, len(filtered)*2)
	seen := make(map[string]struct{}, len(filtered)*2)
	appendValue := func(raw string) {
		if value := strings.TrimSpace(raw); value != "" {
			if _, ok := seen[value]; ok {
				return
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	for _, item := range filtered {
		for _, value := range item.StringValues {
			appendValue(value)
		}
		for _, value := range item.Refs {
			appendValue(value)
		}
	}
	return values
}

func artifactURLs(items []agentstate.PlanStepArtifact) []string {
	urls := artifactStringValues(items, artifactKindURLs)
	if len(urls) > 0 {
		return urls
	}
	urls = artifactStringValues(items, artifactKindSearchResults)
	if len(urls) > 0 {
		return urls
	}
	return artifactStringValues(items, artifactKindFetchResults)
}

func evidenceItemsFromArtifacts(items []agentstate.PlanStepArtifact) []agentstate.EvidenceItem {
	filtered := artifactsByKind(items, artifactKindEvidenceRefs)
	if len(filtered) == 0 {
		return nil
	}
	evidenceItems := make([]agentstate.EvidenceItem, 0, len(filtered))
	seen := make(map[string]struct{}, len(filtered))
	for idx, item := range filtered {
		content := firstNonEmpty(strings.Join(item.StringValues, "\n\n"), item.Summary)
		sourceRef := firstNonEmpty(strings.Join(item.Refs, ","), item.SourceStepID)
		if strings.TrimSpace(content) == "" && strings.TrimSpace(sourceRef) == "" {
			continue
		}
		key := content + "|" + sourceRef
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		evidenceItems = append(evidenceItems, agentstate.EvidenceItem{
			ID:        firstNonEmpty(item.SourceStepID, "artifact_evidence") + "_" + strconv.Itoa(idx+1),
			Source:    "plan_artifact",
			Content:   content,
			Level:     "high",
			SourceRef: sourceRef,
		})
	}
	return evidenceItems
}

func hasStructuredArtifact(items []agentstate.PlanStepArtifact) bool {
	return len(artifactsByKind(items, artifactKindStructuredOutput)) > 0
}

func firstNonEmptyStructuredArtifactValue(items []agentstate.PlanStepArtifact) string {
	values := artifactStringValues(items, artifactKindStructuredOutput)
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	for _, artifact := range artifactsByKind(items, artifactKindStructuredOutput) {
		if trimmed := strings.TrimSpace(artifact.Summary); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstArtifactValue(items []agentstate.PlanStepArtifact, kind string) string {
	values := artifactStringValues(items, kind)
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	for _, artifact := range artifactsByKind(items, kind) {
		if trimmed := strings.TrimSpace(artifact.Summary); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
