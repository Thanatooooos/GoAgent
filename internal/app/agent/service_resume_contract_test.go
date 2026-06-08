package agent

import (
	"context"
	"testing"
)

func TestServiceResumeAfterApproval_RequiresCheckpointID(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))

	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		Decision: ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected checkpoint id required error")
	}
	if ServiceErrorCode(err) != ErrorCodeCheckpointIDRequired {
		t.Fatalf("expected checkpoint id required code, got %q (%v)", ServiceErrorCode(err), err)
	}
	desc := DescribeServiceError(err)
	if desc.Kind != ErrorKindInvalidRequest || desc.Retryable {
		t.Fatalf("expected invalid_request non-retryable descriptor, got %+v", desc)
	}
}

func TestServiceResumeAfterApproval_RequiresSessionStore(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	service.sessionStore = nil

	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-no-store",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected session store not initialized error")
	}
	if ServiceErrorCode(err) != ErrorCodeSessionStoreNotInitialized {
		t.Fatalf("expected session store not initialized code, got %q (%v)", ServiceErrorCode(err), err)
	}
	desc := DescribeServiceError(err)
	if desc.Kind != ErrorKindInternal || desc.Retryable {
		t.Fatalf("expected internal non-retryable descriptor, got %+v", desc)
	}
}

func TestServiceResumeAfterApproval_DuplicateResumeAfterApproveReturnsNotFound(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "duplicate resume approve contract flow",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval initial outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval(first) error = %v", err)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected duplicate resume after approve to fail")
	}
	if ServiceErrorCode(err) != ErrorCodeApprovalSessionNotFound {
		t.Fatalf("expected approval session not found code, got %q (%v)", ServiceErrorCode(err), err)
	}
	desc := DescribeServiceError(err)
	if desc.Kind != ErrorKindNotFound || desc.Retryable {
		t.Fatalf("expected not_found non-retryable descriptor, got %+v", desc)
	}
}

func TestServiceResumeAfterApproval_DuplicateResumeAfterRejectReturnsNotFound(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "duplicate resume reject contract flow",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval initial outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval(first) error = %v", err)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
	})
	if err == nil {
		t.Fatal("expected duplicate resume after reject to fail")
	}
	if ServiceErrorCode(err) != ErrorCodeApprovalSessionNotFound {
		t.Fatalf("expected approval session not found code, got %q (%v)", ServiceErrorCode(err), err)
	}
	desc := DescribeServiceError(err)
	if desc.Kind != ErrorKindNotFound || desc.Retryable {
		t.Fatalf("expected not_found non-retryable descriptor, got %+v", desc)
	}
}
