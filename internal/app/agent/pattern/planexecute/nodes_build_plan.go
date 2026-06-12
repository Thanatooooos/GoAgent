package planexecute

import (
	"context"
	"fmt"
	"strings"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newBuildPlanNode(
	synthesizer PlanSynthesizer,
) (agentkernel.Node, error) {
	if synthesizer == nil {
		return nil, fmt.Errorf("build_plan node requires plan synthesizer")
	}
	return agentkernel.NewNodeFunc("build_plan", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		synthesized, err := synthesizer.Synthesize(ctx, PlanSynthesisInput{Session: session})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		plan := synthesized.Plan
		reasoning := firstNonEmpty(synthesized.Reasoning, "built explicit plan-execute plan")
		notes := append([]string(nil), synthesized.Notes...)
		if session != nil && session.Snapshot.Plan.ReplanCount > 0 {
			plan.ReplanCount = session.Snapshot.Plan.ReplanCount
		}
		logPlanBuilt(session, plan, reasoning)
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
