package planexecute

import (
	"context"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

// PlanSynthesisInput carries the runtime session context a synthesizer can use
// to materialize or re-materialize a plan.
type PlanSynthesisInput struct {
	Session *agentruntime.RuntimeSession
}

// PlanSynthesisResult is the structured output of plan creation before the
// build_plan node projects it into runtime state.
type PlanSynthesisResult struct {
	Plan      agentstate.PlanState
	Reasoning string
	Notes     []string
}

// PlanSynthesizer owns how a plan is created for the plan_execute pattern.
// Execution nodes consume the synthesized plan but do not decide how it was
// produced.
type PlanSynthesizer interface {
	Synthesize(ctx context.Context, input PlanSynthesisInput) (PlanSynthesisResult, error)
}
