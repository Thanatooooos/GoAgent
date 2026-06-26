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
	return &agentstate.ApprovalDelta{
		Status:       &status,
		Reason:       stringPtrIfNotEmpty(reason),
		Node:         &node,
		Capability:   stringPtrIfNotEmpty(capability),
		CheckpointID: stringPtrIfNotEmpty(checkpointID),
		RerunNode:    stringPtrIfNotEmpty(rerunNode),
		RequestedAt:  &requestedAt,
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

func approvalReason(session *RuntimeSession) string {
	if session == nil {
		return "approval_required"
	}
	return firstNonEmpty(session.Snapshot.Approval.Reason, session.Snapshot.Evidence.SufficiencyReason, "approval_required")
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

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
