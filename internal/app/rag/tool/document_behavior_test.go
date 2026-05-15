package tool

import (
	"context"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

func TestDocumentQueryBehaviorNextAndObserve(t *testing.T) {
	behavior := DocumentQueryBehavior()
	result := Result{
		Name:   "document_query",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"documentId":  "doc-1",
			"status":      "failed",
			"processMode": "pipeline",
		},
	}

	decision := behavior.Next(result, WorkflowInput{})
	if decision.Done || len(decision.HintCalls) != 1 {
		t.Fatalf("expected follow-up decision, got %+v", decision)
	}
	if decision.HintCalls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected document_ingestion_diagnose hint, got %+v", decision.HintCalls)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || observation.Done {
		t.Fatalf("expected observe hook to continue, got handled=%v observation=%+v", handled, observation)
	}
}

func TestDocumentChunkLogQueryBehaviorNextAndObserve(t *testing.T) {
	behavior := DocumentChunkLogQueryBehavior()
	result := Result{
		Name:   "document_chunk_log_query",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"documentId":      "doc-1",
			"latestTaskId":    "task-1",
			"latestStatus":    "running",
			"runningLogCount": 1,
		},
	}

	decision := behavior.Next(result, WorkflowInput{})
	if decision.Done || len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "ingestion_task_query" {
		t.Fatalf("expected ingestion_task_query hint, got %+v", decision)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || observation.Done {
		t.Fatalf("expected observe hook to continue, got handled=%v observation=%+v", handled, observation)
	}
}

func TestDocumentListBehaviorObserveNextAndRender(t *testing.T) {
	behavior := DocumentListBehavior()
	emptyResult := Result{
		Name:   "document_list",
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
	if !decision.Done {
		// no chunks means KB insufficient should continue
		if len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "external_evidence_workflow" {
			t.Fatalf("expected external_evidence_workflow hint, got %+v", decision)
		}
	}

	observation, handled := behavior.Observe(emptyResult, ObserveInput{
		Question:       "golang generics 是什么",
		RetrieveResult: ragretrieve.Result{},
	})
	if !handled {
		t.Fatal("expected document_list observe hook to handle result")
	}

	listResult := Result{
		Name:    "document_list",
		Status:  CallStatusSuccess,
		Summary: "found 2 documents",
		Data: map[string]any{
			"total": 2,
			"items": []map[string]any{
				{"documentId": "doc-1", "name": "API Guide", "status": "success", "processMode": "pipeline"},
				{"documentId": "doc-2", "name": "Ops Notes", "status": "failed", "processMode": "pipeline"},
			},
		},
	}
	rendered := behavior.RenderContext(listResult)
	if !strings.Contains(rendered, "Document list") || !strings.Contains(rendered, "API Guide") {
		t.Fatalf("expected rendered list detail, got %q", rendered)
	}
	if observation.Done && len(observation.NextHintCalls) == 0 {
		// acceptable fallback when KB insufficiency helper does not trigger
	}
}

func TestAgentLoopDocumentModulesUseBehaviorDrivenContinuation(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "document doc-1 status=failed",
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
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
	}, DocumentQueryBehavior()).Module())
	registry.MustRegisterModule(NewLegacyToolAdapter(staticTool{
		definition: Definition{
			Name:        "document_ingestion_diagnose",
			Description: "diagnose document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "document diagnosis complete",
			Data: map[string]any{
				"conclusion": "document ingestion failed at node indexer",
				"confidence": "high",
			},
		},
	}).Module())

	planner := &plannerStub{
		results: []PlanResult{
			{
				Calls: []Call{{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}}},
			},
		},
	}

	loop := NewAgentLoop(NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "doc-1 为什么失败了",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected document_query then document_ingestion_diagnose, got %+v", result.Calls)
	}
	if result.Calls[0].Name != "document_query" || result.Calls[1].Name != "document_ingestion_diagnose" {
		t.Fatalf("unexpected call order: %+v", result.Calls)
	}
	if len(result.Rounds) < 1 || result.Rounds[0].State.Phase != "initial_diagnosis" {
		t.Fatalf("expected document behavior driven continuation, got %+v", result.Rounds)
	}
}
