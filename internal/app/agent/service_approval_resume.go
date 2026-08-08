package agent

import (
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

type approvalResumeDecision struct {
	value    string
	approved bool
}

func resolveApprovalResumeDecision(req ResumeApprovalRequest) (approvalResumeDecision, error) {
	switch strings.TrimSpace(req.Decision) {
	case "":
		if req.Approved {
			return approvalResumeDecision{value: agentstate.ApprovalStatusApproved, approved: true}, nil
		}
		return approvalResumeDecision{value: agentstate.ApprovalStatusRejected, approved: false}, nil
	case ApprovalDecisionApproved:
		return approvalResumeDecision{value: agentstate.ApprovalStatusApproved, approved: true}, nil
	case ApprovalDecisionRejected:
		return approvalResumeDecision{value: agentstate.ApprovalStatusRejected, approved: false}, nil
	default:
		return approvalResumeDecision{}, serviceError(
			ErrorCodeApprovalDecisionInvalid,
			`approval decision must be one of "approved" or "rejected"`,
		)
	}
}

func shouldFinalizeRejectedApprovalWithoutResume(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	if session.Checkpoint != nil {
		if node := strings.TrimSpace(session.Checkpoint.Node); node != "" {
			return node != "approval"
		}
	}
	if node := strings.TrimSpace(session.Snapshot.Execution.CurrentNode); node != "" {
		return node != "approval"
	}
	return strings.TrimSpace(session.Snapshot.Approval.Node) != "approval"
}

func approvalCheckpointMatchesRequest(session *agentruntime.RuntimeSession, checkpointID string) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.Snapshot.Approval.CheckpointID) == strings.TrimSpace(checkpointID)
}
