package planexecute

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newExecuteStepNode(registry *agentcapability.Registry, resolver agentresolve.Resolver) (agentkernel.Node, error) {
	if registry == nil {
		return nil, fmt.Errorf("plan-execute execute-step requires capability registry")
	}
	return agentkernel.NewNodeFunc("execute_step", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		plan := copyPlan(session.Snapshot.Plan)
		if plan.CurrentStepIndex < 0 || plan.CurrentStepIndex >= len(plan.Steps) {
			return agentruntime.NodeResult{}, fmt.Errorf("plan-execute execute-step requires an active step")
		}
		step := plan.Steps[plan.CurrentStepIndex]
		handle, spec, input, err := resolveInvocation(step, registry, resolver)
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		startedAt := time.Now()
		result, err := handle.Invoke(ctx, agentcapability.InvocationRequest{
			SessionID: session.SessionID,
			Snapshot:  session.Snapshot,
			Input:     input,
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}

		step.Status = agentstate.PlanStepStatusRunning
		step.LastSummary = firstNonEmpty(result.Observation.Summary, result.Action.Summary, step.Title)
		step.LastErrorClass = result.ErrorClass
		resultState := resultSummary(step, result.Status, result.ErrorClass, result.Output, result.Observation.Summary)
		resultState.ProducedEvidence = stepHasEvidence(spec, step, result)
		plan.Steps[plan.CurrentStepIndex] = step
		plan.LastStepResult = resultState

		events := []agentstate.RuntimeEvent{
			agentstate.NewRuntimeEventAt(startedAt, session.SessionID, "execute_step", agentstate.EventTypeCapabilityStart, firstNonEmpty(result.Action.Summary, step.Title)),
		}
		eventType := agentstate.EventTypeCapabilityResult
		if result.Status == agentcapability.StatusSkipped {
			eventType = agentstate.EventTypeCapabilitySkipped
		}
		events = append(events, agentstate.NewRuntimeEvent(session.SessionID, "execute_step", eventType, firstNonEmpty(result.Observation.Summary, step.LastSummary)))

		delta := result.Delta
		delta.Plan = &agentstate.PlanDelta{
			Replace: &plan,
		}
		delta.Execution = executionNodeDelta("execute_step")

		return agentruntime.NodeResult{
			Events: events,
			Delta:  delta,
		}, nil
	})
}

func resolveInvocation(step agentstate.PlanStep, registry *agentcapability.Registry, resolver agentresolve.Resolver) (agentcapability.Handle, agentcapability.Spec, any, error) {
	if resolver != nil {
		resolved, err := resolver.Resolve(selectionFromStep(step))
		if err == nil {
			return resolved.Handle, resolved.Spec, resolved.Input, nil
		}
		if strings.TrimSpace(step.CapabilityName) == "" {
			return nil, agentcapability.Spec{}, nil, err
		}
	}
	handle, err := registry.Handle(step.CapabilityName)
	if err != nil {
		return nil, agentcapability.Spec{}, nil, err
	}
	spec, ok := registry.Spec(step.CapabilityName)
	if !ok {
		return nil, agentcapability.Spec{}, nil, fmt.Errorf("capability %q spec is not registered", step.CapabilityName)
	}
	input := legacyInvocationInputForStep(step)
	if normalizer, ok := handle.(agentcapability.InputNormalizer); ok {
		input, err = normalizer.NormalizeInput(input)
		if err != nil {
			return nil, agentcapability.Spec{}, nil, err
		}
	}
	return handle, spec, input, nil
}

func legacyInvocationInputForStep(step agentstate.PlanStep) any {
	if len(step.CapabilityInput) > 0 {
		return cloneInputMap(step.CapabilityInput)
	}
	switch strings.TrimSpace(step.CapabilityName) {
	case agentcapability.NameWebSearch:
		return map[string]any{"query": step.Query}
	case agentcapability.NameWebFetch:
		return map[string]any{"urls": append([]string(nil), step.URLs...)}
	default:
		return nil
	}
}
