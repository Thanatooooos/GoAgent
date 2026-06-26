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
	eventType := agentstate.EventTypeApprovalResolved
	if !decision.approved {
		eventType = agentstate.EventTypeApprovalRejected
	}
	appendApprovalRuntimeEvent(session, "approval", eventType, decision.value, finalCheckpointID)
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

func appendApprovalResolvedIfMissing(session *agentruntime.RuntimeSession, checkpointID string) {
	if session == nil || hasRuntimeEventTypeInSession(session, agentstate.EventTypeApprovalResolved) {
		return
	}
	appendApprovalRuntimeEvent(session, "approval", agentstate.EventTypeApprovalResolved, agentstate.ApprovalStatusApproved, checkpointID)
}

func mergeApprovalResumeHistory(previous *agentruntime.RuntimeSession, current *agentruntime.RuntimeSession) {
	if previous == nil || current == nil || previous == current {
		return
	}
	if len(previous.Journal) == 0 {
		return
	}
	if len(current.Journal) == 0 {
		current.Journal = cloneJournal(previous.Journal)
		return
	}
	if current.Journal[0].Sequence != 1 {
		return
	}
	if !hasRuntimeEventTypeInSession(previous, agentstate.EventTypeApprovalPending) ||
		hasRuntimeEventTypeInSession(current, agentstate.EventTypeApprovalPending) {
		return
	}

	merged := cloneJournal(previous.Journal)
	start := 0
	if current.Journal[0].EventType == agentstate.EventTypeSessionStarted {
		start = 1
	}
	for i := start; i < len(current.Journal); i++ {
		event := current.Journal[i]
		event.Sequence = len(merged) + 1
		if strings.TrimSpace(event.SessionID) == "" {
			event.SessionID = current.SessionID
		}
		merged = append(merged, event)
	}
	current.Journal = merged
}

func hasRuntimeEventTypeInSession(session *agentruntime.RuntimeSession, eventType string) bool {
	if session == nil {
		return false
	}
	for _, event := range session.Journal {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}
