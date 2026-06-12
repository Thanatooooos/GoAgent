package planexecute

import (
	"context"
	"time"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newSelectStepNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("select_step", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		plan := copyPlan(session.Snapshot.Plan)
		index := findFirstPendingStep(plan)
		reason := reasonNoActiveStep
		branch := branchFinalize
		if index >= 0 {
			plan.CurrentStepIndex = index
			prepareStepInputs(session, &plan.Steps[index])
			reason = plan.Steps[index].StepID
			branch = branchExecute
			if requiresRuntimeApproval(session, plan.Steps[index]) && session.Snapshot.Approval.Status != agentstate.ApprovalStatusApproved {
				branch = "approval"
				reason = plan.Steps[index].CapabilityName + "_approval_required"
			}
		} else {
			plan.CurrentStepIndex = -1
		}
		var approvalDelta *agentstate.ApprovalDelta
		if branch == "approval" && index >= 0 {
			status := agentstate.ApprovalStatusPending
			node := "approval"
			capability := plan.Steps[index].CapabilityName
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
		}
		if index >= 0 {
			logStepSelected(session, plan.Steps[index], branch, reason)
		}
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Plan: &agentstate.PlanDelta{
					Replace: &plan,
				},
				Approval:  approvalDelta,
				Execution: executionNodeDelta("select_step"),
			},
			Decision: &agentruntime.DecisionArtifact{
				Kind:       "branch",
				Target:     branch,
				Confidence: 0.81,
				Reasoning:  reason,
			},
		}, nil
	})
}
