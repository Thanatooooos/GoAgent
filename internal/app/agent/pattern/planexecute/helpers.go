package planexecute

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
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

func buildPlan(session *agentruntime.RuntimeSession) agentstate.PlanState {
	query := normalizeQuery(session)
	replanCount := 0
	if session != nil {
		replanCount = session.Snapshot.Plan.ReplanCount
	}
	return agentstate.PlanState{
		Goal:   firstNonEmpty(query, "collect external evidence"),
		PlanID: fmt.Sprintf("plan_%d", replanCount+1),
		Status: agentstate.PlanStatusActive,
		Steps: []agentstate.PlanStep{
			{
				StepID:           "step_search",
				Title:            "Search for relevant web sources",
				CapabilityName:   agentcapability.NameWebSearch,
				CapabilityKind:   agentcapability.KindTool,
				CapabilityFamily: agentcapability.FamilyExternalEvidence,
				CapabilityRole:   agentcapability.RoleSearch,
				CapabilityInput: map[string]any{
					"query": query,
				},
				Query:            query,
				Status:           agentstate.PlanStepStatusPending,
				ExpectedEvidence: []string{"fetchable search results"},
			},
			{
				StepID:           "step_fetch",
				Title:            "Fetch the best available source",
				CapabilityName:   agentcapability.NameWebFetch,
				CapabilityKind:   agentcapability.KindTool,
				CapabilityFamily: agentcapability.FamilyExternalEvidence,
				CapabilityRole:   agentcapability.RoleFetch,
				CapabilityInput:  map[string]any{},
				DependsOn:        []string{"step_search"},
				Status:           agentstate.PlanStepStatusPending,
				ExpectedEvidence: []string{"readable fetched evidence"},
			},
		},
		CurrentStepIndex:   -1,
		ReplanCount:        replanCount,
		CompletionCriteria: []string{"at least one readable fetched evidence item"},
		Confidence:         "medium",
		LastPlanReason:     reasonPlanBuilt,
	}
}

func buildPlanFromSpecs(session *agentruntime.RuntimeSession, searchSpec, fetchSpec agentcapability.Spec) agentstate.PlanState {
	plan := buildPlan(session)
	if len(plan.Steps) > 0 {
		plan.Steps[0].RequiresApproval = searchSpec.RequiresApproval
	}
	if len(plan.Steps) > 1 {
		plan.Steps[1].RequiresApproval = fetchSpec.RequiresApproval
	}
	return plan
}

func buildPlanFromSelection(session *agentruntime.RuntimeSession, matched agentresolve.MatchedCapability, selection selectcapability.CapabilitySelection) agentstate.PlanState {
	replanCount := 0
	if session != nil {
		replanCount = session.Snapshot.Plan.ReplanCount
	}
	role := strings.TrimSpace(selection.Role)
	if role == "" && len(matched.Spec.Roles) > 0 {
		role = matched.Spec.Roles[0]
	}
	expectedEvidence := []string{"structured capability result"}
	if matched.Spec.ProducesEvidence {
		expectedEvidence = []string{"grounded evidence produced by selected capability"}
	}
	input := cloneInputMap(selection.Input)
	step := agentstate.PlanStep{
		StepID:           "step_selected_capability",
		Title:            buildSelectionStepTitle(matched, selection),
		CapabilityName:   matched.Name,
		CapabilityKind:   matched.Spec.Kind,
		CapabilityFamily: matched.Spec.Family,
		CapabilityRole:   role,
		CapabilityInput:  input,
		Status:           agentstate.PlanStepStatusPending,
		RequiresApproval: matched.Spec.RequiresApproval,
		ExpectedEvidence: expectedEvidence,
	}
	backfillLegacyStepFields(&step)
	return agentstate.PlanState{
		Goal:   firstNonEmpty(normalizeQuery(session), matched.Spec.Description, matched.Name),
		PlanID: fmt.Sprintf("plan_%d", replanCount+1),
		Status: agentstate.PlanStatusActive,
		Steps: []agentstate.PlanStep{
			step,
		},
		CurrentStepIndex:   -1,
		ReplanCount:        replanCount,
		CompletionCriteria: []string{"selected capability returns enough evidence to finalize"},
		Confidence:         strings.TrimSpace(selection.Confidence),
		LastPlanReason:     reasonPlanBuilt,
	}
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

func buildSelectionStepTitle(matched agentresolve.MatchedCapability, selection selectcapability.CapabilitySelection) string {
	if trimmed := strings.TrimSpace(selection.Reason); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(matched.Spec.Description); trimmed != "" {
		return trimmed
	}
	return "Run selected capability " + matched.Name
}

func backfillLegacyStepFields(step *agentstate.PlanStep) {
	if step == nil || len(step.CapabilityInput) == 0 {
		return
	}
	switch strings.TrimSpace(step.CapabilityName) {
	case agentcapability.NameWebSearch:
		if query, ok := step.CapabilityInput["query"].(string); ok {
			step.Query = strings.TrimSpace(query)
		}
	case agentcapability.NameWebFetch:
		step.URLs = append([]string(nil), toStringSlice(step.CapabilityInput["urls"])...)
	}
}

func selectionFromStep(step agentstate.PlanStep) selectcapability.CapabilitySelection {
	return selectcapability.CapabilitySelection{
		Name:       step.CapabilityName,
		Kind:       step.CapabilityKind,
		Family:     step.CapabilityFamily,
		Role:       step.CapabilityRole,
		Input:      cloneInputMap(step.CapabilityInput),
		Reason:     step.Title,
		Confidence: "",
	}
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
		URLs:           append([]string(nil), step.URLs...),
	}
	switch value := output.(type) {
	case agentsearch.SearchOutput:
		result.URLs = append([]string(nil), value.URLs...)
	case *agentsearch.SearchOutput:
		if value != nil {
			result.URLs = append([]string(nil), value.URLs...)
		}
	}
	return result
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
