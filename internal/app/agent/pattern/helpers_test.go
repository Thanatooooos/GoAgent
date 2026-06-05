package pattern

import (
	"context"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
)

func TestResolveNamedBindings(t *testing.T) {
	searchHandle, err := agentsearch.NewCapability(stubPatternSearchInvoker{})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubPatternFetchInvoker{})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchHandle, fetchHandle} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	resolved, err := ResolveNamedBindings(registry, nil, "reactive", agentcapability.RoleSearch, agentcapability.RoleFetch)
	if err != nil {
		t.Fatalf("ResolveNamedBindings() error = %v", err)
	}
	if resolved.Resolve(agentcapability.RoleSearch) != agentcapability.NameWebSearch {
		t.Fatalf("expected search binding, got %+v", resolved)
	}
	if resolved.Resolve(agentcapability.RoleFetch) != agentcapability.NameWebFetch {
		t.Fatalf("expected fetch binding, got %+v", resolved)
	}
}

func TestResolveNamedBindingPrefixesScopeInErrors(t *testing.T) {
	registry := agentcapability.NewRegistry()
	_, err := ResolveNamedBinding(registry, nil, "plan-execute", agentcapability.RoleSearch)
	if err == nil {
		t.Fatal("expected binding resolution error")
	}
	if !strings.Contains(err.Error(), "plan-execute search binding") {
		t.Fatalf("expected scoped error prefix, got %v", err)
	}
}

type stubPatternSearchInvoker struct{}

func (stubPatternSearchInvoker) Search(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
	return agentsearch.SearchOutput{}, nil
}

type stubPatternFetchInvoker struct{}

func (stubPatternFetchInvoker) Fetch(_ context.Context, _ []string) (agentfetch.Output, error) {
	return agentfetch.Output{}, nil
}
