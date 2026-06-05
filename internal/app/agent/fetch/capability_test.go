package fetch

import (
	"context"
	"reflect"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
)

type stubCapabilityInvoker struct {
	lastURLs []string
	output   Output
	err      error
}

func (s *stubCapabilityInvoker) Fetch(_ context.Context, urls []string) (Output, error) {
	s.lastURLs = append([]string(nil), urls...)
	return s.output, s.err
}

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	invoker := &stubCapabilityInvoker{
		output: Output{
			Summary: "fetched 2 urls",
			Pages: []PageResult{
				{URL: "https://a.example", Text: "A"},
				{URL: "https://b.example", Text: "B"},
			},
		},
	}
	handle, err := NewCapability(invoker, agentcapability.WithRiskLevel(agentcapability.RiskLevelLow), agentcapability.WithSupportsParallel(false))
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	inputURLs := []string{"https://a.example", "https://b.example"}
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{URLs: inputURLs},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if !reflect.DeepEqual(invoker.lastURLs, inputURLs) {
		t.Fatalf("expected urls to be forwarded, got %+v", invoker.lastURLs)
	}
	if handle.Spec().RiskLevel != agentcapability.RiskLevelLow || handle.Spec().SupportsParallel {
		t.Fatalf("unexpected capability spec override: %+v", handle.Spec())
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.FetchResults) != 2 {
		t.Fatalf("expected fetch delta to contain two pages, got %+v", result.Delta)
	}
}

func TestCapabilityInvokeSkipsWhenNoURLs(t *testing.T) {
	handle, err := NewCapability(&stubCapabilityInvoker{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusSkipped {
		t.Fatalf("expected skipped status, got %+v", result)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.Notes) == 0 {
		t.Fatalf("expected skip note in delta, got %+v", result.Delta)
	}
}

func TestCapabilityResolveAllowsSkipWhenNoURLs(t *testing.T) {
	handle, err := NewCapability(&stubCapabilityInvoker{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(handle); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	resolver := agentresolve.NewRegistryResolver(registry)

	resolved, err := resolver.Resolve(selectcapability.CapabilitySelection{
		Name:  agentcapability.NameWebFetch,
		Input: map[string]any{"urls": []string{}},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	result, err := resolved.Handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: resolved.Input,
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusSkipped {
		t.Fatalf("expected skipped status after resolver path, got %+v", result)
	}
}

func TestCapabilityInvokeClassifiesPermissionDegrade(t *testing.T) {
	invoker := &stubCapabilityInvoker{
		output: Output{
			Summary:       "fetch requires approval",
			Degraded:      true,
			DegradeReason: "provider requires approval",
			ErrorMessage:  "forbidden by upstream provider",
			Pages: []PageResult{
				{URL: "https://restricted.example", ErrorMessage: "403 forbidden"},
			},
		},
	}
	handle, err := NewCapability(invoker)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{URLs: []string{"https://restricted.example"}},
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
	if result.Delta.Context == nil || result.Delta.Context.FetchErrorClass == nil || *result.Delta.Context.FetchErrorClass != agentcapability.ErrorClassPermission {
		t.Fatalf("expected fetch error class delta to be permission_error, got %+v", result.Delta)
	}
}
