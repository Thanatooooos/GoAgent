package agent

import (
	"strings"
	"time"

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

func (s *Service) applyApprovalDecision(session *agentruntime.RuntimeSession, checkpointID string, req ResumeApprovalRequest, decision approvalResumeDecision) error {
	if session == nil {
		return nil
	}
	now := time.Now()
	reason := session.Snapshot.Approval.Reason
	finalCheckpointID := firstNonEmpty(session.Snapshot.Approval.CheckpointID, checkpointID)
	if err := s.applySessionDelta(session, "approval", agentstate.StateDelta{
		Approval: &agentstate.ApprovalDelta{
			Status:       stringPtr(decision.value),
			CheckpointID: stringPtr(finalCheckpointID),
			DecisionNote: stringPtr(strings.TrimSpace(req.DecisionNote)),
			ReviewedAt:   &now,
		},
		Execution: &agentstate.ExecutionDelta{
			InterruptReason: stringPtr(reason),
		},
	}, now); err != nil {
		return err
	}
	session.Metadata.ApprovalDecision = decision.value
	session.Metadata.ApprovalNote = strings.TrimSpace(req.DecisionNote)
	session.Metadata.UpdatedAt = now
	return nil
}

func (s *Service) finalizeRejectedApproval(session *agentruntime.RuntimeSession) (*agentruntime.RuntimeSession, error) {
	if session == nil {
		return nil, nil
	}
	now := time.Now()
	reason := "approval_rejected"
	final := "I couldn't continue because the required approval was not granted."
	reviewedAt := session.Snapshot.Approval.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = now
	}

	if err := s.applySessionDelta(session, "degrade", agentstate.StateDelta{
		Approval: &agentstate.ApprovalDelta{
			Status:     stringPtr(agentstate.ApprovalStatusRejected),
			ReviewedAt: &reviewedAt,
		},
		Evidence: &agentstate.EvidenceDelta{
			SufficiencyReason: stringPtr(reason),
		},
		Execution: &agentstate.ExecutionDelta{
			CurrentNode:      stringPtr("degrade"),
			LastBranchTarget: stringPtr("degrade"),
			LastBranchReason: stringPtr(reason),
			Interrupted:      boolPtr(false),
			InterruptReason:  stringPtr(""),
		},
		Answer: &agentstate.AnswerDelta{
			DegradeReason: stringPtr(reason),
			Final:         stringPtr(final),
		},
	}, now); err != nil {
		return nil, err
	}
	session.Metadata.ApprovalDecision = agentstate.ApprovalStatusRejected
	session.Metadata.UpdatedAt = now

	appendRuntimeEvent(session, agentstate.NewRuntimeEventAt(now, session.SessionID, "degrade", agentstate.EventTypeDegraded, reason))
	return session, nil
}

func appendRuntimeEvent(session *agentruntime.RuntimeSession, event agentstate.RuntimeEvent) {
	if session == nil {
		return
	}
	event.Sequence = len(session.Journal) + 1
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if strings.TrimSpace(event.SessionID) == "" {
		event.SessionID = session.SessionID
	}
	session.Journal = append(session.Journal, event)
}

func approvalCheckpointMatchesRequest(session *agentruntime.RuntimeSession, checkpointID string) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.Snapshot.Approval.CheckpointID) == strings.TrimSpace(checkpointID)
}
