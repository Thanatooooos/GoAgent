package agent

import (
	"errors"
	"testing"
)

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
