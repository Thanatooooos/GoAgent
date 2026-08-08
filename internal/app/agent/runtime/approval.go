package runtime

import (
	"context"
	"strings"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

// ResolveApprovalDecisionStatus reads the runtime-owned approval decision from
// persisted session state first, then falls back to in-memory metadata.
func ResolveApprovalDecisionStatus(ctx context.Context, session *RuntimeSession, store SessionStore) string {
	if session == nil {
		return ""
	}
	if store != nil {
		keys := []string{
			strings.TrimSpace(session.Snapshot.Approval.CheckpointID),
			strings.TrimSpace(session.SessionID),
		}
		if session.Checkpoint != nil {
			keys = append(keys, strings.TrimSpace(session.Checkpoint.ID))
		}
		for _, key := range keys {
			if key == "" {
				continue
			}
			stored, ok, err := store.Get(ctx, key)
			if err == nil && ok && stored != nil {
				if decision := strings.TrimSpace(stored.Snapshot.Approval.Status); decision != "" && decision != agentstate.ApprovalStatusPending {
					return decision
				}
			}
		}
	}
	if decision := strings.TrimSpace(session.Metadata.ApprovalDecision); decision != "" {
		return decision
	}
	return strings.TrimSpace(session.Snapshot.Approval.Status)
}

// BuildPendingApprovalNodeResult produces the shared runtime contract for an
// approval-pending node interruption.
func BuildPendingApprovalNodeResult(session *RuntimeSession, note string) NodeResult {
	reason := approvalReason(session)
	return NodeResult{
		Events: []agentstate.RuntimeEvent{
			agentstate.NewRuntimeEventAt(time.Now(), sessionID(session), "approval", agentstate.EventTypeInterrupt, reason),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: []string{strings.TrimSpace(note)},
			},
			Execution: approvalPendingExecutionDelta(reason),
		},
	}
}

// BuildPendingApprovalDelta produces the shared runtime-owned approval state
// shape used when execution first transitions into pending approval.
func BuildPendingApprovalDelta(reason string, capability string, rerunNode string, checkpointID string, requestedAt time.Time) *agentstate.ApprovalDelta {
	status := agentstate.ApprovalStatusPending
	node := "approval"
	reviewedAt := time.Time{}
	decisionNote := ""
	return &agentstate.ApprovalDelta{
		Status:       &status,
		Reason:       stringPtrIfNotEmpty(reason),
		Node:         &node,
		Capability:   stringPtrIfNotEmpty(capability),
		CheckpointID: stringPtrIfNotEmpty(checkpointID),
		RerunNode:    stringPtrIfNotEmpty(rerunNode),
		RequestedAt:  &requestedAt,
		ReviewedAt:   &reviewedAt,
		DecisionNote: &decisionNote,
	}
}

// BuildApprovedApprovalNodeResult produces the shared runtime contract for
// resuming execution after approval.
func BuildApprovedApprovalNodeResult(session *RuntimeSession, fallbackTarget string, progressKind string, note string) NodeResult {
	target := firstNonEmpty(strings.TrimSpace(session.Snapshot.Approval.RerunNode), strings.TrimSpace(fallbackTarget))
	reason := firstNonEmpty(strings.TrimSpace(session.Snapshot.Approval.Reason), "approval_granted")
	status := agentstate.ApprovalStatusApproved
	decisionNote := strings.TrimSpace(session.Metadata.ApprovalNote)
	reviewedAt := session.Snapshot.Approval.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now()
	}
	return NodeResult{
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: []string{strings.TrimSpace(note)},
			},
			Approval: &agentstate.ApprovalDelta{
				Status:       &status,
				ReviewedAt:   &reviewedAt,
				DecisionNote: stringPtrIfNotEmpty(decisionNote),
			},
			Execution: approvalResolvedExecutionDelta(target, reason, progressKind),
		},
		Decision: &DecisionArtifact{
			Kind:       "branch",
			Target:     target,
			Confidence: 0.80,
			Reasoning:  reason,
		},
	}
}

// BuildRejectedApprovalNodeResult produces the shared runtime contract for
// rejecting a gated action. Some patterns also need an answer degrade reason.
func BuildRejectedApprovalNodeResult(session *RuntimeSession, target string, progressKind string, note string, includeAnswerDegrade bool) NodeResult {
	reason := "approval_rejected"
	status := agentstate.ApprovalStatusRejected
	decisionNote := strings.TrimSpace(session.Metadata.ApprovalNote)
	reviewedAt := session.Snapshot.Approval.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now()
	}
	delta := agentstate.StateDelta{
		Context: &agentstate.ContextDelta{
			Notes: []string{strings.TrimSpace(note)},
		},
		Evidence: &agentstate.EvidenceDelta{
			SufficiencyReason: &reason,
		},
		Approval: &agentstate.ApprovalDelta{
			Status:       &status,
			ReviewedAt:   &reviewedAt,
			DecisionNote: stringPtrIfNotEmpty(decisionNote),
		},
		Execution: approvalResolvedExecutionDelta(target, reason, progressKind),
	}
	if includeAnswerDegrade {
		delta.Answer = &agentstate.AnswerDelta{
			DegradeReason: &reason,
		}
	}
	return NodeResult{
		Delta: delta,
		Decision: &DecisionArtifact{
			Kind:       "branch",
			Target:     strings.TrimSpace(target),
			Confidence: 0.90,
			Reasoning:  reason,
		},
	}
}

// NormalizePendingApprovalSession ensures a pending approval session already
// produced by runtime/pattern code has complete shared approval state and one
// approval_pending lifecycle event before service projection/persistence.
func NormalizePendingApprovalSession(session *RuntimeSession, checkpointID string) bool {
	return normalizePendingApprovalSession(session, checkpointID, agentstate.DefaultReducer{})
}

func normalizePendingApprovalSession(session *RuntimeSession, checkpointID string, reducer agentstate.Reducer) bool {
	if session == nil || !isInterruptedSession(session) {
		return false
	}
	if strings.TrimSpace(session.Snapshot.Approval.Status) != agentstate.ApprovalStatusPending {
		return false
	}

	now := time.Now()
	finalCheckpointID := firstNonEmpty(
		strings.TrimSpace(session.Snapshot.Approval.CheckpointID),
		strings.TrimSpace(checkpointID),
		firstNonEmptyRuntimeCheckpointID(session, ""),
	)
	requestedAt := session.Snapshot.Approval.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = now
	}
	reason := approvalReason(session)
	node := firstNonEmpty(strings.TrimSpace(session.Snapshot.Approval.Node), "approval")
	delta := agentstate.StateDelta{
		Approval: &agentstate.ApprovalDelta{
			Status:       stringPtr(agentstate.ApprovalStatusPending),
			Reason:       stringPtr(reason),
			Node:         stringPtr(node),
			Capability:   stringPtrIfNotEmpty(strings.TrimSpace(session.Snapshot.Approval.Capability)),
			CheckpointID: stringPtrIfNotEmpty(finalCheckpointID),
			RerunNode:    stringPtrIfNotEmpty(strings.TrimSpace(session.Snapshot.Approval.RerunNode)),
			RequestedAt:  &requestedAt,
		},
		Execution: &agentstate.ExecutionDelta{
			Interrupted:     boolPtr(true),
			InterruptReason: stringPtr(reason),
		},
	}
	if err := newStateApplier(reducer).apply(session, "approval", delta, now); err != nil {
		return false
	}
	if !hasApprovalLifecycleEvent(session, agentstate.EventTypeApprovalPending, finalCheckpointID) {
		appendApprovalRuntimeEvent(session, "approval", agentstate.EventTypeApprovalPending, reason, finalCheckpointID)
	}
	return true
}

// ApplyApprovalDecision records the reviewed approval status in shared runtime
// state before any resume/finalize operation continues.
func ApplyApprovalDecision(session *RuntimeSession, checkpointID string, decision string, note string) error {
	return applyApprovalDecision(session, checkpointID, decision, note, agentstate.DefaultReducer{})
}

func applyApprovalDecision(session *RuntimeSession, checkpointID string, decision string, note string, reducer agentstate.Reducer) error {
	if session == nil {
		return nil
	}
	now := time.Now()
	trimmedDecision := strings.TrimSpace(decision)
	finalCheckpointID := firstNonEmpty(
		strings.TrimSpace(session.Snapshot.Approval.CheckpointID),
		strings.TrimSpace(checkpointID),
		firstNonEmptyRuntimeCheckpointID(session, ""),
	)
	if finalCheckpointID != "" {
		checkpointID = finalCheckpointID
	}
	reason := strings.TrimSpace(session.Snapshot.Approval.Reason)
	delta := agentstate.StateDelta{
		Approval: &agentstate.ApprovalDelta{
			Status:       &trimmedDecision,
			CheckpointID: stringPtrIfNotEmpty(finalCheckpointID),
			ReviewedAt:   &now,
			DecisionNote: stringPtr(strings.TrimSpace(note)),
		},
		Execution: &agentstate.ExecutionDelta{
			Interrupted:     boolPtr(true),
			InterruptReason: stringPtr(reason),
		},
	}
	if err := newStateApplier(reducer).apply(session, "approval", delta, now); err != nil {
		return err
	}
	session.Metadata.ApprovalDecision = trimmedDecision
	session.Metadata.ApprovalNote = strings.TrimSpace(note)
	session.Metadata.UpdatedAt = now

	eventType := agentstate.EventTypeApprovalResolved
	if trimmedDecision == agentstate.ApprovalStatusRejected {
		eventType = agentstate.EventTypeApprovalRejected
	}
	if !hasApprovalLifecycleEvent(session, eventType, finalCheckpointID) {
		appendApprovalRuntimeEvent(session, "approval", eventType, trimmedDecision, finalCheckpointID)
	}
	return nil
}

// FinalizeRejectedApproval converts a rejected approval into the stable
// degraded shared runtime state used by outward projections.
func FinalizeRejectedApproval(session *RuntimeSession) (*RuntimeSession, error) {
	return finalizeRejectedApproval(session, agentstate.DefaultReducer{})
}

func finalizeRejectedApproval(session *RuntimeSession, reducer agentstate.Reducer) (*RuntimeSession, error) {
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
	delta := agentstate.StateDelta{
		Evidence: &agentstate.EvidenceDelta{
			SufficiencyReason: &reason,
		},
		Approval: &agentstate.ApprovalDelta{
			Status:     stringPtr(agentstate.ApprovalStatusRejected),
			ReviewedAt: &reviewedAt,
		},
		Execution: &agentstate.ExecutionDelta{
			CurrentNode:      stringPtr("degrade"),
			LastBranchTarget: stringPtr("degrade"),
			LastBranchReason: stringPtr(reason),
			Interrupted:      boolPtr(false),
			InterruptReason:  stringPtr(""),
		},
		Answer: &agentstate.AnswerDelta{
			DegradeReason: &reason,
			Final:         &final,
		},
	}
	if err := newStateApplier(reducer).apply(session, "degrade", delta, now); err != nil {
		return nil, err
	}
	session.Metadata.ApprovalDecision = agentstate.ApprovalStatusRejected
	session.Metadata.UpdatedAt = now
	if !hasRuntimeEventTypeInSession(session, agentstate.EventTypeDegraded) {
		appendRuntimeEvent(session, agentstate.NewRuntimeEventAt(now, session.SessionID, "degrade", agentstate.EventTypeDegraded, reason))
	}
	return session, nil
}

// MergeApprovalResumeHistory preserves approval lifecycle events when the
// resumed runner returns a new session instance with a fresh journal.
func MergeApprovalResumeHistory(previous *RuntimeSession, current *RuntimeSession) {
	if previous == nil || current == nil || previous == current {
		return
	}
	if len(previous.Journal) == 0 {
		return
	}
	if len(current.Journal) == 0 {
		current.Journal = cloneRuntimeJournal(previous.Journal)
		return
	}
	if current.Journal[0].Sequence != 1 {
		return
	}
	if !hasRuntimeEventTypeInSession(previous, agentstate.EventTypeApprovalPending) ||
		hasRuntimeEventTypeInSession(current, agentstate.EventTypeApprovalPending) {
		return
	}

	merged := cloneRuntimeJournal(previous.Journal)
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

func approvalReason(session *RuntimeSession) string {
	if session == nil {
		return "approval_required"
	}
	return firstNonEmpty(session.Snapshot.Approval.Reason, session.Snapshot.Evidence.SufficiencyReason, "approval_required")
}

func appendRuntimeEvent(session *RuntimeSession, event agentstate.RuntimeEvent) {
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

func appendApprovalRuntimeEvent(session *RuntimeSession, node string, eventType string, payload string, checkpointID string) {
	if session == nil {
		return
	}
	event := agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, node, eventType, payload)
	if trimmed := strings.TrimSpace(checkpointID); trimmed != "" {
		event.Checkpoint = agentstate.NewCheckpointRef(trimmed, node)
	}
	appendRuntimeEvent(session, event)
}

func hasRuntimeEventTypeInSession(session *RuntimeSession, eventType string) bool {
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

func hasApprovalLifecycleEvent(session *RuntimeSession, eventType string, checkpointID string) bool {
	if session == nil {
		return false
	}
	for _, event := range session.Journal {
		if event.EventType != eventType {
			continue
		}
		if strings.TrimSpace(checkpointID) == "" {
			return true
		}
		if event.Checkpoint != nil && strings.TrimSpace(event.Checkpoint.ID) == strings.TrimSpace(checkpointID) {
			return true
		}
	}
	return false
}

func cloneRuntimeJournal(events []agentstate.RuntimeEvent) []agentstate.RuntimeEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]agentstate.RuntimeEvent, len(events))
	copy(cloned, events)
	return cloned
}

func approvalPendingExecutionDelta(reason string) *agentstate.ExecutionDelta {
	interrupted := true
	return &agentstate.ExecutionDelta{
		CurrentNode:      stringPtr("approval"),
		ScheduledActions: []string{"approval"},
		CompletedActions: []string{"approval"},
		Interrupted:      &interrupted,
		InterruptReason:  stringPtr(reason),
	}
}

func approvalResolvedExecutionDelta(target string, reason string, progressKind string) *agentstate.ExecutionDelta {
	interrupted := false
	return &agentstate.ExecutionDelta{
		CurrentNode:      stringPtr("approval"),
		ScheduledActions: []string{"approval"},
		CompletedActions: []string{"approval"},
		Interrupted:      &interrupted,
		InterruptReason:  stringPtr(""),
		LastBranchTarget: stringPtr(strings.TrimSpace(target)),
		LastBranchReason: stringPtr(reason),
		LastProgressKind: stringPtr(strings.TrimSpace(progressKind)),
	}
}

func sessionID(session *RuntimeSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.SessionID)
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
