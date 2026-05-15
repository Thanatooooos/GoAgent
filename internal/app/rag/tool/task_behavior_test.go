package tool

import (
	"context"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

func TestTaskListBehaviorObserveNextAndRender(t *testing.T) {
	behavior := TaskListBehavior()
	emptyResult := Result{
		Name:   "task_list",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"total": 0,
			"items": []map[string]any{},
		},
	}

	decision := behavior.Next(emptyResult, WorkflowInput{
		Question:       "golang generics 是什么",
		RetrieveResult: ragretrieve.Result{},
	})
	if !decision.Done && (len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "external_evidence_workflow") {
		t.Fatalf("expected external_evidence_workflow hint, got %+v", decision)
	}

	observation, handled := behavior.Observe(emptyResult, ObserveInput{
		Question:       "golang generics 是什么",
		RetrieveResult: ragretrieve.Result{},
	})
	if !handled {
		t.Fatal("expected task_list observe hook to handle result")
	}
	_ = observation

	listResult := Result{
		Name:    "task_list",
		Status:  CallStatusSuccess,
		Summary: "found 2 tasks",
		Data: map[string]any{
			"total": 2,
			"items": []map[string]any{
				{"taskId": "task-1", "pipelineId": "pipe-1", "status": "running", "sourceFileName": "a.pdf"},
				{"taskId": "task-2", "pipelineId": "pipe-1", "status": "failed", "sourceFileName": "b.pdf"},
			},
		},
	}
	rendered := behavior.RenderContext(listResult)
	if !strings.Contains(rendered, "Task list") || !strings.Contains(rendered, "task-1") {
		t.Fatalf("expected rendered task list detail, got %q", rendered)
	}
}

func TestIngestionTaskQueryBehaviorNextAndObserve(t *testing.T) {
	behavior := IngestionTaskQueryBehavior()
	result := Result{
		Name:   "ingestion_task_query",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"taskId": "task-1",
			"status": "running",
			"taskNodeSummary": []map[string]any{
				{"nodeId": "indexer", "status": "running"},
			},
		},
	}

	decision := behavior.Next(result, WorkflowInput{})
	if decision.Done || len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "ingestion_task_node_query" {
		t.Fatalf("expected node query hint, got %+v", decision)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || observation.Done {
		t.Fatalf("expected observe hook to continue, got handled=%v observation=%+v", handled, observation)
	}
}

func TestIngestionTaskNodeQueryBehaviorObserveAndRender(t *testing.T) {
	behavior := IngestionTaskNodeQueryBehavior()
	result := Result{
		Name:   "ingestion_task_node_query",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"taskId":       "task-1",
			"nodeId":       "indexer",
			"status":       "failed",
			"errorMessage": "connection refused",
		},
	}

	decision := behavior.Next(result, WorkflowInput{})
	if !decision.Done || !decision.Terminal {
		t.Fatalf("expected terminal node query decision, got %+v", decision)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || !observation.Done {
		t.Fatalf("expected observe hook to complete, got handled=%v observation=%+v", handled, observation)
	}

	rendered := behavior.RenderContext(result)
	if !strings.Contains(rendered, "Task node detail") || !strings.Contains(rendered, "indexer") {
		t.Fatalf("expected rendered node detail, got %q", rendered)
	}
}

func TestAgentLoopTaskModulesUseBehaviorDrivenContinuation(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "ingestion_task_query",
			Description: "query task",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "taskId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "ingestion_task_query",
			Status:  CallStatusSuccess,
			Summary: "task task-1 is still running at node indexer",
			Data: map[string]any{
				"taskId": "task-1",
				"status": "running",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "indexer", "status": "running"},
				},
			},
		},
	}, ToolSpec{
		Capability:          CapabilityDiagnosis,
		EvidenceSources:     []string{EvidenceSourceSystemRecords},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "system",
	}, IngestionTaskQueryBehavior()).Module())
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "ingestion_task_node_query",
			Description: "query task node",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "taskId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "ingestion_task_node_query",
			Status:  CallStatusSuccess,
			Summary: "task node detail ready",
			Data: map[string]any{
				"taskId": "task-1",
				"nodeId": "indexer",
				"status": "running",
			},
		},
	}, ToolSpec{
		Capability:          CapabilityDiagnosis,
		EvidenceSources:     []string{EvidenceSourceSystemRecords},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "system",
	}, IngestionTaskNodeQueryBehavior()).Module())

	planner := &plannerStub{
		results: []PlanResult{
			{
				Calls: []Call{{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": "task-1"}}},
			},
		},
	}

	loop := NewAgentLoop(NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "task-1 当前运行到哪里了",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected ingestion_task_query then ingestion_task_node_query, got %+v", result.Calls)
	}
	if result.Calls[0].Name != "ingestion_task_query" || result.Calls[1].Name != "ingestion_task_node_query" {
		t.Fatalf("unexpected call order: %+v", result.Calls)
	}
	if len(result.Rounds) < 1 || result.Rounds[0].State.Phase != "verification" {
		t.Fatalf("expected task behavior driven continuation, got %+v", result.Rounds)
	}
}
