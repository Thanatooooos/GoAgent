package tool

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

func TestAgentLoopReportsContextBudgetStats(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{Name: "context_budget_probe", ReadOnly: true},
		result: Result{
			Name:    "context_budget_probe",
			Status:  CallStatusSuccess,
			Summary: strings.Repeat("evidence ", 20),
		},
	})
	planner := &plannerStub{results: []PlanResult{{
		Calls: []Call{{Name: "context_budget_probe"}},
	}}}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetMaxIterations(1)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:           "probe",
		ContextTokenBudget: 30,
		ContextEstimator:   tokenbudget.RuneEstimator{},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ContextBudget.Truncated || result.ContextBudget.TokensBefore <= result.ContextBudget.TokensAfter {
		t.Fatalf("context budget stats = %+v", result.ContextBudget)
	}
}
