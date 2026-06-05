package external_evidence

import (
	"context"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

type stubHandle struct {
	spec   agentcapability.Spec
	result agentcapability.InvocationResult
	err    error
}

func (h stubHandle) Spec() agentcapability.Spec {
	return h.spec
}

func (h stubHandle) Invoke(_ context.Context, _ agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	return h.result, h.err
}

func TestCapabilityInvokeCombinesSearchAndFetch(t *testing.T) {
	searchHandle := stubHandle{
		spec: agentcapability.Spec{
			Name:         agentcapability.NameWebSearch,
			Kind:         agentcapability.KindTool,
			Family:       agentcapability.FamilyExternalEvidence,
			Roles:        []string{agentcapability.RoleSearch},
			Description:  "search",
			InputSchema:  agentcapability.NewSchema(agentsearch.CapabilityInput{}),
			OutputSchema: agentcapability.NewSchema(agentsearch.SearchOutput{}),
		},
		result: agentcapability.InvocationResult{
			Output: agentsearch.SearchOutput{
				Query:   "golang",
				Summary: "found 1 result",
				URLs:    []string{"https://go.dev"},
			},
			Observation: agentcapability.ObservationRecord{Summary: "found 1 result"},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					SearchQuery:      stringPtr("golang"),
					SearchErrorClass: stringPtr(""),
				},
			},
			Status: agentcapability.StatusSucceeded,
		},
	}
	fetchHandle := stubHandle{
		spec: agentcapability.Spec{
			Name:         agentcapability.NameWebFetch,
			Kind:         agentcapability.KindTool,
			Family:       agentcapability.FamilyExternalEvidence,
			Roles:        []string{agentcapability.RoleFetch},
			Description:  "fetch",
			InputSchema:  agentcapability.NewSchema(agentfetch.CapabilityInput{}),
			OutputSchema: agentcapability.NewSchema(agentfetch.Output{}),
		},
		result: agentcapability.InvocationResult{
			Output: agentfetch.Output{
				Summary: "fetched 1 urls",
				Pages: []agentfetch.PageResult{
					{URL: "https://go.dev", Text: "Generics"},
				},
			},
			Observation: agentcapability.ObservationRecord{Summary: "fetched 1 urls"},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					FetchErrorClass: stringPtr(""),
					FetchResults:    []agentstate.FetchResultRef{{URL: "https://go.dev"}},
				},
			},
			Status: agentcapability.StatusSucceeded,
		},
	}

	handle, err := NewCapability(searchHandle, fetchHandle)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "golang"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if handle.Spec().Kind != agentcapability.KindWorkflow || len(handle.Spec().Dependencies) != 2 {
		t.Fatalf("unexpected workflow spec: %+v", handle.Spec())
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok {
		t.Fatalf("expected workflow output type, got %T", result.Output)
	}
	if output.Search.Query != "golang" || len(output.Fetch.Pages) != 1 {
		t.Fatalf("unexpected workflow output: %+v", output)
	}
	if result.Delta.Context == nil || result.Delta.Context.SearchQuery == nil || *result.Delta.Context.SearchQuery != "golang" || len(result.Delta.Context.FetchResults) != 1 {
		t.Fatalf("expected merged context delta, got %+v", result.Delta)
	}
	if result.Delta.Context.SearchErrorClass == nil || *result.Delta.Context.SearchErrorClass != "" {
		t.Fatalf("expected merged context delta to preserve explicit search error reset, got %+v", result.Delta.Context)
	}
}

func stringPtr(value string) *string {
	return &value
}
