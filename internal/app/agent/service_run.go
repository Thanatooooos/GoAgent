package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func (s *Service) Run(ctx context.Context, req Request) (Response, error) {
	result, err := s.RunDetailed(ctx, req)
	if err != nil {
		return Response{}, err
	}
	return result.Response, nil
}

func (s *Service) RunDetailed(ctx context.Context, req Request) (RunResponse, error) {
	final, outcome, err := s.runDetailedSession(ctx, req)
	if err != nil {
		return RunResponse{}, err
	}
	return RunResponse{
		Response: responseFromSession(final),
		Outcome:  outcome,
		Journal:  cloneJournal(final.Journal),
	}, nil
}

func (s *Service) RunHandoff(ctx context.Context, req Request) (HandoffResult, error) {
	result, err := s.RunHandoffDetailed(ctx, req)
	if err != nil {
		return HandoffResult{}, err
	}
	return result.Handoff, nil
}

func (s *Service) RunHandoffDetailed(ctx context.Context, req Request) (HandoffRunResponse, error) {
	final, outcome, err := s.runDetailedSession(ctx, req)
	if err != nil {
		return HandoffRunResponse{}, err
	}
	return HandoffRunResponse{
		Handoff: s.buildHandoff(final),
		Outcome: outcome,
	}, nil
}

func (s *Service) runDetailedSession(ctx context.Context, req Request) (*agentruntime.RuntimeSession, RunOutcome, error) {
	if s == nil || s.runner == nil {
		return nil, RunOutcome{}, serviceError(ErrorCodeServiceNotInitialized, "agent service is not initialized")
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, RunOutcome{}, serviceError(ErrorCodeQuestionRequired, "question is required")
	}

	session := newRuntimeSession(req, s.maxIterations, s.outputMode, s.runtimeName)
	logAgentRunStart(req, s.pattern, s.runtimeName, s.maxIterations)
	logAgentToolStageSeed(req, session)
	checkpointID := newCheckpointID(session)
	final, err := s.runner.RunWithCheckpoint(ctx, session, checkpointID)
	if err != nil {
		if s.normalizePendingApproval(final, checkpointID) {
			if storeErr := s.storePendingSession(ctx, checkpointID, final); storeErr != nil {
				logAgentExecutionError("store_pending_session", req.TraceID, checkpointID, storeErr)
				return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeApprovalSessionSaveFailed, "failed to persist pending approval session", "store_pending_session", storeErr)
			}
			outcome := s.outcomeFromSession(final)
			logAgentRunCompleted(final, outcome)
			return final, outcome, nil
		}
		logAgentExecutionError("run_with_checkpoint", req.TraceID, checkpointID, err)
		return nil, RunOutcome{}, serviceErrorWrap(ErrorCodeRuntimeExecutionFailed, "agent runtime execution failed", "run_with_checkpoint", err)
	}
	if deleteErr := s.deletePendingSession(ctx, checkpointID); deleteErr != nil {
		logAgentExecutionError("delete_pending_session", req.TraceID, checkpointID, deleteErr)
	}
	outcome := s.outcomeFromSession(final)
	logAgentRunCompleted(final, outcome)
	return final, outcome, nil
}

func (s *Service) buildHandoff(session *agentruntime.RuntimeSession) HandoffResult {
	if s != nil && s.handoff != nil {
		return s.handoff.Build(session)
	}
	return agenthandoff.Build(session)
}

func (s *Service) outcomeFromSession(session *agentruntime.RuntimeSession) RunOutcome {
	if session == nil {
		return RunOutcome{}
	}
	checkpointID := ""
	if session.Checkpoint != nil {
		checkpointID = strings.TrimSpace(session.Checkpoint.ID)
	}
	outcome := RunOutcome{
		Interrupted: session.Snapshot.Execution.Interrupted,
	}
	if s.isAwaitingApproval(session) {
		outcome.InterruptReason = firstNonEmpty(session.Snapshot.Approval.Reason, session.Snapshot.Execution.InterruptReason)
		outcome.CheckpointID = firstNonEmpty(session.Snapshot.Approval.CheckpointID, checkpointID)
		outcome.Status = RunStatusAwaitingApproval
		outcome.Approval = s.approvalPendingFromSession(session, checkpointID)
		return outcome
	}
	outcome.Interrupted = false
	if strings.TrimSpace(session.Snapshot.Answer.DegradeReason) != "" {
		outcome.Status = RunStatusDegraded
		return outcome
	}
	outcome.Status = RunStatusCompleted
	return outcome
}

func (s *Service) isAwaitingApproval(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.Snapshot.Approval.Status) == agentstate.ApprovalStatusPending
}

func (s *Service) normalizePendingApproval(session *agentruntime.RuntimeSession, checkpointID string) bool {
	if session == nil || !session.Snapshot.Execution.Interrupted {
		return false
	}
	now := time.Now()
	if s.isAwaitingApproval(session) {
		if strings.TrimSpace(session.Snapshot.Approval.CheckpointID) == "" {
			session.Snapshot.Approval.CheckpointID = firstNonEmpty(checkpointID, checkpointIDFromSession(session))
		}
		if session.Snapshot.Approval.RequestedAt.IsZero() {
			session.Snapshot.Approval.RequestedAt = now
		}
		return true
	}

	spec, capabilityName, rerunNode, ok := s.approvalCapabilityForNode(session.Snapshot.Execution.CurrentNode)
	if !ok || !spec.RequiresApproval {
		return false
	}
	session.Snapshot.Approval = agentstate.ApprovalState{
		Status:       agentstate.ApprovalStatusPending,
		Reason:       approvalRequiredReason(session.Snapshot.Execution.CurrentNode),
		Node:         session.Snapshot.Execution.CurrentNode,
		Capability:   capabilityName,
		CheckpointID: firstNonEmpty(checkpointID, checkpointIDFromSession(session)),
		RerunNode:    rerunNode,
		RequestedAt:  now,
	}
	return true
}

func (s *Service) approvalCapabilityForNode(node string) (agentcapability.Spec, string, string, bool) {
	if s == nil || s.registry == nil {
		return agentcapability.Spec{}, "", "", false
	}
	switch strings.TrimSpace(node) {
	case "search":
		return s.specFromRole(agentcapability.RoleSearch, "search")
	case "fetch":
		return s.specFromRole(agentcapability.RoleFetch, "fetch")
	case "external_evidence":
		return s.specFromRole(agentcapability.RoleCollectExternalEvidence, "external_evidence")
	default:
		return agentcapability.Spec{}, "", "", false
	}
}

func (s *Service) specFromRole(role string, rerunNode string) (agentcapability.Spec, string, string, bool) {
	name := ""
	if s.bindings != nil {
		name = s.bindings.Resolve(role)
	}
	if strings.TrimSpace(name) == "" && s.registry != nil {
		resolved, err := agentcapability.ResolveBinding(s.registry, s.bindings, role)
		if err == nil {
			name = resolved
		}
	}
	if strings.TrimSpace(name) == "" {
		return agentcapability.Spec{}, "", "", false
	}
	spec, ok := s.registry.Spec(name)
	return spec, name, rerunNode, ok
}

// storePendingSession persists one pending approval session under the canonical
// checkpoint lookup key and, when distinct, a secondary session-id alias.
func (s *Service) storePendingSession(ctx context.Context, checkpointID string, session *agentruntime.RuntimeSession) error {
	if s == nil || s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.Put(ctx, checkpointID, session); err != nil {
		return err
	}
	if session != nil && strings.TrimSpace(session.SessionID) != "" && strings.TrimSpace(session.SessionID) != strings.TrimSpace(checkpointID) {
		return s.sessionStore.Put(ctx, session.SessionID, session)
	}
	return nil
}

// deletePendingSession clears both the canonical checkpoint lookup entry and
// any secondary session-id alias for the same pending approval session.
func (s *Service) deletePendingSession(ctx context.Context, checkpointID string) error {
	if s == nil || s.sessionStore == nil || strings.TrimSpace(checkpointID) == "" {
		return nil
	}
	session, ok, err := s.sessionStore.Get(ctx, checkpointID)
	if err != nil {
		return err
	}
	if err := s.sessionStore.Delete(ctx, checkpointID); err != nil {
		return err
	}
	if !ok || session == nil {
		return nil
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" || sessionID == strings.TrimSpace(checkpointID) {
		return nil
	}
	return s.sessionStore.Delete(ctx, sessionID)
}

func checkpointIDFromSession(session *agentruntime.RuntimeSession) string {
	if session == nil || session.Checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(session.Checkpoint.ID)
}

func newCheckpointID(session *agentruntime.RuntimeSession) string {
	base := "agent"
	if session != nil && strings.TrimSpace(session.SessionID) != "" {
		base = strings.ReplaceAll(strings.TrimSpace(session.SessionID), " ", "_")
	}
	return fmt.Sprintf("approval-%s-%d", base, time.Now().UnixNano())
}

func approvalRequiredReason(node string) string {
	switch strings.TrimSpace(node) {
	case "search":
		return "search_approval_required"
	case "fetch":
		return "fetch_approval_required"
	case "external_evidence":
		return "external_evidence_approval_required"
	default:
		return "approval_required"
	}
}

func (s *Service) applySessionDelta(session *agentruntime.RuntimeSession, node string, delta agentstate.StateDelta, now time.Time) error {
	if session == nil {
		return nil
	}
	reducer := s.reducer
	if reducer == nil {
		reducer = agentstate.DefaultReducer{}
	}
	nextSnapshot, err := reducer.Apply(session.Snapshot, delta)
	if err != nil {
		return err
	}
	session.Snapshot = nextSnapshot
	session.Metadata.UpdatedAt = now
	event := agentstate.NewRuntimeEventAt(now, session.SessionID, node, agentstate.EventTypeStateApplied, "")
	cloned := agentstate.CloneDelta(delta)
	event.Delta = &cloned
	appendRuntimeEvent(session, event)
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
