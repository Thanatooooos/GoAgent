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
	return decodeCapabilityInput(raw)
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := decodeCapabilityInput(req.Input)
	if err != nil {
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: "fetch invocation rejected",
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
				Summary: "skip web fetch",
			},
			Observation: agentcapability.ObservationRecord{
				Summary:  "web fetch skipped: no fetchable urls",
				Degraded: false,
			},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					Notes: appendNonEmpty(nil, "web fetch skipped: no fetchable urls"),
				},
			},
			Status: agentcapability.StatusSkipped,
		}, nil
	}

	urls := normalizeURLs(input.URLs)
	output, invokeErr := c.invoker.Fetch(ctx, urls)
	if invokeErr != nil && len(output.Pages) == 0 {
		return agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Name:    c.spec.Name,
				Summary: fmt.Sprintf("fetch %d web urls", len(urls)),
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
			Summary: strings.Join(urls, ", "),
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    output.Summary,
			Degraded:   output.Degraded,
			ErrorClass: classificationForFetch(output),
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				FetchErrorClass: stringPtr(classificationForFetch(output)),
				FetchResults:    toFetchRefs(output),
				Notes:           appendNonEmpty(nil, note),
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

func decodeCapabilityInput(raw any) (CapabilityInput, error) {
	input, err := agentcapability.DecodeStructuredInput[CapabilityInput](raw, "fetch capability input is required")
	if err != nil {
		return CapabilityInput{}, fmt.Errorf("fetch capability input has unexpected type %T: %w", raw, err)
	}
	return input, nil
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
	if matchesPermissionError(fetchClassificationMessages(output)...) {
		return agentcapability.ErrorClassPermission
	}
	if matchesDependencyError(fetchClassificationMessages(output)...) {
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

func fetchClassificationMessages(output Output) []string {
	values := []string{output.DegradeReason, output.ErrorMessage, output.Summary}
	for _, page := range output.Pages {
		values = append(values, page.ErrorMessage)
	}
	return values
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
