package think

import (
	"context"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	handle, err := NewCapability()
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Thought: "  plan next step  "},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	if result.Action.Name != agentcapability.NameThink {
		t.Fatalf("unexpected action name: %+v", result.Action)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.Notes) != 1 || result.Delta.Context.Notes[0] != "plan next step" {
		t.Fatalf("unexpected think delta: %+v", result.Delta)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok || output.Thought != "plan next step" {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
}

func TestCapabilityInvokeRejectsUnexpectedInput(t *testing.T) {
	handle, err := NewCapability()
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: 1}); err == nil {
		t.Fatal("expected unexpected input type to fail")
	}
}

func TestCapabilityInvokeRejectsEmptyThoughtByPrecondition(t *testing.T) {
	handle, err := NewCapability()
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Thought: "   "},
	}); err == nil {
		t.Fatal("expected empty thought precondition to fail")
	}
}
