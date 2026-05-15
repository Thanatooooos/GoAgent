package tool

import (
	"context"
	"strings"
	"testing"
)

func TestDocumentIngestionDiagnoseBehaviorNextObserveAndGuidance(t *testing.T) {
	behavior := DocumentIngestionDiagnoseBehavior()
	result := Result{
		Name:   "document_ingestion_diagnose",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"conclusion":     "document ingestion failed, but no failed node was captured",
			"confidence":     "medium",
			"facts":          []string{"document.status=failed"},
			"nextActions":    []string{"check task detail"},
			"latestTaskId":   "task-1",
			"latestLogError": "task node details missing",
		},
	}

	decision := behavior.Next(result, WorkflowInput{})
	if decision.Done || len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "ingestion_task_query" {
		t.Fatalf("expected ingestion_task_query continuation, got %+v", decision)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || observation.Done {
		t.Fatalf("expected observe hook to continue, got handled=%v observation=%+v", handled, observation)
	}

	notes := behavior.BuildGuidance(result, GuidanceInput{AllResults: []Result{
		result,
		{
			Name: "ingestion_task_node_query",
			Data: map[string]any{
				"taskId":       "task-1",
				"nodeId":       "indexer",
				"status":       "failed",
				"errorMessage": "connection refused",
			},
		},
	}})
	if len(notes) != 1 || !strings.Contains(notes[0].Text, "connection refused") {
		t.Fatalf("expected diagnosis guidance with deeper evidence, got %+v", notes)
	}
}

func TestTraceAndThinkBehaviors(t *testing.T) {
	traceBehavior := TraceNodeQueryBehavior()
	traceResult := Result{
		Name:    "trace_node_query",
		Status:  CallStatusSuccess,
		Summary: "trace trace-1 status=failed conversationId=conv-1 nodes=2",
		Data: map[string]any{
			"traceId":      "trace-1",
			"status":       "failed",
			"errorMessage": "rewrite node failed",
			"nodeCount":    2,
			"nodes": []map[string]any{
				{"nodeId": "rewrite", "nodeType": "llm", "nodeName": "rewrite", "status": "failed"},
				{"nodeId": "retrieve", "nodeType": "retriever", "nodeName": "retrieve", "status": "pending"},
			},
		},
	}

	decision := traceBehavior.Next(traceResult, WorkflowInput{})
	if !decision.Done || !decision.Terminal {
		t.Fatalf("expected trace node query to be terminal, got %+v", decision)
	}
	observation, handled := traceBehavior.Observe(traceResult, ObserveInput{})
	if !handled || !observation.Done {
		t.Fatalf("expected trace observe hook to complete, got handled=%v observation=%+v", handled, observation)
	}
	rendered := traceBehavior.RenderContext(traceResult)
	if !strings.Contains(rendered, "Trace nodes") || !strings.Contains(rendered, "rewrite") {
		t.Fatalf("expected rendered trace nodes, got %q", rendered)
	}

	thinkBehavior := ThinkBehavior()
	thinkDecision := thinkBehavior.Next(Result{Name: "think", Summary: "plan next step"}, WorkflowInput{})
	if !thinkDecision.Done || !thinkDecision.Terminal {
		t.Fatalf("expected think behavior to be terminal, got %+v", thinkDecision)
	}
}

func TestGraphBehaviorsProvideGuidanceAndCompletion(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "graph diagnose",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed 3 hops",
			Data: map[string]any{
				"documentId":     "doc-1",
				"latestTaskId":   "task-1",
				"latestNodeId":   "indexer",
				"conclusion":     "document ingestion failed at node indexer",
				"confidence":     "high",
				"diagnosisDepth": "node_level",
				"chainLength":    3,
			},
		},
	}, ToolSpec{
		Capability:          CapabilityDiagnosis,
		EvidenceSources:     []string{EvidenceSourceSystemRecords},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "graph",
	}, DocumentRootCauseDiagnosisBehavior()).Module())

	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{{Name: "document_root_cause_diagnosis", Arguments: map[string]any{"documentId": "doc-1"}}},
		}},
	}

	loop := NewAgentLoop(NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "doc-1 为什么失败了",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) != 1 || !result.Rounds[0].Done {
		t.Fatalf("expected single completed round, got %+v", result.Rounds)
	}
	if result.Rounds[0].Confidence < 0.9 {
		t.Fatalf("expected graph behavior confidence >= 0.9, got %v", result.Rounds[0].Confidence)
	}
	if !strings.Contains(result.Context, "Conclusion: document ingestion failed at node indexer") {
		t.Fatalf("expected graph render context, got %q", result.Context)
	}
	if !strings.Contains(result.AnswerGuidance, "document ingestion failed at node indexer") {
		t.Fatalf("expected module-first graph guidance, got %q", result.AnswerGuidance)
	}
}

func TestDocumentDiagnoseWithSearchBehaviorGuidance(t *testing.T) {
	registry := NewRegistry()
	result := Result{
		Name:    "document_diagnose_with_search",
		Status:  CallStatusSuccess,
		Summary: "diagnose+search chain: diagnosis=success -> web_search=success(\"connection refused troubleshooting\")",
		Data: map[string]any{
			"documentId":        "doc-1",
			"conclusion":        "document ingestion failed at node indexer: connection refused",
			"diagnosisDepth":    "node_level",
			"searchQuery":       "connection refused troubleshooting",
			"searchResultCount": 4,
		},
	}
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "document_diagnose_with_search",
			Description: "diagnose with search",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: result,
	}, ToolSpec{
		Capability:          CapabilitySearch,
		EvidenceSources:     []string{EvidenceSourceSystemRecords, EvidenceSourceExternalWeb},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "graph",
	}, DocumentDiagnoseWithSearchBehavior()).Module())

	guidance := BuildAnswerGuidanceWithRegistry(registry, []Result{result})
	if !strings.Contains(guidance, "connection refused troubleshooting") {
		t.Fatalf("expected search query in diagnose+search guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "不要编造具体修复方案") {
		t.Fatalf("expected warning about missing fetched page evidence, got %q", guidance)
	}
}
