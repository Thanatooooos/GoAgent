package fetch

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// CapabilityInput is the typed invocation input for the fetch capability.
type CapabilityInput struct {
	URLs []string `json:"urls"`
}

type capabilityAdapter struct {
	spec    agentcapability.Spec
	invoker FetchInvoker
}

// FetchInvoker adapts the existing fetch service contract into a capability.
type FetchInvoker interface {
	Fetch(ctx context.Context, urls []string) (Output, error)
}

// NewCapability wraps an existing fetch invoker in a generic runtime capability.
func NewCapability(invoker FetchInvoker, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if invoker == nil {
		return nil, fmt.Errorf("fetch invoker is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameWebFetch,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyExternalEvidence,
		Roles:            []string{agentcapability.RoleFetch},
		Description:      "Fetches page content for selected URLs and extracts readable text.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(Output{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		SupportsParallel: true,
		SupportsResume:   false,
		ProducesEvidence: true,
		Idempotency:      agentcapability.IdempotencyBestEffort,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "urls",
				Requirement: "non_empty",
				Description: "Fetch requires at least one URL candidate.",
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
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "fetch capability input is required", "fetch capability input")
}

func (c capabilityAdapter) NormalizeResolverInput(raw any) (any, error) {
	return agentcapability.DecodeInput[CapabilityInput](raw, "fetch capability input is required", "fetch capability input")
}

func (c capabilityAdapter) ResolveOnPreconditionFailure(err error) bool {
	return agentcapability.IsPreconditionError(err)
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "fetch capability input is required", "fetch capability input")
	if err != nil {
		if !agentcapability.IsPreconditionError(err) {
			return agentcapability.ValidationFailureResult(c.spec, "fetch invocation rejected", err), err
		}
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: "skip web fetch",
			},
			Observation: agentcapability.ObservationRecord{
				Summary:  "web fetch skipped: no fetchable urls",
				Degraded: false,
			},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					Notes: agentcapability.AppendNonEmpty(nil, "web fetch skipped: no fetchable urls"),
				},
			},
			Status: agentcapability.StatusSkipped,
		}, nil
	}

	urls := normalizeURLs(input.URLs)
	output, invokeErr := c.invoker.Fetch(ctx, urls)
	if invokeErr != nil && len(output.Pages) == 0 {
		return agentcapability.ExternalFailureResult(c.spec, fmt.Sprintf("fetch %d web urls", len(urls)), invokeErr), invokeErr
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
			Summary: strings.Join(urls, ", "),
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    output.Summary,
			Degraded:   output.Degraded,
			ErrorClass: classificationForFetch(output),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				FetchErrorClass: agentcapability.StringPtr(classificationForFetch(output)),
				FetchResults:    toFetchRefs(output),
				Notes:           agentcapability.AppendNonEmpty(nil, note),
			},
		},
		Status:     status,
		ErrorClass: classificationForFetch(output),
	}
	if invokeErr != nil {
		return result, nil
	}
	return result, nil
}

func toFetchRefs(output Output) []agentstate.FetchResultRef {
	if len(output.Pages) == 0 {
		return nil
	}
	refs := make([]agentstate.FetchResultRef, 0, len(output.Pages))
	for idx, page := range output.Pages {
		summary := summarizeFetchText(page.Text)
		refs = append(refs, agentstate.FetchResultRef{
			ID:             fmt.Sprintf("fetch_%d", idx+1),
			URL:            page.URL,
			Title:          page.URL,
			Summary:        summary,
			Text:           page.Text,
			ContentRef:     page.URL,
			OriginalLength: page.OriginalLength,
			WasTruncated:   page.WasTruncated,
			Degraded:       strings.TrimSpace(page.ErrorMessage) != "",
			ErrorReason:    page.ErrorMessage,
		})
	}
	return refs
}

func summarizeFetchText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 240 {
		return text
	}
	return strings.TrimSpace(text[:240])
}

func classificationForFetch(output Output) string {
	if !output.Degraded {
		return ""
	}
	if agentcapability.MatchesPermissionError(fetchClassificationMessages(output)...) {
		return agentcapability.ErrorClassPermission
	}
	if agentcapability.MatchesDependencyError(fetchClassificationMessages(output)...) {
		return agentcapability.ErrorClassDependency
	}
	return agentcapability.ErrorClassExternal
}

func applyCapabilityOptions(spec *agentcapability.Spec, options ...agentcapability.Option) {
	agentcapability.ApplyOptions(spec, options...)
}

func fetchClassificationMessages(output Output) []string {
	values := []string{output.DegradeReason, output.ErrorMessage, output.Summary}
	for _, page := range output.Pages {
		values = append(values, page.ErrorMessage)
	}
	return values
}
