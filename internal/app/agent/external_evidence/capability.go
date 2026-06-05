package external_evidence

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

// CapabilityInput is the typed invocation input for the external evidence collection workflow.
type CapabilityInput struct {
	Query string `json:"query"`
}

// CapabilityOutput is the combined workflow output for external evidence collection.
type CapabilityOutput struct {
	Search agentsearch.SearchOutput `json:"search"`
	Fetch  agentfetch.Output        `json:"fetch"`
}

type capabilityAdapter struct {
	spec   agentcapability.Spec
	search agentcapability.Handle
	fetch  agentcapability.Handle
}

// NewCapability builds the high-level external evidence workflow capability.
func NewCapability(searchHandle agentcapability.Handle, fetchHandle agentcapability.Handle, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if searchHandle == nil {
		return nil, fmt.Errorf("search capability is required")
	}
	if fetchHandle == nil {
		return nil, fmt.Errorf("fetch capability is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameExternalEvidenceCollect,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyExternalEvidence,
		Roles:            []string{agentcapability.RoleCollectExternalEvidence},
		Description:      "Collects external evidence by searching the web and fetching readable page content.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		SupportsParallel: false,
		SupportsResume:   false,
		Dependencies: []string{
			agentcapability.NameWebSearch,
			agentcapability.NameWebFetch,
		},
		ProducesEvidence: true,
		Idempotency:      agentcapability.IdempotencyBestEffort,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "query",
				Requirement: "non_empty",
				Description: "Workflow requires a non-empty search query.",
			},
		},
	}
	for _, option := range options {
		if option != nil {
			option(&spec)
		}
	}

	return capabilityAdapter{
		spec:   spec,
		search: searchHandle,
		fetch:  fetchHandle,
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
				Summary: "external evidence collection rejected",
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
				Summary: "external evidence collection rejected",
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

	searchResult, err := c.search.Invoke(ctx, agentcapability.InvocationRequest{
		SessionID: req.SessionID,
		Snapshot:  req.Snapshot,
		Input:     agentsearch.CapabilityInput{Query: input.Query},
		Metadata:  req.Metadata,
	})
	if err != nil {
		return searchResult, err
	}
	searchOutput, err := decodeSearchOutput(searchResult.Output)
	if err != nil {
		return agentcapability.InvocationResult{}, err
	}

	fetchURLs := collectFetchURLs(searchOutput)
	fetchResult, err := c.fetch.Invoke(ctx, agentcapability.InvocationRequest{
		SessionID: req.SessionID,
		Snapshot:  req.Snapshot,
		Input:     agentfetch.CapabilityInput{URLs: fetchURLs},
		Metadata:  req.Metadata,
	})
	if err != nil {
		return fetchResult, err
	}
	fetchOutput, err := decodeFetchOutput(fetchResult.Output)
	if err != nil {
		return agentcapability.InvocationResult{}, err
	}

	status := agentcapability.StatusSucceeded
	if searchResult.Status == agentcapability.StatusDegraded || fetchResult.Status == agentcapability.StatusDegraded {
		status = agentcapability.StatusDegraded
	}
	if fetchResult.Status == agentcapability.StatusSkipped {
		status = agentcapability.StatusSkipped
	}

	return agentcapability.InvocationResult{
		Output: CapabilityOutput{
			Search: searchOutput,
			Fetch:  fetchOutput,
		},
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("collect external evidence for %q", strings.TrimSpace(input.Query)),
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    firstNonEmpty(fetchResult.Observation.Summary, searchResult.Observation.Summary),
			Degraded:   status == agentcapability.StatusDegraded,
			ErrorClass: firstNonEmpty(fetchResult.ErrorClass, searchResult.ErrorClass),
		},
		Delta:        agentstate.MergeStateDeltas(searchResult.Delta, fetchResult.Delta),
		Status:       status,
		ErrorClass:   firstNonEmpty(fetchResult.ErrorClass, searchResult.ErrorClass),
		EvidenceRefs: append(append([]agentstate.EvidenceRef(nil), searchResult.EvidenceRefs...), fetchResult.EvidenceRefs...),
	}, nil
}

func decodeCapabilityInput(raw any) (CapabilityInput, error) {
	input, err := agentcapability.DecodeStructuredInput[CapabilityInput](raw, "external evidence input is required")
	if err != nil {
		return CapabilityInput{}, fmt.Errorf("external evidence input has unexpected type %T: %w", raw, err)
	}
	return input, nil
}

func decodeSearchOutput(raw any) (agentsearch.SearchOutput, error) {
	switch value := raw.(type) {
	case agentsearch.SearchOutput:
		return value, nil
	case *agentsearch.SearchOutput:
		if value == nil {
			return agentsearch.SearchOutput{}, fmt.Errorf("search output is required")
		}
		return *value, nil
	default:
		return agentsearch.SearchOutput{}, fmt.Errorf("unexpected search output type %T", raw)
	}
}

func decodeFetchOutput(raw any) (agentfetch.Output, error) {
	switch value := raw.(type) {
	case agentfetch.Output:
		return value, nil
	case *agentfetch.Output:
		if value == nil {
			return agentfetch.Output{}, fmt.Errorf("fetch output is required")
		}
		return *value, nil
	default:
		return agentfetch.Output{}, fmt.Errorf("unexpected fetch output type %T", raw)
	}
}

func collectFetchURLs(output agentsearch.SearchOutput) []string {
	return append([]string(nil), output.URLs...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
