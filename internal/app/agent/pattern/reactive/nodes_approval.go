package reactive

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
			return agentruntime.BuildPendingApprovalNodeResult(session, "approval required before the runtime can continue"), nil
		}
		switch agentruntime.ResolveApprovalDecisionStatus(ctx, session, store) {
		case agentstate.ApprovalStatusApproved:
			return agentruntime.BuildApprovedApprovalNodeResult(session, "degrade", progressNone, "approval granted; resuming the gated capability"), nil
		case agentstate.ApprovalStatusRejected:
			return agentruntime.BuildRejectedApprovalNodeResult(session, "degrade", progressNone, "approval rejected; ending the run in degrade mode", false), nil
		default:
			return agentruntime.NodeResult{}, fmt.Errorf("approval decision is required before resume")
		}
	})
}

func branchAfterApproval(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil {
		return "degrade", nil
	}
	if target := session.Snapshot.Execution.LastBranchTarget; target != "" {
		return target, nil
	}
	if session.Snapshot.Approval.Status == agentstate.ApprovalStatusApproved && session.Snapshot.Approval.RerunNode != "" {
		return session.Snapshot.Approval.RerunNode, nil
	}
	return "degrade", nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
