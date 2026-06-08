package planexecute

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	assessmentSatisfied = "satisfied"
	assessmentFailed    = "failed"
)

type assessmentPolicyResult struct {
	disposition   string
	retryable     bool
	finalize      bool
	sufficient    bool
	successReason string
	failureReason string
	evidenceItems []agentstate.EvidenceItem
	contextDelta  *agentstate.ContextDelta
}

func assessStepCompletion(session *agentruntime.RuntimeSession, step agentstate.PlanStep, last agentstate.PlanStepResult) assessmentPolicyResult {
	switch completionPolicyForStep(step) {
	case completionPolicySearchResults:
		return assessSearchResultsPolicy(session, last)
	case completionPolicyFetchResults:
		return assessFetchResultsPolicy(session, last)
	case completionPolicyEvidence:
		return assessEvidencePolicy(session, step, last)
	case completionPolicyStructuredOutput:
		return assessStructuredOutputPolicy(last)
	case completionPolicyNonEmptyObservation:
		fallthrough
	default:
		return assessObservationPolicy(last)
	}
}

func completionPolicyForStep(step agentstate.PlanStep) string {
	if policy := strings.TrimSpace(step.CompletionPolicy); policy != "" {
		return policy
	}
	switch {
	case stepProduces(step, artifactKindSearchResults), strings.TrimSpace(step.CapabilityName) == agentcapability.NameWebSearch:
		return completionPolicySearchResults
	case stepProduces(step, artifactKindFetchResults):
		return completionPolicyFetchResults
	case stepProduces(step, artifactKindEvidenceRefs), strings.TrimSpace(step.CapabilityName) == agentcapability.NameWebFetch:
		return completionPolicyEvidence
	case stepProduces(step, artifactKindStructuredOutput), step.RequiresApproval:
		return completionPolicyStructuredOutput
	default:
		return completionPolicyNonEmptyObservation
	}
}

func assessSearchResultsPolicy(session *agentruntime.RuntimeSession, last agentstate.PlanStepResult) assessmentPolicyResult {
	hasSearchResults := len(artifactURLs(last.Artifacts)) > 0 || hasArtifactKind(last, artifactKindSearchResults)
	if !hasSearchResults && session != nil && len(session.Snapshot.Context.SearchResults) > 0 {
		hasSearchResults = true
	}
	if !hasSearchResults && len(last.URLs) > 0 {
		hasSearchResults = true
	}
	if last.Status == agentcapability.StatusSucceeded && hasSearchResults {
		return assessmentPolicyResult{
			disposition:   assessmentSatisfied,
			retryable:     true,
			finalize:      false,
			successReason: reasonSearchResultsReady,
		}
	}
	return assessmentPolicyResult{
		disposition:   assessmentFailed,
		retryable:     true,
		failureReason: reasonSearchResultsMissing,
	}
}

func assessFetchResultsPolicy(session *agentruntime.RuntimeSession, last agentstate.PlanStepResult) assessmentPolicyResult {
	hasFetchResults := hasArtifactKind(last, artifactKindFetchResults) || len(artifactStringValues(last.Artifacts, artifactKindFetchResults)) > 0
	if !hasFetchResults && session != nil && len(session.Snapshot.Context.FetchResults) > 0 {
		hasFetchResults = true
	}
	if !hasFetchResults && len(last.URLs) > 0 {
		hasFetchResults = true
	}
	if last.Status == agentcapability.StatusSucceeded && hasFetchResults {
		return assessmentPolicyResult{
			disposition:   assessmentSatisfied,
			retryable:     true,
			finalize:      false,
			successReason: reasonFetchResultsReady,
		}
	}
	return assessmentPolicyResult{
		disposition:   assessmentFailed,
		retryable:     true,
		failureReason: reasonFetchResultsMissing,
	}
}

func assessEvidencePolicy(session *agentruntime.RuntimeSession, step agentstate.PlanStep, last agentstate.PlanStepResult) assessmentPolicyResult {
	evidenceItems := evidenceItemsFromArtifacts(last.Artifacts)
	contextDelta := &agentstate.ContextDelta{}
	if stepProduces(step, artifactKindFetchResults) || strings.TrimSpace(step.CapabilityName) == agentcapability.NameWebFetch {
		if len(evidenceItems) == 0 {
			evidenceItems = newEvidenceFromFetch(session)
		}
		if len(last.URLs) > 0 {
			contextDelta.SeenURLs = append([]string(nil), last.URLs...)
		}
	}
	hasEvidence := last.ProducedEvidence || len(evidenceItems) > 0 || hasArtifactKind(last, artifactKindEvidenceRefs)
	if last.Status == agentcapability.StatusSucceeded && hasEvidence {
		return assessmentPolicyResult{
			disposition:   assessmentSatisfied,
			retryable:     true,
			finalize:      true,
			sufficient:    true,
			successReason: successReasonForEvidenceStep(step),
			evidenceItems: evidenceItems,
			contextDelta:  contextDelta,
		}
	}
	return assessmentPolicyResult{
		disposition:   assessmentFailed,
		retryable:     true,
		failureReason: failureReasonForEvidenceStep(step),
		evidenceItems: evidenceItems,
		contextDelta:  contextDelta,
	}
}

func assessStructuredOutputPolicy(last agentstate.PlanStepResult) assessmentPolicyResult {
	hasStructuredOutput := hasStructuredArtifact(last.Artifacts)
	if !hasStructuredOutput && strings.TrimSpace(last.Observation) != "" {
		hasStructuredOutput = true
	}
	if last.Status == agentcapability.StatusSucceeded && hasStructuredOutput {
		return assessmentPolicyResult{
			disposition:   assessmentSatisfied,
			retryable:     true,
			finalize:      false,
			successReason: reasonPlanCompleted,
		}
	}
	return assessmentPolicyResult{
		disposition:   assessmentFailed,
		retryable:     true,
		failureReason: reasonPlanFailed,
	}
}

func assessObservationPolicy(last agentstate.PlanStepResult) assessmentPolicyResult {
	if last.Status == agentcapability.StatusSucceeded && strings.TrimSpace(last.Observation) != "" {
		return assessmentPolicyResult{
			disposition:   assessmentSatisfied,
			retryable:     true,
			finalize:      false,
			successReason: reasonPlanCompleted,
		}
	}
	return assessmentPolicyResult{
		disposition:   assessmentFailed,
		retryable:     true,
		failureReason: reasonPlanFailed,
	}
}

func successReasonForEvidenceStep(step agentstate.PlanStep) string {
	if strings.TrimSpace(step.CapabilityName) == agentcapability.NameWebFetch {
		return reasonFetchEvidenceReady
	}
	return reasonPlanCompleted
}

func failureReasonForEvidenceStep(step agentstate.PlanStep) string {
	if strings.TrimSpace(step.CapabilityName) == agentcapability.NameWebFetch {
		return reasonFetchEvidenceMissing
	}
	return reasonPlanFailed
}

func hasArtifactKind(last agentstate.PlanStepResult, kind string) bool {
	target := strings.TrimSpace(kind)
	if target == "" {
		return false
	}
	for _, artifact := range last.Artifacts {
		if strings.TrimSpace(artifact.Kind) == target || strings.TrimSpace(artifact.Name) == target {
			return true
		}
	}
	return false
}

func stepAllowsReplan(step agentstate.PlanStep) bool {
	return strings.TrimSpace(step.FailurePolicy) != failurePolicyDegrade
}

func stepProduces(step agentstate.PlanStep, artifactKind string) bool {
	target := strings.TrimSpace(artifactKind)
	if target == "" {
		return false
	}
	for _, produced := range step.Produces {
		if strings.TrimSpace(produced) == target {
			return true
		}
	}
	return false
}
