package agent

import (
	"context"
	"testing"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestSessionBoundary_HandoffApprovedTerminalPathClearsCheckpointAndSessionAliases(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff session boundary approve",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for session boundary",
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval() error = %v", err)
	}

	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestSessionBoundary_RejectTerminalPathClearsCheckpointAndSessionAliases(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "reject session boundary",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "rejected for session boundary",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestSessionBoundary_ResumeRequestRemainsCheckpointOnlyEvenWhenSessionAliasExists(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "checkpoint only resume boundary",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.Approval.SessionID,
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected checkpoint-only contract to reject session-id lookup")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionNotFound, ErrorKindNotFound, false)
}

func TestSessionBoundary_CheckpointLookupDoesNotDependOnSessionAlias(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "checkpoint lookup independent of alias",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	if err := service.sessionStore.Delete(context.Background(), initial.Outcome.Approval.SessionID); err != nil {
		t.Fatalf("sessionStore.Delete(sessionID) error = %v", err)
	}

	resumed, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected checkpoint-key resume to continue even without session alias, got %+v", resumed.Outcome)
	}
}

func TestSessionBoundary_MemorySessionStoreDeleteIsIdempotent(t *testing.T) {
	store := agentruntime.NewMemorySessionStore()
	session := &agentruntime.RuntimeSession{SessionID: "idempotent-delete-session"}

	if err := store.Put(context.Background(), "idempotent-delete-checkpoint", session); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Delete(context.Background(), "idempotent-delete-checkpoint"); err != nil {
		t.Fatalf("Delete(first) error = %v", err)
	}
	if err := store.Delete(context.Background(), "idempotent-delete-checkpoint"); err != nil {
		t.Fatalf("Delete(second) error = %v", err)
	}

	stored, ok, err := store.Get(context.Background(), "idempotent-delete-checkpoint")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok || stored != nil {
		t.Fatalf("expected deleted checkpoint entry to remain absent, got ok=%v session=%+v", ok, stored)
	}
}
