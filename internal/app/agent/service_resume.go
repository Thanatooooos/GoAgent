package agent

import (
	"context"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
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
	if s == nil || s.runtimeEngine == nil {
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
	if !approvalCheckpointMatchesRequest(session, checkpointID) {
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
	if !decision.approved && shouldFinalizeRejectedApprovalWithoutResume(session) {
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

	runResult, runErr := s.runtimeEngine.Resume(ctx, session, checkpointID)
	final := session
	if runResult != nil && runResult.Session != nil {
		final = runResult.Session
	}
	mergeApprovalResumeHistory(session, final)
	if runResult != nil && runResult.Outcome.Decision == agentruntime.DecisionWaitApproval {
		if s.normalizePendingApproval(final, checkpointID) {
			if storeErr := s.storePendingSession(ctx, checkpointID, final); storeErr != nil {
				logAgentExecutionError("store_pending_session_after_resume", session.Request.TraceID, checkpointID, storeErr)
				return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionSaveFailed, "failed to persist pending approval session after resume", "store_pending_session_after_resume", storeErr)
			}
			outcome := s.outcomeFromSession(final)
			logAgentRunCompleted(final, outcome)
			return final, outcome, nil
		}
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeRuntimeExecutionFailed, "agent runtime resume failed", "normalize_pending_approval_after_resume", runErr)
	}
	if runErr != nil {
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
