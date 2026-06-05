package planexecute

import (
	"context"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newAssessStepNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("assess_step", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		plan := copyPlan(session.Snapshot.Plan)
		currentStep, hasCurrent := currentPlanStep(plan)
		if !hasCurrent {
			return agentruntime.NodeResult{}, nil
		}
		last := plan.LastStepResult
		branch := branchDegrade
		reason := reasonPlanFailed
		progress := progressPlanDegraded
		sufficient := false
		evidenceItems := []agentstate.EvidenceItem(nil)
		contextDelta := &agentstate.ContextDelta{}
		var approvalDelta *agentstate.ApprovalDelta

		switch currentStep.CapabilityName {
		case agentcapability.NameWebSearch:
			if last.Status == agentcapability.StatusSucceeded && len(session.Snapshot.Context.SearchResults) > 0 {
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusCompleted
				branch = branchContinue
				reason = reasonSearchResultsReady
				progress = progressStepCompleted
			} else if canReplan(plan) {
				plan.ReplanCount++
				branch = branchReplan
				reason = reasonSearchResultsMissing
				progress = progressPlanReplanned
			} else {
				plan.Status = agentstate.PlanStatusDegraded
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusFailed
				branch = branchDegrade
				reason = reasonSearchResultsMissing
				progress = progressPlanDegraded
			}
		case agentcapability.NameWebFetch:
			evidenceItems = newEvidenceFromFetch(session)
			if len(last.URLs) > 0 {
				contextDelta.SeenURLs = append([]string(nil), last.URLs...)
			}
			if last.Status == agentcapability.StatusSucceeded && len(evidenceItems) > 0 {
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusCompleted
				plan.Status = agentstate.PlanStatusCompleted
				branch = branchFinalize
				reason = reasonFetchEvidenceReady
				progress = progressPlanFinalized
				sufficient = true
			} else if last.ErrorClass == agentcapability.ErrorClassPermission && requiresRuntimeApproval(session, currentStep) {
				branch = "approval"
				reason = currentStep.CapabilityName + "_approval_required"
				progress = progressPlanDegraded
				status := agentstate.ApprovalStatusPending
				node := "approval"
				capability := currentStep.CapabilityName
				rerunNode := "execute_step"
				requestedAt := time.Now()
				approvalDelta = &agentstate.ApprovalDelta{
					Status:      &status,
					Reason:      &reason,
					Node:        &node,
					Capability:  &capability,
					RerunNode:   &rerunNode,
					RequestedAt: &requestedAt,
				}
			} else if canReplan(plan) {
				plan.ReplanCount++
				branch = branchReplan
				reason = reasonFetchEvidenceMissing
				progress = progressPlanReplanned
			} else {
				plan.Status = agentstate.PlanStatusDegraded
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusFailed
				branch = branchDegrade
				reason = reasonFetchEvidenceMissing
				progress = progressPlanDegraded
			}
		default:
			if last.Status == agentcapability.StatusSucceeded && last.ProducedEvidence {
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusCompleted
				plan.Status = agentstate.PlanStatusCompleted
				branch = branchFinalize
				reason = reasonPlanCompleted
				progress = progressPlanFinalized
				sufficient = true
			} else if last.ErrorClass == agentcapability.ErrorClassPermission && requiresRuntimeApproval(session, currentStep) {
				branch = "approval"
				reason = currentStep.CapabilityName + "_approval_required"
				progress = progressPlanDegraded
				status := agentstate.ApprovalStatusPending
				node := "approval"
				capability := currentStep.CapabilityName
				rerunNode := "execute_step"
				requestedAt := time.Now()
				approvalDelta = &agentstate.ApprovalDelta{
					Status:      &status,
					Reason:      &reason,
					Node:        &node,
					Capability:  &capability,
					RerunNode:   &rerunNode,
					RequestedAt: &requestedAt,
				}
			} else if last.Status == agentcapability.StatusSucceeded {
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusCompleted
				branch = branchContinue
				reason = reasonPlanCompleted
				progress = progressStepCompleted
			} else if canReplan(plan) {
				plan.ReplanCount++
				branch = branchReplan
				reason = reasonPlanFailed
				progress = progressPlanReplanned
			} else {
				plan.Status = agentstate.PlanStatusDegraded
				plan.Steps[plan.CurrentStepIndex].Status = agentstate.PlanStepStatusFailed
				branch = branchDegrade
				reason = reasonPlanFailed
				progress = progressPlanDegraded
			}
		}

		plan.LastAssessment = reason
		if branch == branchContinue && findFirstPendingStep(plan) < 0 {
			branch = branchFinalize
			reason = reasonPlanCompleted
			progress = progressPlanFinalized
		}

		delta := agentstate.StateDelta{
			Plan: &agentstate.PlanDelta{
				Replace: &plan,
			},
			Context: contextDelta,
			Evidence: &agentstate.EvidenceDelta{
				AddItems:          evidenceItems,
				Sufficient:        boolPtr(sufficient),
				SufficiencyReason: stringPtr(reason),
				NewItemsThisRound: intPtr(len(evidenceItems)),
			},
			Approval:  approvalDelta,
			Execution: executionAssessDelta(branch, reason, progress),
		}
		if branch == branchDegrade {
			delta.Answer = &agentstate.AnswerDelta{
				DegradeReason: stringPtr(reason),
			}
		}
		if branch == branchReplan {
			delta.Context.SearchQuery = stringPtr(normalizeQuery(session))
		}

		return agentruntime.NodeResult{
			Delta: delta,
			Decision: &agentruntime.DecisionArtifact{
				Kind:       "branch",
				Target:     branch,
				Confidence: branchConfidence(branch, last),
				Reasoning:  reason,
			},
		}, nil
	})
}

func currentPlanStep(plan agentstate.PlanState) (agentstate.PlanStep, bool) {
	if plan.CurrentStepIndex < 0 || plan.CurrentStepIndex >= len(plan.Steps) {
		return agentstate.PlanStep{}, false
	}
	return plan.Steps[plan.CurrentStepIndex], true
}

func canReplan(plan agentstate.PlanState) bool {
	return plan.ReplanCount < defaultMaxReplans
}

func branchConfidence(branch string, result agentstate.PlanStepResult) float64 {
	switch branch {
	case branchFinalize:
		if result.ProducedEvidence {
			return 0.91
		}
		return 0.76
	case branchContinue:
		return 0.82
	case branchReplan:
		return 0.67
	default:
		if strings.TrimSpace(result.ErrorClass) == agentcapability.ErrorClassValidation {
			return 0.93
		}
		return 0.71
	}
}
