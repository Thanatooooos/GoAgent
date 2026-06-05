package planner

import (
	"context"

	agentruntime "local/rag-project/internal/app/agent/runtime"
)

type Planner interface {
	Plan(ctx context.Context, input PlanInput) (PlanResult, error)
}

type PlanInput struct {
	Session          *agentruntime.RuntimeSession
	BaselineDecision string
	BaselineReason   string
}

type PlanResult struct {
	Decision          string
	Reason            string
	Confidence        float64
	NextQuery         string
	PreferredURLs     []string
	AvoidURLs         []string
	AnswerEvidenceIDs []string
	Notes             []string
}
