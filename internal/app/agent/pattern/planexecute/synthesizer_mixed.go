package planexecute

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func (s defaultPlanSynthesizer) buildMixedPlanFromSelection(session *agentruntime.RuntimeSession, matched agentresolve.MatchedCapability, selection selectcapability.CapabilitySelection) (agentstate.PlanState, bool) {
	if strings.TrimSpace(matched.Name) != agentcapability.NameDocumentInvestigation {
		return agentstate.PlanState{}, false
	}
	if !allowWebSearch(session) || s.registry == nil {
		return agentstate.PlanState{}, false
	}
	externalSpec, ok := s.registry.Spec(agentcapability.NameExternalEvidenceCollect)
	if !ok {
		return agentstate.PlanState{}, false
	}

	replanCount := 0
	if session != nil {
		replanCount = session.Snapshot.Plan.ReplanCount
	}
	investigateStep := buildSelectedPlanStep(matched, selection)
	evidenceStep := agentstate.PlanStep{
		StepID:           "step_collect_external_evidence",
		Title:            "Collect external evidence for the investigated document",
		Goal:             "gather external evidence that corroborates or contextualizes the document investigation result",
		CapabilityName:   externalSpec.Name,
		CapabilityKind:   externalSpec.Kind,
		CapabilityFamily: externalSpec.Family,
		CapabilityRole:   firstRoleOrEmpty(externalSpec.Roles),
		Consumes:         []string{artifactKindStructuredOutput},
		Produces:         []string{artifactKindSearchResults, artifactKindFetchResults, artifactKindEvidenceRefs, artifactKindStructuredOutput},
		CompletionPolicy: completionPolicyEvidence,
		FailurePolicy:    failurePolicyReplan,
		MaxAttempts:      1,
		Status:           agentstate.PlanStepStatusPending,
		RequiresApproval: externalSpec.RequiresApproval,
		ExpectedEvidence: []string{"corroborating external evidence related to the investigated document"},
	}

	return agentstate.PlanState{
		Goal:   firstNonEmpty(normalizeQuery(session), matched.Spec.Description, matched.Name),
		PlanID: fmt.Sprintf("plan_%d", replanCount+1),
		Status: agentstate.PlanStatusActive,
		Steps: []agentstate.PlanStep{
			investigateStep,
			evidenceStep,
		},
		CurrentStepIndex:   -1,
		ReplanCount:        replanCount,
		CompletionCriteria: []string{"direct document diagnosis plus corroborating external evidence when web search is allowed"},
		Confidence:         strings.TrimSpace(selection.Confidence),
		LastPlanReason:     reasonPlanBuilt,
	}, true
}

func allowWebSearch(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	if session.Snapshot.Request.RuntimeOptions.AllowWebSearch {
		return true
	}
	return session.Request.Options.AllowWebSearch
}

func firstRoleOrEmpty(roles []string) string {
	for _, role := range roles {
		if trimmed := strings.TrimSpace(role); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
