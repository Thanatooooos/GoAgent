package planexecute

import (
	"context"
	"fmt"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newApprovalNode(resumable bool, store agentruntime.SessionStore) (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("approval", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		if !resumable {
			return agentruntime.BuildPendingApprovalNodeResult(session, "approval required before plan-execute can continue"), nil
		}
		switch agentruntime.ResolveApprovalDecisionStatus(ctx, session, store) {
		case agentstate.ApprovalStatusApproved:
			return agentruntime.BuildApprovedApprovalNodeResult(session, "finalize", progressPlanDegraded, "approval granted; resuming the gated plan step"), nil
		case agentstate.ApprovalStatusRejected:
			return agentruntime.BuildRejectedApprovalNodeResult(session, "finalize", progressPlanDegraded, "approval rejected; ending the run in degrade mode", true), nil
		default:
			return agentruntime.NodeResult{}, fmt.Errorf("approval decision is required before resume")
		}
	})
}

func branchAfterApproval(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil {
		return "finalize", nil
	}
	if target := session.Snapshot.Execution.LastBranchTarget; target != "" {
		return target, nil
	}
	if session.Snapshot.Approval.Status == agentstate.ApprovalStatusApproved && session.Snapshot.Approval.RerunNode != "" {
		return session.Snapshot.Approval.RerunNode, nil
	}
	return "finalize", nil
}
