package agent

import (
	"errors"
	"testing"
)

func TestDescribeServiceErrorCodeReturnsStableDescriptors(t *testing.T) {
	testCases := []struct {
		code      string
		message   string
		kind      string
		retryable bool
	}{
		{ErrorCodeServiceNotInitialized, "agent service is not initialized", ErrorKindInternal, false},
		{ErrorCodeQuestionRequired, "question is required", ErrorKindInvalidRequest, false},
		{ErrorCodeSessionStoreNotInitialized, "agent service session store is not initialized", ErrorKindInternal, false},
		{ErrorCodeCheckpointIDRequired, "checkpoint id is required", ErrorKindInvalidRequest, false},
		{ErrorCodeApprovalDecisionInvalid, `approval decision must be one of "approved" or "rejected"`, ErrorKindInvalidRequest, false},
		{ErrorCodeApprovalSessionLoadFailed, "failed to load approval session", ErrorKindUnavailable, true},
		{ErrorCodeApprovalSessionSaveFailed, "failed to persist approval session", ErrorKindUnavailable, true},
		{ErrorCodeApprovalSessionDeleteFailed, "failed to delete pending approval session", ErrorKindUnavailable, true},
		{ErrorCodeApprovalSessionNotFound, "approval session not found", ErrorKindNotFound, false},
		{ErrorCodeApprovalNotPending, "approval session is not awaiting approval", ErrorKindFailedPrecondition, false},
		{ErrorCodeRuntimeExecutionFailed, "agent runtime execution failed", ErrorKindUnavailable, true},
	}

	for _, tc := range testCases {
		t.Run(tc.code, func(t *testing.T) {
			desc := describeServiceErrorCode(tc.code)
			if desc.Code != tc.code || desc.Message != tc.message || desc.Kind != tc.kind || desc.Retryable != tc.retryable {
				t.Fatalf("unexpected descriptor for %q: %+v", tc.code, desc)
			}
		})
	}
}

func TestServiceErrorUsesDefaultDescriptorMessageWhenMessageEmpty(t *testing.T) {
	err := serviceError(ErrorCodeQuestionRequired, "")
	desc := DescribeServiceError(err)
	if desc.Code != ErrorCodeQuestionRequired {
		t.Fatalf("expected question_required code, got %+v", desc)
	}
	if desc.Message != "question is required" {
		t.Fatalf("expected default question message, got %+v", desc)
	}
	if desc.Kind != ErrorKindInvalidRequest || desc.Retryable {
		t.Fatalf("expected invalid_request non-retryable descriptor, got %+v", desc)
	}
}

func TestServiceErrorWrapUsesDefaultDescriptorMessageWhenMessageEmpty(t *testing.T) {
	cause := errors.New("storage unavailable")
	err := serviceErrorWrap(ErrorCodeApprovalSessionLoadFailed, "", "load_approval_session", cause)
	desc := DescribeServiceError(err)
	if desc.Code != ErrorCodeApprovalSessionLoadFailed {
		t.Fatalf("expected approval_session_load_failed code, got %+v", desc)
	}
	if desc.Message != "failed to load approval session" {
		t.Fatalf("expected default wrapped message, got %+v", desc)
	}
	if desc.Kind != ErrorKindUnavailable || !desc.Retryable {
		t.Fatalf("expected unavailable retryable descriptor, got %+v", desc)
	}
	var target *ServiceError
	if !errors.As(err, &target) || target == nil {
		t.Fatalf("expected wrapped service error, got %v", err)
	}
	if target.Operation != "load_approval_session" {
		t.Fatalf("expected operation to be preserved, got %+v", target)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause to remain discoverable, got %v", err)
	}
}

func TestServiceErrorCodeReturnsEmptyForNilAndGenericErrors(t *testing.T) {
	if got := ServiceErrorCode(nil); got != "" {
		t.Fatalf("expected empty code for nil error, got %q", got)
	}
	if got := ServiceErrorCode(errors.New("plain runtime failure")); got != "" {
		t.Fatalf("expected empty code for generic error, got %q", got)
	}
}

func TestDescribeServiceErrorNilReturnsZeroDescriptor(t *testing.T) {
	desc := DescribeServiceError(nil)
	if desc != (ServiceErrorDescriptor{}) {
		t.Fatalf("expected zero descriptor for nil error, got %+v", desc)
	}
}

func TestDescribeServiceError_GenericErrorFallsBackToInternalDescriptor(t *testing.T) {
	desc := DescribeServiceError(errors.New("plain runtime failure"))
	if desc.Code != "" {
		t.Fatalf("expected empty code for generic error, got %+v", desc)
	}
	if desc.Message != "plain runtime failure" {
		t.Fatalf("expected generic error message to be preserved, got %+v", desc)
	}
	if desc.Kind != ErrorKindInternal || desc.Retryable {
		t.Fatalf("expected internal non-retryable descriptor, got %+v", desc)
	}
}
