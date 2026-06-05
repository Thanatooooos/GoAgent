package planexecute

import (
	"context"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newBuildPlanNode(
	registry *agentcapability.Registry,
	searchSpec agentcapability.Spec,
	fetchSpec agentcapability.Spec,
	catalogBuilder agentcatalog.Builder,
	selector selectcapability.Selector,
	resolver agentresolve.Resolver,
) (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("build_plan", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		plan := buildPlanFromSpecs(session, searchSpec, fetchSpec)
		reasoning := "built linear search-then-fetch plan"
		notes := []string{"built explicit plan-execute workflow"}
		if selectedPlan, selectedReasoning, selectedNotes, ok := buildPlanWithSelector(ctx, session, registry, catalogBuilder, selector, resolver); ok {
			plan = selectedPlan
			reasoning = selectedReasoning
			notes = selectedNotes
		}
		if session != nil && session.Snapshot.Plan.ReplanCount > 0 {
			plan.ReplanCount = session.Snapshot.Plan.ReplanCount
		}
		contextDelta := &agentstate.ContextDelta{
			Notes: notes,
		}
		if len(plan.Steps) > 0 && strings.TrimSpace(plan.Steps[0].Query) != "" {
			contextDelta.SearchQuery = &plan.Steps[0].Query
		}
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Plan: &agentstate.PlanDelta{
					Replace: &plan,
				},
				Context:   contextDelta,
				Execution: executionNodeDelta("build_plan"),
			},
			Decision: &agentruntime.DecisionArtifact{
				Kind:       "plan",
				Target:     reasonPlanBuilt,
				Confidence: 0.72,
				Reasoning:  reasoning,
			},
		}, nil
	})
}

func buildPlanWithSelector(
	ctx context.Context,
	session *agentruntime.RuntimeSession,
	registry *agentcapability.Registry,
	catalogBuilder agentcatalog.Builder,
	selector selectcapability.Selector,
	resolver agentresolve.Resolver,
) (agentstate.PlanState, string, []string, bool) {
	if registry == nil || catalogBuilder == nil || selector == nil || resolver == nil {
		return agentstate.PlanState{}, "", nil, false
	}
	var contextNotes []string
	if session != nil {
		contextNotes = append([]string(nil), session.Snapshot.Context.Notes...)
	}
	cards, err := catalogBuilder.Build(registry)
	if err != nil || len(cards) == 0 {
		return agentstate.PlanState{}, "", nil, false
	}
	selectionOutput, err := selector.Select(ctx, selectcapability.SelectionInput{
		UserRequest:   normalizeQuery(session),
		ContextNotes:  contextNotes,
		Capabilities:  cards,
		MaxSelections: 1,
	})
	if err != nil || len(selectionOutput.Selections) == 0 {
		return agentstate.PlanState{}, "", nil, false
	}
	matched, err := resolver.Match(selectionOutput.Selections[0])
	if err != nil {
		return agentstate.PlanState{}, "", nil, false
	}
	plan := buildPlanFromSelection(session, matched, selectionOutput.Selections[0])
	return plan,
		"built selector-driven plan around " + matched.Name,
		[]string{"built selector-driven plan around capability " + matched.Name},
		true
}
