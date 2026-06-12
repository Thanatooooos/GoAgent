package think

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// CapabilityInput is the typed invocation input for the think capability.
type CapabilityInput struct {
	Thought string `json:"thought"`
}

// CapabilityOutput echoes the reasoning thought for observability.
type CapabilityOutput struct {
	Thought string `json:"thought"`
}

type capabilityAdapter struct {
	spec agentcapability.Spec
}

// NewCapability builds the meta think capability.
func NewCapability(options ...agentcapability.Option) (agentcapability.Handle, error) {
	spec := agentcapability.Spec{
		Name:             agentcapability.NameThink,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyMeta,
		Roles:            []string{agentcapability.RoleThink},
		Description:      "Records an explicit reasoning step for observability without calling external services.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: true,
		SupportsResume:   false,
		ProducesEvidence: false,
		Idempotency:      agentcapability.IdempotencyIdempotent,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "thought",
				Requirement: agentcapability.PreconditionRequirementNonEmpty,
				Description: "Think requires a non-empty thought.",
			},
		},
	}
	agentcapability.ApplyOptions(&spec, options...)
	return capabilityAdapter{spec: spec}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "think capability input is required", "think capability input")
}

func (c capabilityAdapter) Invoke(_ context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "think capability input is required", "think capability input")
	if err != nil {
		return agentcapability.ValidationFailureResult(c.spec, "think invocation rejected", err), err
	}

	thought := strings.TrimSpace(input.Thought)
	output := CapabilityOutput{Thought: thought}
	summary := thought
	if len([]rune(summary)) > 120 {
		summary = string([]rune(summary)[:120]) + "..."
	}

	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("think: %s", summary),
		},
		Observation: agentcapability.ObservationRecord{
			Summary: summary,
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: agentcapability.AppendNonEmpty(nil, thought),
			},
		},
		Status: agentcapability.StatusSucceeded,
	}, nil
}
