package search

import (
	"context"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

type stubCapabilityInvoker struct {
	lastQuery string
	output    SearchOutput
	err       error
}

func (s *stubCapabilityInvoker) Search(_ context.Context, query string) (SearchOutput, error) {
	s.lastQuery = query
	return s.output, s.err
}

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	invoker := &stubCapabilityInvoker{
		output: SearchOutput{
			Query:    "golang",
			Provider: "stub",
			Summary:  "found 1 result",
			Results: []SearchResultItem{
				{Title: "Go Docs", URL: "https://go.dev", Snippet: "Generics", Domain: "go.dev"},
			},
		},
	}
	handle, err := NewCapability(invoker, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "  golang  "},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if invoker.lastQuery != "golang" {
		t.Fatalf("expected trimmed query to be forwarded, got %q", invoker.lastQuery)
	}
	if handle.Spec().Name != agentcapability.NameWebSearch || !handle.Spec().RequiresApproval {
		t.Fatalf("unexpected capability spec: %+v", handle.Spec())
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	if result.Action.Name != agentcapability.NameWebSearch || result.Observation.Summary != "found 1 result" {
		t.Fatalf("unexpected invocation result metadata: %+v", result)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.SearchResults) != 1 {
		t.Fatalf("expected search delta to contain one search result, got %+v", result.Delta)
	}
}

func TestCapabilityInvokeRejectsUnexpectedInput(t *testing.T) {
	handle, err := NewCapability(&stubCapabilityInvoker{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: "bad"}); err == nil {
		t.Fatal("expected unexpected input type to fail")
	}
}

func TestCapabilityInvokeRejectsEmptyQueryByPrecondition(t *testing.T) {
	handle, err := NewCapability(&stubCapabilityInvoker{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "   "},
	}); err == nil {
		t.Fatal("expected empty query precondition to fail")
	}
}

func TestCapabilityInvokeClassifiesPermissionDegrade(t *testing.T) {
	invoker := &stubCapabilityInvoker{
		output: SearchOutput{
			Query:         "restricted source",
			Provider:      "stub",
			Summary:       "search requires approval",
			Degraded:      true,
			DegradeReason: "provider requires approval",
			ErrorMessage:  "permission denied by upstream provider",
		},
	}
	handle, err := NewCapability(invoker)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "restricted source"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", result)
	}
	if result.ErrorClass != agentcapability.ErrorClassPermission || result.Observation.ErrorClass != agentcapability.ErrorClassPermission {
		t.Fatalf("expected permission classification, got %+v", result)
	}
	if result.Delta.Context == nil || result.Delta.Context.SearchErrorClass == nil || *result.Delta.Context.SearchErrorClass != agentcapability.ErrorClassPermission {
		t.Fatalf("expected search error class delta to be permission_error, got %+v", result.Delta)
	}
}
