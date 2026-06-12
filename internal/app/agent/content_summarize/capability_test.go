package content_summarize

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/framework/convention"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

type stubCompleter struct {
	lastRequest convention.ChatRequest
	response    string
	err         error
}

func (s *stubCompleter) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.lastRequest = request
	if s.err != nil {
		return "", s.err
	}
	if strings.TrimSpace(s.response) != "" {
		return s.response, nil
	}
	return "这是摘要", nil
}

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	completer := &stubCompleter{}
	handle, err := NewCapability(completer)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{
			Content:  "  long content body ",
			Purpose:  purposeEvidenceDigest,
			Language: "中文",
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok || output.Summary == "" || output.OriginalChars == 0 {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.Notes) == 0 {
		t.Fatalf("expected summarize note in delta, got %+v", result.Delta)
	}
	if len(completer.lastRequest.Messages) != 2 {
		t.Fatalf("expected chat request messages, got %+v", completer.lastRequest)
	}
}

func TestCapabilityInvokeRejectsUnexpectedInput(t *testing.T) {
	handle, err := NewCapability(&stubCompleter{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: 1}); err == nil {
		t.Fatal("expected unexpected input type to fail")
	}
}

func TestCapabilityInvokeRejectsEmptyContentByPrecondition(t *testing.T) {
	handle, err := NewCapability(&stubCompleter{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Content: "   "},
	}); err == nil {
		t.Fatal("expected empty content precondition to fail")
	}
}

func TestCapabilityInvokeDegradedOnExternalError(t *testing.T) {
	handle, err := NewCapability(&stubCompleter{err: context.Canceled})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	_, err = handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Content: "content"},
	})
	if err == nil {
		t.Fatal("expected external failure")
	}
}
