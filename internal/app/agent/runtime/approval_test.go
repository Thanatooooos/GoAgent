package runtime

import (
	"context"
	"testing"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

func TestResolveApprovalDecisionStatus_PrefersStoredReviewedDecision(t *testing.T) {
	store := NewMemorySessionStore()
	session := &RuntimeSession{
		SessionID: "sess-approval-status",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusPending,
				CheckpointID: "cp-approval-status",
			},
		},
	}
	stored := CloneSession(session)
	stored.Snapshot.Approval.Status = agentstate.ApprovalStatusApproved
	if err := store.Put(context.Background(), "cp-approval-status", stored); err != nil {
		t.Fatalf("store.Put() error = %v", err)
	}

	status := ResolveApprovalDecisionStatus(context.Background(), session, store)
	if status != agentstate.ApprovalStatusApproved {
		t.Fatalf("expected stored reviewed decision, got %q", status)
	}
}

func TestBuildPendingApprovalNodeResult_UsesSharedInterruptContract(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-pending-approval-result",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Reason: "fetch_approval_required",
			},
		},
	}

	result := BuildPendingApprovalNodeResult(session, "shared approval note")
	if len(result.Events) != 1 || result.Events[0].EventType != agentstate.EventTypeInterrupt {
		t.Fatalf("expected interrupt event, got %+v", result.Events)
	}
	if result.Delta.Execution == nil || result.Delta.Execution.Interrupted == nil || !*result.Delta.Execution.Interrupted {
		t.Fatalf("expected interrupted execution delta, got %+v", result.Delta.Execution)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.Notes) != 1 || result.Delta.Context.Notes[0] != "shared approval note" {
		t.Fatalf("expected shared note in context delta, got %+v", result.Delta.Context)
	}
}

func TestBuildPendingApprovalDelta_UsesSharedApprovalStateShape(t *testing.T) {
	requestedAt := time.Now()
	delta := BuildPendingApprovalDelta("fetch_approval_required", "web_fetch", "fetch", "cp-approval", requestedAt)
	if delta == nil || delta.Status == nil || *delta.Status != agentstate.ApprovalStatusPending {
		t.Fatalf("expected pending approval status, got %+v", delta)
	}
	if delta.Node == nil || *delta.Node != "approval" {
		t.Fatalf("expected shared approval node, got %+v", delta)
	}
	if delta.Capability == nil || *delta.Capability != "web_fetch" {
		t.Fatalf("expected capability passthrough, got %+v", delta)
	}
	if delta.RerunNode == nil || *delta.RerunNode != "fetch" {
		t.Fatalf("expected rerun node passthrough, got %+v", delta)
	}
	if delta.CheckpointID == nil || *delta.CheckpointID != "cp-approval" {
		t.Fatalf("expected checkpoint passthrough, got %+v", delta)
	}
	if delta.RequestedAt == nil || !delta.RequestedAt.Equal(requestedAt) {
		t.Fatalf("expected requested-at passthrough, got %+v", delta)
	}
}

func TestBuildApprovedApprovalNodeResult_UsesSharedResumeContract(t *testing.T) {
	reviewedAt := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-approved-approval-result",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Reason:      "fetch_approval_required",
				RerunNode:   "fetch",
				ReviewedAt:  reviewedAt,
				DecisionNote: "approved",
			},
		},
		Metadata: SessionMetadata{
			ApprovalNote: "approved",
		},
	}

	result := BuildApprovedApprovalNodeResult(session, "fallback", "progress_none", "approval granted")
	if result.Decision == nil || result.Decision.Target != "fetch" {
		t.Fatalf("expected rerun target from approval state, got %+v", result.Decision)
	}
	if result.Delta.Approval == nil || result.Delta.Approval.Status == nil || *result.Delta.Approval.Status != agentstate.ApprovalStatusApproved {
		t.Fatalf("expected approved approval delta, got %+v", result.Delta.Approval)
	}
	if result.Delta.Execution == nil || result.Delta.Execution.Interrupted == nil || *result.Delta.Execution.Interrupted {
		t.Fatalf("expected resume execution delta, got %+v", result.Delta.Execution)
	}
	if result.Delta.Execution.LastProgressKind == nil || *result.Delta.Execution.LastProgressKind != "progress_none" {
		t.Fatalf("expected shared progress kind, got %+v", result.Delta.Execution)
	}
}

func TestBuildRejectedApprovalNodeResult_CanOptionallySetDegradeReason(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-rejected-approval-result",
		Metadata: SessionMetadata{
			ApprovalNote: "rejected",
		},
	}

	result := BuildRejectedApprovalNodeResult(session, "finalize", "progress_plan_degraded", "approval rejected", true)
	if result.Decision == nil || result.Decision.Target != "finalize" {
		t.Fatalf("expected reject target, got %+v", result.Decision)
	}
	if result.Delta.Approval == nil || result.Delta.Approval.Status == nil || *result.Delta.Approval.Status != agentstate.ApprovalStatusRejected {
		t.Fatalf("expected rejected approval delta, got %+v", result.Delta.Approval)
	}
	if result.Delta.Answer == nil || result.Delta.Answer.DegradeReason == nil || *result.Delta.Answer.DegradeReason != "approval_rejected" {
		t.Fatalf("expected degrade reason when requested, got %+v", result.Delta.Answer)
	}
}
