package chat

import (
	"errors"
	"testing"

	agentapp "local/rag-project/internal/app/agent"
)

func TestNewRagChatAgentServiceErrorPayload_GenericErrorFallsBackToInternal(t *testing.T) {
	payload := newRagChatAgentServiceErrorPayload(errors.New("transport bridge failed"))
	if payload.Code != "" {
		t.Fatalf("expected empty code for generic error, got %+v", payload)
	}
	if payload.Message != "transport bridge failed" {
		t.Fatalf("expected generic message to be preserved, got %+v", payload)
	}
	if payload.Kind != agentapp.ErrorKindInternal || payload.Retryable {
		t.Fatalf("expected internal non-retryable payload, got %+v", payload)
	}
}

func TestBuildAgentRuntimeServiceErrorTraceExtra_UsesTypedDescriptor(t *testing.T) {
	extra := buildAgentRuntimeServiceErrorTraceExtra(&agentapp.ServiceError{
		Code:      agentapp.ErrorCodeApprovalSessionNotFound,
		Message:   "approval session not found",
		Kind:      agentapp.ErrorKindNotFound,
		Retryable: false,
	})
	if extra["status"] != "failed" {
		t.Fatalf("expected failed status in trace extra, got %+v", extra)
	}
	serviceError, ok := extra["serviceError"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured service error trace extra, got %+v", extra)
	}
	if serviceError["code"] != agentapp.ErrorCodeApprovalSessionNotFound ||
		serviceError["kind"] != agentapp.ErrorKindNotFound ||
		serviceError["message"] != "approval session not found" {
		t.Fatalf("unexpected service error trace payload: %+v", serviceError)
	}
}
