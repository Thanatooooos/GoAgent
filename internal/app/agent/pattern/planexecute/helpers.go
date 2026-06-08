package planexecute

import (
	"fmt"
	"strconv"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func normalizeQuery(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	candidates := []string{
		session.Snapshot.Context.SearchQuery,
		session.Snapshot.Context.RewrittenQuery,
		session.Request.Question,
		session.Snapshot.Request.Question,
	}
	for _, candidate := range candidates {
		if trimmed := strings.Join(strings.Fields(strings.TrimSpace(candidate)), " "); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func copyPlan(plan agentstate.PlanState) agentstate.PlanState {
	return agentstate.ClonePlanState(plan)
}

func findFirstPendingStep(plan agentstate.PlanState) int {
	for i, step := range plan.Steps {
		if step.Status != agentstate.PlanStepStatusPending {
			continue
		}
		if dependenciesSatisfied(plan, step) {
			return i
		}
	}
	return -1
}

func dependenciesSatisfied(plan agentstate.PlanState, step agentstate.PlanStep) bool {
	if len(step.DependsOn) == 0 {
		return true
	}
	completed := make(map[string]struct{}, len(plan.Steps))
	for _, item := range plan.Steps {
		if item.Status == agentstate.PlanStepStatusCompleted {
			completed[item.StepID] = struct{}{}
		}
	}
	for _, dep := range step.DependsOn {
		if _, ok := completed[dep]; !ok {
			return false
		}
	}
	return true
}

func selectedFetchURLs(session *agentruntime.RuntimeSession) []string {
	if session == nil {
		return nil
	}
	artifactURLs := artifactURLs(lastStepArtifacts(session))
	if len(artifactURLs) > 0 {
		return selectPreferredURLs(artifactURLs, session.Snapshot.Context.SeenURLs, session.Snapshot.Context.AvoidURLs, session.Snapshot.Context.PreferredURLs)
	}
	results := session.Snapshot.Context.SearchResults
	if len(results) == 0 {
		return nil
	}
	seen := toStringSet(session.Snapshot.Context.SeenURLs)
	avoid := toStringSet(session.Snapshot.Context.AvoidURLs)
	preferred := toStringSet(session.Snapshot.Context.PreferredURLs)

	selectURL := func(prefer bool) string {
		for _, result := range results {
			url := strings.TrimSpace(result.URL)
			if url == "" {
				continue
			}
			if _, skipped := avoid[url]; skipped {
				continue
			}
			if _, alreadySeen := seen[url]; alreadySeen {
				continue
			}
			if prefer {
				if _, wanted := preferred[url]; !wanted {
					continue
				}
			}
			return url
		}
		return ""
	}

	if url := selectURL(true); url != "" {
		return []string{url}
	}
	if url := selectURL(false); url != "" {
		return []string{url}
	}
	return nil
}

func selectPreferredURLs(candidates, seenURLs, avoidURLs, preferredURLs []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	seen := toStringSet(seenURLs)
	avoid := toStringSet(avoidURLs)
	preferred := toStringSet(preferredURLs)

	selectURL := func(prefer bool) string {
		for _, candidate := range candidates {
			url := strings.TrimSpace(candidate)
			if url == "" {
				continue
			}
			if _, skipped := avoid[url]; skipped {
				continue
			}
			if _, alreadySeen := seen[url]; alreadySeen {
				continue
			}
			if prefer {
				if _, wanted := preferred[url]; !wanted {
					continue
				}
			}
			return url
		}
		return ""
	}

	if url := selectURL(true); url != "" {
		return []string{url}
	}
	if url := selectURL(false); url != "" {
		return []string{url}
	}
	return nil
}

func cloneInputMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func selectionFromStep(step agentstate.PlanStep) selectcapability.CapabilitySelection {
	return selectcapability.CapabilitySelection{
		Name:       step.CapabilityName,
		Kind:       step.CapabilityKind,
		Family:     step.CapabilityFamily,
		Role:       step.CapabilityRole,
		Input:      selectionInputFromStep(step),
		Reason:     step.Title,
		Confidence: "",
	}
}

func selectionInputFromStep(step agentstate.PlanStep) map[string]any {
	input := cloneInputMap(step.CapabilityInput)
	switch strings.TrimSpace(step.CapabilityName) {
	case agentcapability.NameWebSearch:
		if strings.TrimSpace(step.Query) != "" {
			if input == nil {
				input = make(map[string]any, 1)
			}
			if _, ok := input["query"]; !ok {
				input["query"] = step.Query
			}
		}
	case agentcapability.NameWebFetch:
		if len(step.URLs) > 0 {
			if input == nil {
				input = make(map[string]any, 1)
			}
			if _, ok := input["urls"]; !ok {
				input["urls"] = append([]string(nil), step.URLs...)
			}
		}
	}
	if len(input) == 0 {
		return nil
	}
	return input
}

func toStringSlice(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	default:
		return nil
	}
}

func newEvidenceFromFetch(session *agentruntime.RuntimeSession) []agentstate.EvidenceItem {
	if session == nil {
		return nil
	}
	if items := evidenceItemsFromArtifacts(lastStepArtifacts(session)); len(items) > 0 {
		existing := make(map[string]struct{}, len(session.Snapshot.Evidence.Items))
		for _, item := range session.Snapshot.Evidence.Items {
			existing[evidenceKey(item)] = struct{}{}
		}
		filtered := make([]agentstate.EvidenceItem, 0, len(items))
		for _, item := range items {
			key := evidenceKey(item)
			if _, ok := existing[key]; ok {
				continue
			}
			existing[key] = struct{}{}
			filtered = append(filtered, item)
		}
		if len(filtered) > 0 {
			return filtered
		}
	}
	existing := make(map[string]struct{}, len(session.Snapshot.Evidence.Items))
	for _, item := range session.Snapshot.Evidence.Items {
		existing[evidenceKey(item)] = struct{}{}
	}
	items := make([]agentstate.EvidenceItem, 0, len(session.Snapshot.Context.FetchResults))
	for _, result := range session.Snapshot.Context.FetchResults {
		if strings.TrimSpace(result.Text) == "" || result.Degraded {
			continue
		}
		item := agentstate.EvidenceItem{
			ID:        result.ID,
			Source:    "fetch",
			Content:   firstNonEmpty(result.Summary, result.Text),
			Level:     "high",
			SourceRef: result.URL,
		}
		key := evidenceKey(item)
		if _, ok := existing[key]; ok {
			continue
		}
		existing[key] = struct{}{}
		items = append(items, item)
	}
	return items
}

func evidenceKey(item agentstate.EvidenceItem) string {
	return item.ID + "|" + item.SourceRef + "|" + item.Content
}

func outputMode(session *agentruntime.RuntimeSession, configured string) string {
	if session != nil {
		if mode := strings.TrimSpace(session.Snapshot.Request.RuntimeOptions.OutputMode); mode != "" {
			return mode
		}
		if mode := strings.TrimSpace(session.Request.Options.OutputMode); mode != "" {
			return mode
		}
	}
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return agentstate.OutputModeFinalAnswer
}

func buildFinalAnswer(session *agentruntime.RuntimeSession) string {
	if session == nil || len(session.Snapshot.Evidence.Items) == 0 {
		return "I couldn't gather readable evidence for a grounded answer."
	}
	return "Based on plan-execute evidence: " + session.Snapshot.Evidence.Items[0].Content
}

func toStringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resultSummary(step agentstate.PlanStep, status, errorClass string, output any, observation string) agentstate.PlanStepResult {
	summary := firstNonEmpty(observation, step.Title)
	result := agentstate.PlanStepResult{
		StepID:         step.StepID,
		CapabilityName: step.CapabilityName,
		Status:         status,
		ErrorClass:     errorClass,
		Summary:        summary,
		Observation:    summary,
		URLs:           append([]string(nil), step.URLs...),
	}
	switch value := output.(type) {
	case agentsearch.SearchOutput:
		result.URLs = append([]string(nil), value.URLs...)
		result.Artifacts = append(result.Artifacts, agentstate.PlanStepArtifact{
			Name:         artifactKindSearchResults,
			Kind:         artifactKindSearchResults,
			SourceStepID: step.StepID,
			Summary:      firstNonEmpty(value.Summary, summary),
			StringValues: append([]string(nil), value.URLs...),
			Refs:         append([]string(nil), value.URLs...),
			Metadata: map[string]string{
				"query":        strings.TrimSpace(value.Query),
				"result_count": strconv.Itoa(len(value.Results)),
			},
		})
	case *agentsearch.SearchOutput:
		if value != nil {
			result.URLs = append([]string(nil), value.URLs...)
			result.Artifacts = append(result.Artifacts, agentstate.PlanStepArtifact{
				Name:         artifactKindSearchResults,
				Kind:         artifactKindSearchResults,
				SourceStepID: step.StepID,
				Summary:      firstNonEmpty(value.Summary, summary),
				StringValues: append([]string(nil), value.URLs...),
				Refs:         append([]string(nil), value.URLs...),
				Metadata: map[string]string{
					"query":        strings.TrimSpace(value.Query),
					"result_count": strconv.Itoa(len(value.Results)),
				},
			})
		}
	case agentfetch.Output:
		result.Artifacts = append(result.Artifacts, buildFetchArtifacts(step.StepID, summary, value)...)
	case *agentfetch.Output:
		if value != nil {
			result.Artifacts = append(result.Artifacts, buildFetchArtifacts(step.StepID, summary, *value)...)
		}
	}
	if output != nil && !hasStructuredArtifact(result.Artifacts) {
		result.Artifacts = append(result.Artifacts, agentstate.PlanStepArtifact{
			Name:         artifactKindStructuredOutput,
			Kind:         artifactKindStructuredOutput,
			SourceStepID: step.StepID,
			Summary:      summary,
			StringValues: []string{summary},
			Metadata: map[string]string{
				"capability_name": step.CapabilityName,
				"status":          status,
			},
		})
	}
	if len(result.URLs) > 0 {
		result.Artifacts = append(result.Artifacts, agentstate.PlanStepArtifact{
			Name:         "urls",
			Kind:         artifactKindURLs,
			SourceStepID: step.StepID,
			Summary:      "candidate URLs produced by plan step",
			StringValues: append([]string(nil), result.URLs...),
			Refs:         append([]string(nil), result.URLs...),
		})
	}
	return result
}

func buildFetchArtifacts(stepID, fallbackSummary string, output agentfetch.Output) []agentstate.PlanStepArtifact {
	artifacts := []agentstate.PlanStepArtifact{
		{
			Name:         artifactKindFetchResults,
			Kind:         artifactKindFetchResults,
			SourceStepID: stepID,
			Summary:      firstNonEmpty(output.Summary, fallbackSummary),
			StringValues: append([]string(nil), output.URLs...),
			Refs:         append([]string(nil), output.URLs...),
			Metadata: map[string]string{
				"page_count": strconv.Itoa(len(output.Pages)),
			},
		},
	}
	for _, page := range output.Pages {
		if strings.TrimSpace(page.Text) == "" || strings.TrimSpace(page.ErrorMessage) != "" {
			continue
		}
		artifacts = append(artifacts, agentstate.PlanStepArtifact{
			Name:         artifactKindEvidenceRefs,
			Kind:         artifactKindEvidenceRefs,
			SourceStepID: stepID,
			Summary:      firstNonEmpty(page.URL, fallbackSummary),
			StringValues: []string{firstNonEmpty(page.Text)},
			Refs:         []string{page.URL},
		})
	}
	return artifacts
}

func requiresRuntimeApproval(session *agentruntime.RuntimeSession, step agentstate.PlanStep) bool {
	if step.RequiresApproval {
		return true
	}
	if session == nil {
		return false
	}
	return session.Snapshot.Request.RuntimeOptions.RequireApproval || session.Request.Options.RequireApproval
}

func stepHasEvidence(spec agentcapability.Spec, step agentstate.PlanStep, result agentcapability.InvocationResult) bool {
	if result.Status != agentcapability.StatusSucceeded {
		return false
	}
	if len(result.EvidenceRefs) > 0 {
		return true
	}
	if result.Delta.Evidence != nil && len(result.Delta.Evidence.AddItems) > 0 {
		return true
	}
	if !spec.ProducesEvidence {
		return false
	}
	switch strings.TrimSpace(step.CapabilityName) {
	case agentcapability.NameWebFetch:
		return len(newEvidenceItemsFromFetchOutput(result.Output)) > 0
	default:
		return spec.ProducesEvidence
	}
}

func newEvidenceItemsFromFetchOutput(output any) []agentstate.EvidenceItem {
	switch value := output.(type) {
	case agentfetch.Output:
		return newEvidenceItemsFromFetchPages(value.Pages)
	case *agentfetch.Output:
		if value == nil {
			return nil
		}
		return newEvidenceItemsFromFetchPages(value.Pages)
	default:
		return nil
	}
}

func newEvidenceItemsFromFetchPages(pages []agentfetch.PageResult) []agentstate.EvidenceItem {
	if len(pages) == 0 {
		return nil
	}
	items := make([]agentstate.EvidenceItem, 0, len(pages))
	for idx, page := range pages {
		if strings.TrimSpace(page.Text) == "" || strings.TrimSpace(page.ErrorMessage) != "" {
			continue
		}
		items = append(items, agentstate.EvidenceItem{
			ID:        fmt.Sprintf("fetch_%d", idx+1),
			Source:    "fetch",
			Content:   firstNonEmpty(page.Text),
			Level:     "high",
			SourceRef: page.URL,
		})
	}
	return items
}
