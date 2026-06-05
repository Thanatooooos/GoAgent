package agent

import (
	"context"
	"strings"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func (s *Service) ResumeAfterApproval(ctx context.Context, req ResumeApprovalRequest) (RunResponse, error) {
	final, outcome, err := s.resumeAfterApproval(ctx, req)
	if err != nil {
		return RunResponse{}, err
	}
	return RunResponse{
		Response: responseFromSession(final),
		Outcome:  outcome,
		Journal:  cloneJournal(final.Journal),
	}, nil
}

func (s *Service) ResumeHandoffAfterApproval(ctx context.Context, req ResumeApprovalRequest) (HandoffRunResponse, error) {
	final, outcome, err := s.resumeAfterApproval(ctx, req)
	if err != nil {
		return HandoffRunResponse{}, err
	}
	return HandoffRunResponse{
		Handoff: s.buildHandoff(final),
		Outcome: outcome,
	}, nil
}

func (s *Service) resumeAfterApproval(ctx context.Context, req ResumeApprovalRequest) (*agentruntime.RuntimeSession, RunOutcome, error) {
	if s == nil || s.runner == nil {
		return nil, RunOutcome{}, serviceError(ErrorCodeServiceNotInitialized, "agent service is not initialized")
	}
	if s.sessionStore == nil {
		return nil, RunOutcome{}, serviceError(ErrorCodeSessionStoreNotInitialized, "agent service session store is not initialized")
	}
	checkpointID := strings.TrimSpace(req.CheckpointID)
	if checkpointID == "" {
		return nil, RunOutcome{}, serviceError(ErrorCodeCheckpointIDRequired, "checkpoint id is required")
	}
	logAgentResumeStart(req)
	decision, err := resolveApprovalResumeDecision(req)
	if err != nil {
		logAgentExecutionError("resolve_approval_decision", "", checkpointID, err)
		return nil, RunOutcome{}, err
	}
	session, ok, err := s.sessionStore.Get(ctx, checkpointID)
	if err != nil {
		logAgentExecutionError("load_approval_session", "", checkpointID, err)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionLoadFailed, "failed to load approval session", "load_approval_session", err)
	}
	if !ok || session == nil {
		return nil, RunOutcome{}, serviceError(ErrorCodeApprovalSessionNotFound, "approval session not found for checkpoint "+checkpointID)
	}
	if !s.isAwaitingApproval(session) {
		return nil, RunOutcome{}, serviceError(ErrorCodeApprovalNotPending, "checkpoint "+checkpointID+" is not awaiting approval")
	}

	if err := s.applyApprovalDecision(session, checkpointID, req, decision); err != nil {
		logAgentExecutionError("apply_approval_decision", session.Request.TraceID, checkpointID, err)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeRuntimeExecutionFailed, "failed to apply approval decision", "apply_approval_decision", err)
	}
	if err := s.storePendingSession(ctx, checkpointID, session); err != nil {
		logAgentExecutionError("store_approval_decision", session.Request.TraceID, checkpointID, err)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionSaveFailed, "failed to persist approval decision", "store_approval_decision", err)
	}
	if strings.TrimSpace(session.Snapshot.Approval.Node) != "approval" && !decision.approved {
		final, finalizeErr := s.finalizeRejectedApproval(session)
		if finalizeErr != nil {
			logAgentExecutionError("finalize_rejected_approval", session.Request.TraceID, checkpointID, finalizeErr)
			return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeRuntimeExecutionFailed, "failed to finalize rejected approval", "finalize_rejected_approval", finalizeErr)
		}
		if err := s.deletePendingSession(ctx, checkpointID); err != nil {
			logAgentExecutionError("delete_pending_session_after_reject", session.Request.TraceID, checkpointID, err)
			return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionDeleteFailed, "failed to delete pending approval session", "delete_pending_session_after_reject", err)
		}
		outcome := s.outcomeFromSession(final)
		logAgentRunCompleted(final, outcome)
		return final, outcome, nil
	}

	final, runErr := s.runner.Resume(ctx, session, checkpointID)
	if runErr != nil {
		if s.normalizePendingApproval(final, checkpointID) {
			if storeErr := s.storePendingSession(ctx, checkpointID, final); storeErr != nil {
				logAgentExecutionError("store_pending_session_after_resume", session.Request.TraceID, checkpointID, storeErr)
				return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionSaveFailed, "failed to persist pending approval session after resume", "store_pending_session_after_resume", storeErr)
			}
			outcome := s.outcomeFromSession(final)
			logAgentRunCompleted(final, outcome)
			return final, outcome, nil
		}
		logAgentExecutionError("resume_after_approval", session.Request.TraceID, checkpointID, runErr)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeRuntimeExecutionFailed, "agent runtime resume failed", "resume_after_approval", runErr)
	}

	if s.isAwaitingApproval(final) {
		if err := s.storePendingSession(ctx, checkpointID, final); err != nil {
			logAgentExecutionError("store_pending_session_after_resume", session.Request.TraceID, checkpointID, err)
			return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionSaveFailed, "failed to persist pending approval session after resume", "store_pending_session_after_resume", err)
		}
		outcome := s.outcomeFromSession(final)
		logAgentRunCompleted(final, outcome)
		return final, outcome, nil
	}
	if err := s.deletePendingSession(ctx, checkpointID); err != nil {
		logAgentExecutionError("delete_pending_session_after_resume", session.Request.TraceID, checkpointID, err)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionDeleteFailed, "failed to delete pending approval session", "delete_pending_session_after_resume", err)
	}
	outcome := s.outcomeFromSession(final)
	logAgentRunCompleted(final, outcome)
	return final, outcome, nil
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
