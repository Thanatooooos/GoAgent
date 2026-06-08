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
		assessment := assessStepCompletion(session, currentStep, last)
		evidenceItems = append(evidenceItems, assessment.evidenceItems...)
		if assessment.contextDelta != nil {
			contextDelta = assessment.contextDelta
		}

		switch {
		case assessment.disposition == assessmentSatisfied:
			decision := decideStepExecutionOutcome(plan, currentStep, last, assessment)
			plan.Steps[plan.CurrentStepIndex].Status = decision.currentStepStatus
			plan.Status = firstNonEmpty(decision.planStatus, plan.Status)
			reason = decision.reason
			branch = decision.branch
			progress = decision.progress
			sufficient = decision.sufficient
		case last.ErrorClass == agentcapability.ErrorClassPermission && requiresRuntimeApproval(session, currentStep):
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
		default:
			decision := decideStepExecutionOutcome(plan, currentStep, last, assessment)
			plan.Steps[plan.CurrentStepIndex].Status = decision.currentStepStatus
			if decision.replan {
				plan.ReplanCount++
			}
			plan.Status = firstNonEmpty(decision.planStatus, plan.Status)
			branch = decision.branch
			reason = decision.reason
			progress = decision.progress
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
