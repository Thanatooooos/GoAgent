package external_evidence

import (
	"context"
	"reflect"
	"strings"
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
	invoke func(context.Context, agentcapability.InvocationRequest) (agentcapability.InvocationResult, error)
}

func (h stubHandle) Spec() agentcapability.Spec {
	return h.spec
}

func (h stubHandle) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	if h.invoke != nil {
		return h.invoke(ctx, req)
	}
	return h.result, h.err
}

func TestCapabilityInvokeCombinesSearchReviewAndFetch(t *testing.T) {
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
				Summary: "found 3 results",
				Results: []agentsearch.SearchResultItem{
					{Title: "Neutral", URL: "https://neutral.example/doc", Policy: "neutral"},
					{Title: "Allowed", URL: "https://allow.example/doc", Policy: "allow"},
					{Title: "Denied", URL: "https://deny.example/doc", Policy: "deny"},
				},
				URLs: []string{
					"https://neutral.example/doc",
					"https://allow.example/doc",
				},
			},
			Observation: agentcapability.ObservationRecord{Summary: "found 3 results"},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					SearchQuery:      stringPtr("golang"),
					SearchErrorClass: stringPtr(""),
				},
			},
			Status: agentcapability.StatusSucceeded,
		},
	}

	var fetchedURLs []string
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
		invoke: func(_ context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
			input, ok := req.Input.(agentfetch.CapabilityInput)
			if !ok {
				t.Fatalf("expected fetch capability input, got %T", req.Input)
			}
			fetchedURLs = append([]string(nil), input.URLs...)
			return agentcapability.InvocationResult{
				Output: agentfetch.Output{
					Summary: "fetched 2 urls",
					Pages: []agentfetch.PageResult{
						{URL: "https://allow.example/doc", Text: "Allowed page content"},
						{URL: "https://neutral.example/doc", Text: "Neutral page content"},
					},
				},
				Observation: agentcapability.ObservationRecord{Summary: "fetched 2 urls"},
				Delta: agentstate.StateDelta{
					Context: &agentstate.ContextDelta{
						FetchErrorClass: stringPtr(""),
						FetchResults: []agentstate.FetchResultRef{
							{URL: "https://allow.example/doc", Text: "Allowed page content"},
							{URL: "https://neutral.example/doc", Text: "Neutral page content"},
						},
					},
				},
				Status: agentcapability.StatusSucceeded,
			}, nil
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
	if !reflect.DeepEqual(fetchedURLs, []string{"https://allow.example/doc", "https://neutral.example/doc"}) {
		t.Fatalf("expected source review to prioritize allow policy before neutral, got %v", fetchedURLs)
	}

	output, ok := result.Output.(CapabilityOutput)
	if !ok {
		t.Fatalf("expected workflow output type, got %T", result.Output)
	}
	if output.Search.Query != "golang" || len(output.Fetch.Pages) != 2 {
		t.Fatalf("unexpected workflow output: %+v", output)
	}
	if len(output.Review.Selected) != 2 || len(output.Review.Rejected) != 1 {
		t.Fatalf("expected source review output, got %+v", output.Review)
	}
	if output.Review.Readiness != readinessReady {
		t.Fatalf("expected ready review output, got %+v", output.Review)
	}
	if !reflect.DeepEqual(output.Review.CitedURLs, []string{"https://allow.example/doc", "https://neutral.example/doc"}) {
		t.Fatalf("expected cited urls from readable pages, got %+v", output.Review)
	}
	if result.Delta.Context == nil || result.Delta.Context.SearchQuery == nil || *result.Delta.Context.SearchQuery != "golang" || len(result.Delta.Context.FetchResults) != 2 {
		t.Fatalf("expected merged context delta, got %+v", result.Delta)
	}
	if len(result.Delta.Context.Notes) == 0 || !strings.Contains(result.Delta.Context.Notes[len(result.Delta.Context.Notes)-1], "readiness=ready") {
		t.Fatalf("expected source-review notes in merged delta, got %+v", result.Delta.Context)
	}
	if result.Delta.Context.SearchErrorClass == nil || *result.Delta.Context.SearchErrorClass != "" {
		t.Fatalf("expected merged context delta to preserve explicit search error reset, got %+v", result.Delta.Context)
	}
}

func TestCapabilityInvokeDegradesWhenReviewedSourcesProduceNoReadablePages(t *testing.T) {
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
				Query:   "empty evidence",
				Summary: "found 1 result",
				Results: []agentsearch.SearchResultItem{
					{Title: "Only Result", URL: "https://example.com/empty", Policy: "allow"},
				},
			},
			Observation: agentcapability.ObservationRecord{Summary: "found 1 result"},
			Status:      agentcapability.StatusSucceeded,
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
				Summary:       "fetch returned no readable content",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{URL: "https://example.com/empty", ErrorMessage: "temporary fetch failure"},
				},
			},
			Observation: agentcapability.ObservationRecord{Summary: "fetch returned no readable content"},
			Status:      agentcapability.StatusDegraded,
			ErrorClass:  agentcapability.ErrorClassExternal,
		},
	}

	handle, err := NewCapability(searchHandle, fetchHandle)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "empty evidence"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	output, ok := result.Output.(CapabilityOutput)
	if !ok {
		t.Fatalf("expected workflow output type, got %T", result.Output)
	}
	if result.Status != agentcapability.StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", result)
	}
	if output.Review.Readiness != readinessInsufficient || len(output.Review.CitedURLs) != 0 {
		t.Fatalf("expected insufficient review output, got %+v", output.Review)
	}
	if !strings.Contains(result.Observation.Summary, "readiness=insufficient") {
		t.Fatalf("expected readiness summary, got %+v", result.Observation)
	}
}

func stringPtr(value string) *string {
	return &value
}
