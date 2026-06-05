package planexecute

import (
	"context"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func branchAfterSelection(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil || session.Snapshot.Plan.CurrentStepIndex < 0 {
		return branchFinalize, nil
	}
	if session.Snapshot.Approval.Status == agentstate.ApprovalStatusPending && session.Snapshot.Approval.RerunNode == "execute_step" {
		return "approval", nil
	}
	return branchExecute, nil
}

func branchAfterAssessment(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil {
		return branchFinalize, nil
	}
	switch session.Snapshot.Execution.LastBranchTarget {
	case branchContinue:
		return "select_step", nil
	case branchReplan:
		return "build_plan", nil
	case "approval":
		return "approval", nil
	case branchDegrade, branchFinalize:
		return "finalize", nil
	default:
		return "finalize", nil
	}
}
