package tool

import (
	"context"
	"strings"
	"testing"

	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
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

	guidance := ragruntime.BuildAnswerGuidanceWithRegistry(testRegistry, []Result{
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
	})
	if !strings.Contains(guidance, "connection refused") {
		t.Fatalf("expected diagnosis guidance with deeper evidence, got %q", guidance)
	}
}

func TestTraceAndThinkBehaviors(t *testing.T) {
	traceBehavior := TraceNodeQueryBehavior()
	traceResult := Result{
		Name:    "trace_node_query",
		Status:  CallStatusSuccess,
		Summary: "trace trace-1 status=failed conversationId=conv-1 nodes=3",
		Data: map[string]any{
			"traceId":      "trace-1",
			"status":       "failed",
			"errorMessage": "rewrite node failed",
			"nodeCount":    3,
			"nodes": []map[string]any{
				{"nodeId": "rewrite", "nodeType": "llm", "nodeName": "rewrite", "status": "failed"},
				{
					"nodeId":   "long_term_memory",
					"nodeType": "memory",
					"nodeName": "long_term_memory",
					"status":   "success",
					"summary":  "selected 2/3 memories (rules=1 facts=1/2), cache rule=request fact=redis embedding=request, contributions hybrid=1, vector_only=1, sources keyword=1, vector=2, reason=fact_cache_miss",
					"memoryRecall": map[string]any{
						"ruleCount":           1,
						"factCandidateCount":  2,
						"factSelectedCount":   1,
						"candidateCount":      3,
						"selectedCount":       2,
						"ruleCacheLayer":      "request",
						"factCacheLayer":      "redis",
						"embeddingCacheLayer": "request",
						"recomputeReason":     "fact_cache_miss",
						"sourceCounts":        map[string]any{"keyword": 1, "vector": 2},
						"contributionCounts":  map[string]any{"hybrid": 1, "vector_only": 1},
						"memoryIds":           []any{"mem-1", "mem-2"},
						"summary":             "selected 2/3 memories (rules=1 facts=1/2), cache rule=request fact=redis embedding=request, contributions hybrid=1, vector_only=1, sources keyword=1, vector=2, reason=fact_cache_miss",
					},
				},
				{
					"nodeId":   "session_recall",
					"nodeType": "memory",
					"nodeName": "session_recall",
					"status":   "success",
					"summary":  "recalled 1/4 excerpts, topScore=0.9100, cache session=conversation embedding=request, perMessageSkips=1, truncatedBy=max_prompt_tokens, reason=conversation_cache_miss",
					"sessionRecall": map[string]any{
						"candidateCount":         4,
						"excerptCount":           1,
						"topScore":               0.91,
						"cacheLayer":             "conversation",
						"embeddingCacheLayer":    "request",
						"recomputeReason":        "conversation_cache_miss",
						"skippedPerMessageLimit": 1,
						"truncatedBy":            "max_prompt_tokens",
						"selectedMessageIds":     []any{"msg-1"},
						"selectedChunkIds":       []any{"chunk-1"},
						"summary":                "recalled 1/4 excerpts, topScore=0.9100, cache session=conversation embedding=request, perMessageSkips=1, truncatedBy=max_prompt_tokens, reason=conversation_cache_miss",
					},
				},
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
	if !strings.Contains(rendered, "memory recall: selected 2/3 memories") || !strings.Contains(rendered, "vector_only=1") {
		t.Fatalf("expected rendered memory recall summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "session recall: recalled 1/4 excerpts") || !strings.Contains(rendered, "truncatedBy=max_prompt_tokens") {
		t.Fatalf("expected rendered session recall summary, got %q", rendered)
	}

	traceDiagnoseBehavior := TraceRetrievalDiagnoseBehavior()
	traceDiagnoseResult := Result{
		Name:   "trace_retrieval_diagnose",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"conclusion":   "retrieve node returned no evidence because the rewritten query was too broad",
			"confidence":   "high",
			"facts":        []string{"trace trace-1 ended with empty retrieve result"},
			"nextActions":  []string{"narrow the query before retrying retrieval"},
			"latestTaskId": "task-1",
			"latestNodeId": "retrieve",
		},
	}

	decoded, err := traceDiagnoseBehavior.Decode(traceDiagnoseResult)
	if err != nil {
		t.Fatalf("decode trace retrieval diagnose: %v", err)
	}
	diagnosisView, ok := decoded.(TraceRetrievalDiagnoseResultView)
	if !ok {
		t.Fatalf("expected TraceRetrievalDiagnoseResultView, got %T", decoded)
	}
	if diagnosisView.LatestNodeID != "retrieve" {
		t.Fatalf("unexpected trace diagnose node id: %+v", diagnosisView)
	}
	observation, handled = traceDiagnoseBehavior.Observe(traceDiagnoseResult, ObserveInput{})
	if !handled || !observation.Done {
		t.Fatalf("expected trace diagnose observe hook to complete, got handled=%v observation=%+v", handled, observation)
	}
	rendered = traceDiagnoseBehavior.RenderContext(traceDiagnoseResult)
	if !strings.Contains(rendered, "Conclusion: retrieve node returned no evidence") {
		t.Fatalf("expected rendered diagnose context, got %q", rendered)
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

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "doc-1 涓轰粈涔堝け璐ヤ簡",
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

	guidance := ragruntime.BuildAnswerGuidanceWithRegistry(registry, []Result{result})
	if !strings.Contains(guidance, "connection refused troubleshooting") {
		t.Fatalf("expected search query in diagnose+search guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "Do not invent a concrete fix") {
		t.Fatalf("expected warning about missing fetched page evidence, got %q", guidance)
	}

	guidance = ragruntime.BuildAnswerGuidanceWithRegistry(registry, []Result{
		result,
		{
			Name:   "external_evidence_workflow",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"selectedUrls": []string{"https://example.com/vector-store"},
				"pages": []map[string]any{
					{
						"url":  "https://example.com/vector-store",
						"text": "Restart the vector store after confirming the network policy.",
					},
				},
			},
		},
	})
	if !strings.Contains(guidance, "rely only on fetched page content") {
		t.Fatalf("expected fetched-page guidance when external evidence workflow contains readable pages, got %q", guidance)
	}
}
