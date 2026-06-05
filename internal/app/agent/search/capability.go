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

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return decodeCapabilityInput(raw)
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := decodeCapabilityInput(req.Input)
	if err != nil {
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: "search invocation rejected",
			},
			Observation: agentcapability.ObservationRecord{
				Summary:    err.Error(),
				Degraded:   true,
				ErrorClass: agentcapability.ErrorClassValidation,
			},
			Status:     agentcapability.StatusDegraded,
			ErrorClass: agentcapability.ErrorClassValidation,
		}, err
	}
	if err := agentcapability.ValidateInput(c.spec, input); err != nil {
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: "search invocation rejected",
			},
			Observation: agentcapability.ObservationRecord{
				Summary:    err.Error(),
				Degraded:   true,
				ErrorClass: agentcapability.ErrorClassValidation,
			},
			Status:     agentcapability.StatusDegraded,
			ErrorClass: agentcapability.ErrorClassValidation,
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					SearchErrorClass: stringPtr(agentcapability.ErrorClassValidation),
					Notes:            appendNonEmpty(nil, err.Error()),
				},
			},
		}, err
	}

	query := strings.TrimSpace(input.Query)
	output, invokeErr := c.invoker.Search(ctx, query)
	if invokeErr != nil && strings.TrimSpace(output.Query) == "" {
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: fmt.Sprintf("search web for %q", query),
			},
			Observation: agentcapability.ObservationRecord{
				Summary:    invokeErr.Error(),
				Degraded:   true,
				ErrorClass: agentcapability.ErrorClassExternal,
			},
			Status:     agentcapability.StatusDegraded,
			ErrorClass: agentcapability.ErrorClassExternal,
		}, invokeErr
	}

	note := output.Summary
	status := agentcapability.StatusSucceeded
	if output.Degraded {
		note = firstNonEmpty(output.DegradeReason, output.ErrorMessage, output.Summary)
		status = agentcapability.StatusDegraded
	}

	result := agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("search web for %q", firstNonEmpty(output.Query, query)),
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    output.Summary,
			Degraded:   output.Degraded,
			ErrorClass: classificationForSearch(output),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				SearchQuery:          stringPtr(output.Query),
				SearchProvider:       stringPtr(output.Provider),
				SearchProviderActual: stringPtrIfNotEmpty(output.ProviderActual),
				SearchErrorClass:     stringPtr(classificationForSearch(output)),
				SearchResults:        toSearchRefs(output.Results, output.ProviderActual, output.Provider),
				Notes:                appendNonEmpty(nil, note),
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

func decodeCapabilityInput(raw any) (CapabilityInput, error) {
	input, err := agentcapability.DecodeStructuredInput[CapabilityInput](raw, "search capability input is required")
	if err != nil {
		return CapabilityInput{}, fmt.Errorf("search capability input has unexpected type %T: %w", raw, err)
	}
	return input, nil
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
			Source:     firstNonEmpty(item.Domain, item.SourceType, providerActual, provider),
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
	if matchesPermissionError(output.DegradeReason, output.ErrorMessage, output.Summary) {
		return agentcapability.ErrorClassPermission
	}
	if matchesDependencyError(output.DegradeReason, output.ErrorMessage, output.Summary) {
		return agentcapability.ErrorClassDependency
	}
	return agentcapability.ErrorClassExternal
}

func applyCapabilityOptions(spec *agentcapability.Spec, options ...agentcapability.Option) {
	for _, option := range options {
		if option != nil {
			option(spec)
		}
	}
}

func appendNonEmpty(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func matchesPermissionError(values ...string) bool {
	return containsClassificationKeyword(values, "permission", "forbidden", "unauthorized", "approval", "access denied", "not allowed")
}

func matchesDependencyError(values ...string) bool {
	return containsClassificationKeyword(values, "dependency", "upstream unavailable", "provider unavailable")
}

func containsClassificationKeyword(values []string, keywords ...string) bool {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(normalized, keyword) {
				return true
			}
		}
	}
	return false
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
