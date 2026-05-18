package tool_test

import (
	"context"
	"strings"
	"testing"

	. "local/rag-project/internal/app/rag/tool"
	raggraph "local/rag-project/internal/app/rag/tool/invokers/graph"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

// countingTool is a tool stub that counts invocations.
type countingTool struct {
	name   string
	invoke func(ctx context.Context, call Call) (Result, error)
}

func (c *countingTool) Definition() Definition {
	return Definition{Name: c.name, Parameters: []ParameterDefinition{}}
}

func (c *countingTool) Invoke(ctx context.Context, call Call) (Result, error) {
	return c.invoke(ctx, call)
}

type staticTool struct {
	definition Definition
	result     Result
	err        error
}

func (s staticTool) Definition() Definition {
	return s.definition
}

func (s staticTool) Invoke(ctx context.Context, call Call) (Result, error) {
	return s.result, s.err
}

// setupGraphTestRegistry creates a registry with graph tool + all individual tools
// the graph tool needs internally, using stubs that return realistic data.
func setupGraphTestRegistry() *Registry {
	r := NewRegistry()

	// Graph tool: internally chains diagnose → task_query → node_query
	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "Deterministic 3-hop diagnosis chain.",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed 3 hops: diagnose → task_query → node_query",
			Data: map[string]any{
				"documentId":   "doc_fail_01",
				"latestTaskId": "task_fail_01",
				"latestNodeId": "indexer",
				"conclusion":   "document ingestion failed at node indexer, error: connection refused",
				"confidence":   "high",
				"chainLength":  3,
			},
		},
	})

	// Individual diagnose tool (still needed for direct calls)
	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_ingestion_diagnose",
			Description: "diagnose document ingestion",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "diagnose: failed at node indexer",
			Data: map[string]any{
				"documentId":      "doc_fail_01",
				"latestTaskId":    "task_fail_01",
				"latestNodeId":    "indexer",
				"latestNodeError": "connection refused",
				"conclusion":      "failed at node indexer",
				"confidence":      "high",
			},
		},
	})

	// Task query tool (graph internally calls this)
	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "ingestion_task_query",
			Description: "query ingestion task",
			ReadOnly:    true,
			Parameters: []ParameterDefinition{
				{Name: "taskId", Type: ParamTypeString, Required: true},
				{Name: "includeNodes", Type: ParamTypeBoolean, Required: false},
			},
		},
		result: Result{
			Name:    "ingestion_task_query",
			Status:  CallStatusSuccess,
			Summary: "task query: found failed node indexer",
			Data: map[string]any{
				"taskId": "task_fail_01",
				"status": "failed",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "fetcher", "status": "success"},
					{"nodeId": "parser", "status": "success"},
					{"nodeId": "indexer", "status": "failed", "errorMessage": "connection refused"},
				},
			},
		},
	})

	// Node query tool (graph internally calls this)
	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "ingestion_task_node_query",
			Description: "query task node",
			ReadOnly:    true,
			Parameters: []ParameterDefinition{
				{Name: "taskId", Type: ParamTypeString, Required: true},
				{Name: "nodeId", Type: ParamTypeString, Required: false},
			},
		},
		result: Result{
			Name:    "ingestion_task_node_query",
			Status:  CallStatusSuccess,
			Summary: "node query: indexer failed with connection refused",
			Data: map[string]any{
				"taskId":       "task_fail_01",
				"nodeId":       "indexer",
				"status":       "failed",
				"errorMessage": "connection refused: vector store unavailable",
				"nodeOrder":    3,
				"durationMs":   120,
			},
		},
	})

	// Discovery tools
	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_list",
			Description: "list documents",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "status", Type: ParamTypeString, Required: false}},
		},
		result: Result{
			Name:    "document_list",
			Status:  CallStatusSuccess,
			Summary: "found 2 documents",
			Data:    map[string]any{"failedCount": 2},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "document doc-1 is failed in pipeline mode",
			Data:    map[string]any{"documentId": "doc-1", "status": "failed", "processMode": "pipeline"},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_chunk_log_query",
			Description: "query chunk log",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_chunk_log_query",
			Status:  CallStatusSuccess,
			Summary: "chunk log shows 1 failed",
			Data:    map[string]any{"documentId": "doc-1", "latestTaskId": "task-1", "failedLogCount": 1, "latestStatus": "failed"},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "task_list",
			Description: "list tasks",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "status", Type: ParamTypeString, Required: false}},
		},
		result: Result{
			Name:    "task_list",
			Status:  CallStatusSuccess,
			Summary: "found 1 task",
			Data:    map[string]any{"runningCount": 1},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "task_ingestion_diagnose",
			Description: "diagnose task",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "taskId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "task_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "task diagnosis complete",
			Data:    map[string]any{"taskId": "task_fail_01", "conclusion": "task failed", "confidence": "high"},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "think",
			Description: "think",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "content", Type: ParamTypeString, Required: true}},
		},
		result: Result{Name: "think", Status: CallStatusSuccess, Summary: "thinking"},
	})

	return r
}

// TestGraphToolDocFailScenario verifies the graph tool is invoked for "doc_fail_01 why failed"
// and the agent loop completes in a single round.
func TestGraphToolDocFailScenario(t *testing.T) {
	registry := setupGraphTestRegistry()
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc_fail_01 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	// Should use graph tool — single call, single round.
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 graph tool call, got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected graph tool, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q: %s", result.Calls[0].Status, result.Calls[0].Summary)
	}

	// The graph tool should return node-level conclusion.
	if !strings.Contains(strings.ToLower(result.Calls[0].Summary), "diagnosis chain") {
		t.Fatalf("expected diagnosis chain summary, got %q", result.Calls[0].Summary)
	}

	// Agent should stop after 1 round since graph tool provides complete evidence.
	if len(result.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(result.Rounds))
	}
	if !result.Rounds[0].Done {
		t.Fatal("expected agent to finish after graph tool")
	}
}

// TestGraphToolDocRunScenario verifies the graph tool handles running documents correctly.
func TestGraphToolDocRunScenario(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed: document is still running at node indexer",
			Data: map[string]any{
				"documentId":   "doc_run_01",
				"latestTaskId": "task_run_01",
				"latestNodeId": "indexer",
				"conclusion":   "document is still processing, currently at node indexer",
				"confidence":   "high",
				"chainLength":  3,
			},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc_run_01 现在还在运行吗？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected graph tool, got %q", result.Calls[0].Name)
	}
	// Running scenario should NOT mention "failed" in summary.
	if strings.Contains(strings.ToLower(result.Calls[0].Summary), "failed") {
		t.Fatalf("running scenario should not mention failed, got %q", result.Calls[0].Summary)
	}
}

// TestGraphToolNotInvokedForChunkLogQuery verifies that non-diagnosis queries
// do NOT route to the graph tool.
func TestGraphToolNotInvokedForChunkLogQuery(t *testing.T) {
	registry := setupGraphTestRegistry()
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "帮我查一下 document doc-1 的 chunk log",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	if len(result.Calls) == 0 {
		t.Fatal("expected at least 1 call")
	}
	// Should route to document_chunk_log_query, NOT the graph tool.
	if result.Calls[0].Name == "document_root_cause_diagnosis" {
		t.Fatalf("chunk log query should NOT route to graph tool, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Name != "document_chunk_log_query" {
		t.Fatalf("expected document_chunk_log_query, got %q", result.Calls[0].Name)
	}
}

// TestGraphToolNotInvokedForOpenEndedQuestion verifies that open-ended questions
// route to discovery tools (document_list), not the graph tool.
func TestGraphToolNotInvokedForOpenEndedQuestion(t *testing.T) {
	registry := setupGraphTestRegistry()
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:         "最近有哪些文档导入失败了？",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	if len(result.Calls) == 0 {
		t.Fatal("expected at least 1 call")
	}
	if result.Calls[0].Name == "document_root_cause_diagnosis" {
		t.Fatalf("open-ended question should NOT route to graph tool, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Name != "document_list" {
		t.Fatalf("expected document_list for open-ended question, got %q", result.Calls[0].Name)
	}
}

// TestGraphToolTaskDiagnosisNotAffected verifies that task-level diagnosis
// still routes to task_ingestion_diagnose, not the document graph tool.
func TestGraphToolTaskDiagnosisNotAffected(t *testing.T) {
	registry := setupGraphTestRegistry()
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "ingestion task task_fail_01 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	if len(result.Calls) == 0 {
		t.Fatal("expected at least 1 call")
	}
	if result.Calls[0].Name == "document_root_cause_diagnosis" {
		t.Fatalf("task diagnosis should NOT route to document graph tool, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Name != "task_ingestion_diagnose" {
		t.Fatalf("expected task_ingestion_diagnose, got %q", result.Calls[0].Name)
	}
}

// TestGraphToolRealChainExecution verifies the REAL DiagnosisGraphTool (Eino-compiled)
// actually chains through all 3 hops via the executor, not just a stub.
func TestGraphToolRealChainExecution(t *testing.T) {
	registry := NewRegistry()

	// Register the 3 individual tools the real graph tool needs.
	invokeCount := 0
	registry.MustRegister(&countingTool{
		name: "document_ingestion_diagnose",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			invokeCount++
			return Result{
				Name:    "document_ingestion_diagnose",
				Status:  CallStatusSuccess,
				Summary: "diagnose complete",
				Data: map[string]any{
					"documentId":     "doc-1",
					"latestTaskId":   "task-1",
					"latestLogError": "chunk log error detected",
					"conclusion":     "document processing failed",
					"confidence":     "medium",
				},
			}, nil
		},
	})
	registry.MustRegister(&countingTool{
		name: "ingestion_task_query",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			invokeCount++
			return Result{
				Name:    "ingestion_task_query",
				Status:  CallStatusSuccess,
				Summary: "task query complete",
				Data: map[string]any{
					"taskId": "task-1",
					"status": "failed",
					"taskNodeSummary": []map[string]any{
						{"nodeId": "indexer", "status": "failed"},
					},
				},
			}, nil
		},
	})
	registry.MustRegister(&countingTool{
		name: "ingestion_task_node_query",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			invokeCount++
			return Result{
				Name:    "ingestion_task_node_query",
				Status:  CallStatusSuccess,
				Summary: "node query complete",
				Data: map[string]any{
					"taskId":       "task-1",
					"nodeId":       "indexer",
					"status":       "failed",
					"errorMessage": "connection refused: vector store unavailable",
				},
			}, nil
		},
	})

	executor := ragruntime.NewExecutor(registry)
	graphTool, err := raggraph.NewDiagnosisGraphTool(executor)
	if err != nil {
		t.Fatalf("create real diagnosis graph tool: %v", err)
	}
	registry.MustRegister(graphTool)

	// Also register think for LLM-free test.
	registry.MustRegister(&countingTool{
		name: "think",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			return Result{Name: "think", Status: CallStatusSuccess, Summary: "thinking"}, nil
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-1 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	// The REAL graph tool internally calls 3 tools via the executor.
	if invokeCount != 3 {
		t.Fatalf("expected 3 internal tool calls by the real graph, got %d", invokeCount)
	}

	// Single top-level call to the graph tool.
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call (graph tool), got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected graph tool, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q", result.Calls[0].Status)
	}

	// Single round — graph tool returns complete evidence.
	if len(result.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(result.Rounds))
	}
	if !result.Rounds[0].Done {
		t.Fatal("expected agent to finish after graph tool")
	}

	// Context should include the graph tool output.
	if result.Context == "" {
		t.Fatal("expected non-empty context from graph tool")
	}
}

// TestGraphToolDegradedFallback verifies that the agent loop correctly degrades
// when the graph tool fails, and still produces a degraded result.
func TestGraphToolDegradedFallback(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:         "document_root_cause_diagnosis",
			Status:       CallStatusFailed,
			Summary:      "",
			ErrorMessage: "graph execution failed: executor not available",
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc_fail_01 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	// Even when graph tool fails, agent loop should finish gracefully.
	if len(result.Calls) == 0 {
		t.Fatal("expected at least 1 call")
	}
	if result.Calls[0].Status != CallStatusFailed {
		t.Fatalf("expected failed status, got %q", result.Calls[0].Status)
	}
}

// TestGraphToolObserverNodeLevel verifies that when the graph tool returns
// diagnosisDepth=node_level, the RuleObserver gives confidence=0.95.
func TestGraphToolObserverNodeLevel(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed 3 hops",
			Data: map[string]any{
				"diagnosisDepth": "node_level",
				"conclusion":     "failed at node indexer: connection refused",
				"confidence":     "high",
			},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-fail-01 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(result.Rounds))
	}
	if result.Rounds[0].Confidence < 0.9 {
		t.Fatalf("expected confidence >= 0.9 for node_level, got %v", result.Rounds[0].Confidence)
	}
}

// TestGraphToolObserverUnknownDepth verifies that when diagnosisDepth is absent,
// the RuleObserver falls back to confidence=0.6.
func TestGraphToolObserverUnknownDepth(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_list",
			Description: "list documents",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "status", Type: ParamTypeString, Required: false}},
		},
		result: Result{
			Name:    "document_list",
			Status:  CallStatusSuccess,
			Summary: "found 3 documents",
			Data:    map[string]any{"total": 3},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:         "最近有哪些文档？",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(result.Rounds))
	}
	if result.Rounds[0].Confidence > 0.8 {
		t.Fatalf("expected confidence <= 0.8 for unknown depth, got %v", result.Rounds[0].Confidence)
	}
}

// TestFullDiagnoseSearchChain verifies the complete AgentLoop path:
// User asks for failure + solution → routes to document_diagnose_with_search
// → graph runs diagnose → web_search → returns combined result.
func TestFullDiagnoseSearchChain(t *testing.T) {
	registry := NewRegistry()
	// Stub: document_root_cause_diagnosis (called by the outer graph's first hop)
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed 3 hops",
			Data: map[string]any{
				"conclusion":     "document ingestion failed at node indexer: connection refused: vector store unavailable",
				"confidence":     "high",
				"diagnosisDepth": "node_level",
				"latestTaskId":   "task-fail-01",
				"latestNodeId":   "indexer",
			},
		},
	})
	// Stub: web_search (called by the outer graph's second hop)
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "web_search",
			Description: "search the web",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "query", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "web_search",
			Status:  CallStatusSuccess,
			Summary: "found 2 web results",
			Data: map[string]any{
				"query":   "connection refused troubleshooting",
				"results": []map[string]any{},
			},
		},
	})

	executor := ragruntime.NewExecutor(registry)
	searchTool, err := raggraph.NewDiagnoseSearchGraphTool(executor)
	if err != nil {
		t.Fatalf("create real diagnose-search graph tool: %v", err)
	}
	registry.MustRegister(searchTool)

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-fail-01 失败的原因是什么？有没有修复方案参考？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	// 1 call, 1 round — graph tool completes in a single hop.
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0].Name != "document_diagnose_with_search" {
		t.Fatalf("expected document_diagnose_with_search, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q: %s", result.Calls[0].Status, result.Calls[0].Summary)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(result.Rounds))
	}

	summary := result.Calls[0].Summary
	if !strings.Contains(summary, "web_search") {
		t.Fatalf("expected web_search in summary, got %q", summary)
	}
	if !strings.Contains(summary, "diagnosis=success") {
		t.Fatalf("expected diagnosis=success in summary, got %q", summary)
	}

	// Observer should give high confidence for the graph tool result.
	if result.Rounds[0].Confidence < 0.6 {
		t.Fatalf("expected confidence >= 0.6, got %v", result.Rounds[0].Confidence)
	}
}

// TestDiagnoseSearchNotTriggeredForPlainDiagnosis verifies that plain diagnosis
// queries (without solution keywords) still route to the basic graph tool.
func TestDiagnoseSearchNotTriggeredForPlainDiagnosis(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis complete",
			Data:    map[string]any{"conclusion": "failed", "diagnosisDepth": "node_level"},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-fail-01 为什么失败了？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}

	if len(result.Calls) == 0 {
		t.Fatal("expected at least 1 call")
	}
	// Plain diagnosis query should NOT route to diagnose+search.
	if result.Calls[0].Name == "document_diagnose_with_search" {
		t.Fatalf("plain diagnosis should not route to diagnose+search, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected document_root_cause_diagnosis, got %q", result.Calls[0].Name)
	}
}
