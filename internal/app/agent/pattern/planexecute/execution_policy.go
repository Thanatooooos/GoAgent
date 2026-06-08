package planexecute

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	progressStepRetried = "step_retried"
	progressStepSkipped = "step_skipped"
)

type executionDecision struct {
	branch            string
	reason            string
	progress          string
	sufficient        bool
	planStatus        string
	currentStepStatus string
	replan            bool
	retryCurrentStep  bool
}

func decideStepExecutionOutcome(plan agentstate.PlanState, step agentstate.PlanStep, last agentstate.PlanStepResult, assessment assessmentPolicyResult) executionDecision {
	if assessment.disposition == assessmentSatisfied {
		hasPendingNextStep := hasPendingStepsAfterCurrent(plan)
		decision := executionDecision{
			branch:            branchContinue,
			reason:            assessment.successReason,
			progress:          progressStepCompleted,
			currentStepStatus: agentstate.PlanStepStatusCompleted,
			sufficient:        assessment.sufficient && !hasPendingNextStep,
		}
		if assessment.finalize && !hasPendingNextStep {
			decision.branch = branchFinalize
			decision.progress = progressPlanFinalized
			decision.planStatus = agentstate.PlanStatusCompleted
		}
		return decision
	}

	if shouldRetryCurrentStep(step, last, assessment) {
		return executionDecision{
			branch:            branchContinue,
			reason:            failureReasonOrDefault(assessment.failureReason, reasonPlanFailed),
			progress:          progressStepRetried,
			currentStepStatus: agentstate.PlanStepStatusPending,
			retryCurrentStep:  true,
		}
	}

	if step.Optional {
		decision := executionDecision{
			branch:            branchContinue,
			reason:            failureReasonOrDefault(assessment.failureReason, reasonPlanFailed),
			progress:          progressStepSkipped,
			currentStepStatus: agentstate.PlanStepStatusSkipped,
		}
		if findFirstPendingStep(markCurrentStepStatus(plan, agentstate.PlanStepStatusSkipped)) < 0 {
			decision.branch = branchFinalize
			decision.reason = reasonPlanCompleted
			decision.progress = progressPlanFinalized
			decision.planStatus = agentstate.PlanStatusCompleted
		}
		return decision
	}

	if shouldReplanStep(plan, step, assessment) {
		return executionDecision{
			branch:            branchReplan,
			reason:            failureReasonOrDefault(assessment.failureReason, reasonPlanFailed),
			progress:          progressPlanReplanned,
			currentStepStatus: agentstate.PlanStepStatusFailed,
			replan:            true,
		}
	}

	return executionDecision{
		branch:            branchDegrade,
		reason:            failureReasonOrDefault(assessment.failureReason, reasonPlanFailed),
		progress:          progressPlanDegraded,
		planStatus:        agentstate.PlanStatusDegraded,
		currentStepStatus: agentstate.PlanStepStatusFailed,
	}
}

func shouldRetryCurrentStep(step agentstate.PlanStep, last agentstate.PlanStepResult, assessment assessmentPolicyResult) bool {
	if !assessment.retryable {
		return false
	}
	if strings.TrimSpace(last.ErrorClass) == agentcapability.ErrorClassPermission {
		return false
	}
	if stepMaxAttempts(step) <= step.AttemptCount {
		return false
	}
	return true
}

func shouldReplanStep(plan agentstate.PlanState, step agentstate.PlanStep, assessment assessmentPolicyResult) bool {
	if !assessment.retryable || !stepAllowsReplan(step) || !canReplan(plan) {
		return false
	}
	return stepMaxAttempts(step) <= step.AttemptCount
}

func stepMaxAttempts(step agentstate.PlanStep) int {
	if step.MaxAttempts <= 0 {
		return 1
	}
	return step.MaxAttempts
}

func failureReasonOrDefault(reason, fallback string) string {
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		return trimmed
	}
	return fallback
}

func hasPendingStepsAfterCurrent(plan agentstate.PlanState) bool {
	return findFirstPendingStep(plan) >= 0
}

func markCurrentStepStatus(plan agentstate.PlanState, status string) agentstate.PlanState {
	if plan.CurrentStepIndex < 0 || plan.CurrentStepIndex >= len(plan.Steps) {
		return plan
	}
	cloned := copyPlan(plan)
	cloned.Steps[cloned.CurrentStepIndex].Status = status
	return cloned
}
