package reactive

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newApprovalNode(resumable bool, store agentruntime.SessionStore) (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("approval", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		if !resumable {
			return pendingApprovalResult(session), nil
		}
		switch approvalDecisionStatus(ctx, session, store) {
		case agentstate.ApprovalStatusApproved:
			return approvedApprovalResult(session), nil
		case agentstate.ApprovalStatusRejected:
			return rejectedApprovalResult(session), nil
		default:
			return agentruntime.NodeResult{}, fmt.Errorf("approval decision is required before resume")
		}
	})
}

func pendingApprovalResult(session *agentruntime.RuntimeSession) agentruntime.NodeResult {
	reason := approvalReason(session)
	note := "approval required before the runtime can continue"
	return agentruntime.NodeResult{
		Events: []agentstate.RuntimeEvent{
			agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "approval", agentstate.EventTypeInterrupt, reason),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: []string{note},
			},
			Execution: executionApprovalDelta(reason),
		},
	}
}

func approvedApprovalResult(session *agentruntime.RuntimeSession) agentruntime.NodeResult {
	target := session.Snapshot.Approval.RerunNode
	if target == "" {
		target = "degrade"
	}
	reason := firstNonEmpty(session.Snapshot.Approval.Reason, "approval_granted")
	note := "approval granted; resuming the gated capability"
	status := agentstate.ApprovalStatusApproved
	decisionNote := strings.TrimSpace(session.Metadata.ApprovalNote)
	reviewedAt := session.Snapshot.Approval.ReviewedAt
	if reviewedAt.IsZero() {
		now := time.Now()
		reviewedAt = now
	}
	return agentruntime.NodeResult{
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: []string{note},
			},
			Approval: &agentstate.ApprovalDelta{
				Status:       &status,
				ReviewedAt:   &reviewedAt,
				DecisionNote: stringPtrIfNotEmpty(decisionNote),
			},
			Execution: executionApprovalResolutionDelta(target, reason),
		},
		Decision: &agentruntime.DecisionArtifact{
			Kind:       "branch",
			Target:     target,
			Confidence: 0.80,
			Reasoning:  reason,
		},
	}
}

func rejectedApprovalResult(session *agentruntime.RuntimeSession) agentruntime.NodeResult {
	reason := "approval_rejected"
	note := "approval rejected; ending the run in degrade mode"
	status := agentstate.ApprovalStatusRejected
	decisionNote := strings.TrimSpace(session.Metadata.ApprovalNote)
	reviewedAt := session.Snapshot.Approval.ReviewedAt
	if reviewedAt.IsZero() {
		now := time.Now()
		reviewedAt = now
	}
	return agentruntime.NodeResult{
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: []string{note},
			},
			Evidence: &agentstate.EvidenceDelta{
				SufficiencyReason: &reason,
			},
			Approval: &agentstate.ApprovalDelta{
				Status:       &status,
				ReviewedAt:   &reviewedAt,
				DecisionNote: stringPtrIfNotEmpty(decisionNote),
			},
			Execution: executionApprovalResolutionDelta("degrade", reason),
		},
		Decision: &agentruntime.DecisionArtifact{
			Kind:       "branch",
			Target:     "degrade",
			Confidence: 0.90,
			Reasoning:  reason,
		},
	}
}

func branchAfterApproval(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil {
		return "degrade", nil
	}
	if target := session.Snapshot.Execution.LastBranchTarget; target != "" {
		return target, nil
	}
	if session.Snapshot.Approval.Status == agentstate.ApprovalStatusApproved && session.Snapshot.Approval.RerunNode != "" {
		return session.Snapshot.Approval.RerunNode, nil
	}
	return "degrade", nil
}

func approvalReason(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return "approval_required"
	}
	return firstNonEmpty(session.Snapshot.Approval.Reason, session.Snapshot.Evidence.SufficiencyReason, "approval_required")
}

func approvalDecisionStatus(ctx context.Context, session *agentruntime.RuntimeSession, store agentruntime.SessionStore) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
