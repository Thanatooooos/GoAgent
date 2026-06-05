package search

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// CapabilityInput is the typed invocation input for the search capability.
type CapabilityInput struct {
	Query string `json:"query"`
}

type capabilityAdapter struct {
	spec    agentcapability.Spec
	invoker SearchInvoker
}

// SearchInvoker adapts the existing search service contract into a capability.
type SearchInvoker interface {
	Search(ctx context.Context, query string) (SearchOutput, error)
}

// NewCapability wraps an existing search invoker in a generic runtime capability.
func NewCapability(invoker SearchInvoker, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if invoker == nil {
		return nil, fmt.Errorf("search invoker is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameWebSearch,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyExternalEvidence,
		Roles:            []string{agentcapability.RoleSearch},
		Description:      "Searches external web sources and returns fetchable search results.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(SearchOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: false,
		SupportsResume:   false,
		ProducesEvidence: false,
		Idempotency:      agentcapability.IdempotencyIdempotent,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "query",
				Requirement: "non_empty",
				Description: "Search requires a non-empty normalized query.",
			},
		},
	}
	applyCapabilityOptions(&spec, options...)
	return capabilityAdapter{
		spec:    spec,
		invoker: invoker,
	}, nil
}

func applyCapabilityOptions(spec *agentcapability.Spec, options ...agentcapability.Option) {
	agentcapability.ApplyOptions(spec, options...)
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "search capability input is required", "search capability input")
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "search capability input is required", "search capability input")
	if err != nil {
		result := agentcapability.ValidationFailureResult(c.spec, "search invocation rejected", err)
		result.Delta = agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				SearchErrorClass: agentcapability.StringPtr(agentcapability.ErrorClassValidation),
				Notes:            agentcapability.AppendNonEmpty(nil, err.Error()),
			},
		}
		return result, err
	}

	query := strings.TrimSpace(input.Query)
	output, invokeErr := c.invoker.Search(ctx, query)
	if invokeErr != nil && strings.TrimSpace(output.Query) == "" {
		return agentcapability.ExternalFailureResult(c.spec, fmt.Sprintf("search web for %q", query), invokeErr), invokeErr
	}

	note := output.Summary
	status := agentcapability.StatusSucceeded
	if output.Degraded {
		note = agentcapability.FirstNonEmpty(output.DegradeReason, output.ErrorMessage, output.Summary)
		status = agentcapability.StatusDegraded
	}

	result := agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("search web for %q", agentcapability.FirstNonEmpty(output.Query, query)),
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    output.Summary,
			Degraded:   output.Degraded,
			ErrorClass: classificationForSearch(output),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				SearchQuery:          agentcapability.StringPtr(output.Query),
				SearchProvider:       agentcapability.StringPtr(output.Provider),
				SearchProviderActual: agentcapability.StringPtrIfNotEmpty(output.ProviderActual),
				SearchErrorClass:     agentcapability.StringPtr(classificationForSearch(output)),
				SearchResults:        toSearchRefs(output.Results, output.ProviderActual, output.Provider),
				Notes:                agentcapability.AppendNonEmpty(nil, note),
			},
		},
		Status:     status,
		ErrorClass: classificationForSearch(output),
	}
	if invokeErr != nil {
		return result, nil
	}
	return result, nil
}

func toSearchRefs(items []SearchResultItem, providerActual, provider string) []agentstate.SearchResultRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]agentstate.SearchResultRef, 0, len(items))
	for idx, item := range items {
		refs = append(refs, agentstate.SearchResultRef{
			ID:         fmt.Sprintf("search_%d", idx+1),
			Title:      item.Title,
			URL:        item.URL,
			Snippet:    item.Snippet,
			Source:     agentcapability.FirstNonEmpty(item.Domain, item.SourceType, providerActual, provider),
			Domain:     item.Domain,
			SourceType: item.SourceType,
			Policy:     item.Policy,
			RiskFlags:  append([]string(nil), item.RiskFlags...),
			Reasons:    append([]string(nil), item.Reasons...),
		})
	}
	return refs
}

func classificationForSearch(output SearchOutput) string {
	if !output.Degraded {
		return ""
	}
	if agentcapability.MatchesPermissionError(output.DegradeReason, output.ErrorMessage, output.Summary) {
		return agentcapability.ErrorClassPermission
	}
	if agentcapability.MatchesDependencyError(output.DegradeReason, output.ErrorMessage, output.Summary) {
		return agentcapability.ErrorClassDependency
	}
	return agentcapability.ErrorClassExternal
}
