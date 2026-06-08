package planexecute

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

type defaultPlanSynthesizer struct {
	registry       *agentcapability.Registry
	searchSpec     agentcapability.Spec
	fetchSpec      agentcapability.Spec
	catalogBuilder agentcatalog.Builder
	selector       selectcapability.Selector
	resolver       agentresolve.Resolver
}

func newDefaultPlanSynthesizer(
	registry *agentcapability.Registry,
	searchSpec agentcapability.Spec,
	fetchSpec agentcapability.Spec,
	catalogBuilder agentcatalog.Builder,
	selector selectcapability.Selector,
	resolver agentresolve.Resolver,
) PlanSynthesizer {
	return defaultPlanSynthesizer{
		registry:       registry,
		searchSpec:     searchSpec,
		fetchSpec:      fetchSpec,
		catalogBuilder: catalogBuilder,
		selector:       selector,
		resolver:       resolver,
	}
}

func (s defaultPlanSynthesizer) Synthesize(ctx context.Context, input PlanSynthesisInput) (PlanSynthesisResult, error) {
	if selected, ok := s.synthesizeFromSelection(ctx, input.Session); ok {
		return selected, nil
	}
	return PlanSynthesisResult{
		Plan:      buildPlanFromSpecs(input.Session, s.searchSpec, s.fetchSpec),
		Reasoning: "built linear search-then-fetch plan",
		Notes:     []string{"built explicit plan-execute workflow"},
	}, nil
}

func (s defaultPlanSynthesizer) synthesizeFromSelection(ctx context.Context, session *agentruntime.RuntimeSession) (PlanSynthesisResult, bool) {
	if s.registry == nil || s.catalogBuilder == nil || s.selector == nil || s.resolver == nil {
		return PlanSynthesisResult{}, false
	}
	var contextNotes []string
	if session != nil {
		contextNotes = append([]string(nil), session.Snapshot.Context.Notes...)
	}
	cards, err := s.catalogBuilder.Build(s.registry)
	if err != nil || len(cards) == 0 {
		return PlanSynthesisResult{}, false
	}
	selectionOutput, err := s.selector.Select(ctx, selectcapability.SelectionInput{
		UserRequest:   normalizeQuery(session),
		ContextNotes:  contextNotes,
		Capabilities:  cards,
		MaxSelections: 1,
	})
	if err != nil || len(selectionOutput.Selections) == 0 {
		return PlanSynthesisResult{}, false
	}
	matched, err := s.resolver.Match(selectionOutput.Selections[0])
	if err != nil {
		return PlanSynthesisResult{}, false
	}
	if mixedPlan, ok := s.buildMixedPlanFromSelection(session, matched, selectionOutput.Selections[0]); ok {
		return PlanSynthesisResult{
			Plan:      mixedPlan,
			Reasoning: "built mixed-capability plan around " + matched.Name + " and " + agentcapability.NameExternalEvidenceCollect,
			Notes:     []string{"built mixed-capability plan around document investigation and external evidence collection"},
		}, true
	}
	return PlanSynthesisResult{
		Plan:      buildPlanFromSelection(session, matched, selectionOutput.Selections[0]),
		Reasoning: "built selector-driven plan around " + matched.Name,
		Notes:     []string{"built selector-driven plan around capability " + matched.Name},
	}, true
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
				Goal:             "find candidate external sources for the request",
				CapabilityName:   agentcapability.NameWebSearch,
				CapabilityKind:   agentcapability.KindTool,
				CapabilityFamily: agentcapability.FamilyExternalEvidence,
				CapabilityRole:   agentcapability.RoleSearch,
				CapabilityInput: map[string]any{
					"query": query,
				},
				Produces:         []string{"search_results", "urls"},
				CompletionPolicy: completionPolicySearchResults,
				FailurePolicy:    failurePolicyReplan,
				MaxAttempts:      1,
				Query:            query,
				Status:           agentstate.PlanStepStatusPending,
				ExpectedEvidence: []string{"fetchable search results"},
			},
			{
				StepID:           "step_fetch",
				Title:            "Fetch the best available source",
				Goal:             "retrieve readable source content from selected URLs",
				CapabilityName:   agentcapability.NameWebFetch,
				CapabilityKind:   agentcapability.KindTool,
				CapabilityFamily: agentcapability.FamilyExternalEvidence,
				CapabilityRole:   agentcapability.RoleFetch,
				CapabilityInput:  map[string]any{},
				Consumes:         []string{"urls"},
				Produces:         []string{"fetch_results", "evidence_refs"},
				CompletionPolicy: completionPolicyEvidence,
				FailurePolicy:    failurePolicyReplan,
				MaxAttempts:      1,
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
	step := buildSelectedPlanStep(matched, selection)
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

func buildSelectedPlanStep(matched agentresolve.MatchedCapability, selection selectcapability.CapabilitySelection) agentstate.PlanStep {
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
		Goal:             firstNonEmpty(strings.TrimSpace(selection.Reason), matched.Spec.Description, "complete the selected capability objective"),
		CapabilityName:   matched.Name,
		CapabilityKind:   matched.Spec.Kind,
		CapabilityFamily: matched.Spec.Family,
		CapabilityRole:   role,
		CapabilityInput:  input,
		Produces:         []string{artifactKindStructuredOutput},
		CompletionPolicy: completionPolicyStructuredOutput,
		FailurePolicy:    failurePolicyReplan,
		MaxAttempts:      1,
		Status:           agentstate.PlanStepStatusPending,
		RequiresApproval: matched.Spec.RequiresApproval,
		ExpectedEvidence: expectedEvidence,
	}
	if matched.Spec.ProducesEvidence {
		step.Produces = append(step.Produces, artifactKindEvidenceRefs)
		step.CompletionPolicy = completionPolicyEvidence
	}
	backfillLegacyStepFields(&step)
	return step
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
