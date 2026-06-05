package agent

import (
	"strings"

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
