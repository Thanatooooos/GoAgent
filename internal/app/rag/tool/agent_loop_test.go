package tool

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	graphmod "local/rag-project/internal/app/rag/tool/modules/graph"
	metamod "local/rag-project/internal/app/rag/tool/modules/meta"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	tracemod "local/rag-project/internal/app/rag/tool/modules/trace"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
)

// testInvoker is a minimal ToolInvoker for tests that never calls Invoke.
type testInvoker struct{}

func (testInvoker) Invoke(_ context.Context, _ Call) (Result, error) {
	return Result{}, nil
}

func TestMain(m *testing.M) {
	setupTestRegistry()
	os.Exit(m.Run())
}

var testRegistry *Registry

func setupTestRegistry() *Registry {
	r := NewRegistry()

	register := func(name string, spec ToolSpec, behavior ToolBehavior) {
		r.MustRegisterModule(ToolModule{Name: name, Invoker: testInvoker{}, Spec: spec, Behavior: behavior}.Normalize())
	}

	systemSpec := ToolSpec{Capability: CapabilityDiagnosis, EvidenceSources: []string{EvidenceSourceSystemRecords}, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ReadOnly: true, Family: "system"}
	webSpec := ToolSpec{Capability: CapabilitySearch, EvidenceSources: []string{EvidenceSourceExternalWeb}, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ReadOnly: true, Family: "web"}
	traceSpec := ToolSpec{Capability: CapabilityDiagnosis, EvidenceSources: []string{EvidenceSourceRAGTrace}, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ReadOnly: true, Family: "trace"}

	register("think", ToolSpec{Capability: CapabilityGeneral, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ReadOnly: true, Family: "system"}, metamod.ThinkBehavior())
	register("web_search", webSpec, webmod.WebSearchBehavior())
	webFetchSpec := webSpec
	webFetchSpec.After = []string{"web_search"}
	register("web_fetch", webFetchSpec, webmod.WebFetchBehavior())
	register("external_evidence_workflow", webSpec, webmod.ExternalEvidenceWorkflowBehavior())
	register("document_list", systemSpec, systemmod.DocumentListBehavior())
	register("task_list", systemSpec, systemmod.TaskListBehavior())
	register("document_query", systemSpec, systemmod.DocumentQueryBehavior())
	register("document_chunk_log_query", systemSpec, systemmod.DocumentChunkLogQueryBehavior())
	diagnoseSpec := systemSpec
	diagnoseSpec.After = []string{"document_query", "document_chunk_log_query"}
	register("document_ingestion_diagnose", diagnoseSpec, systemmod.DocumentIngestionDiagnoseBehavior())
	taskQuerySpec := systemSpec
	taskQuerySpec.After = []string{"document_ingestion_diagnose", "document_chunk_log_query", "task_ingestion_diagnose"}
	register("ingestion_task_query", taskQuerySpec, systemmod.IngestionTaskQueryBehavior())
	nodeQuerySpec := systemSpec
	nodeQuerySpec.After = []string{"ingestion_task_query", "document_ingestion_diagnose", "task_ingestion_diagnose"}
	register("ingestion_task_node_query", nodeQuerySpec, systemmod.IngestionTaskNodeQueryBehavior())
	register("task_ingestion_diagnose", systemSpec, systemmod.TaskIngestionDiagnoseBehavior())
	register("trace_node_query", traceSpec, tracemod.TraceNodeQueryBehavior())
	register("trace_retrieval_diagnose", traceSpec, tracemod.TraceRetrievalDiagnoseBehavior())
	register("document_root_cause_diagnosis", systemSpec, graphmod.DocumentRootCauseDiagnosisBehavior())
	register("document_diagnose_with_search", systemSpec, graphmod.DocumentDiagnoseWithSearchBehavior())

	testRegistry = r
	return r
}

type plannerStub struct {
	results []PlanResult
	inputs  []PlanInput
}

func (s *plannerStub) Plan(_ context.Context, input PlanInput) (PlanResult, error) {
	s.inputs = append(s.inputs, input)
	if len(s.results) == 0 {
		return PlanResult{}, nil
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}

type eventSinkRecorder struct {
	thinks  []string
	starts  []ToolCallEvent
	results []ToolCallEvent
}

func (r *eventSinkRecorder) OnAgentThink(message string) error {
	r.thinks = append(r.thinks, message)
	return nil
}

func (r *eventSinkRecorder) OnToolStart(event ToolCallEvent) error {
	r.starts = append(r.starts, event)
	return nil
}

func (r *eventSinkRecorder) OnToolResult(event ToolCallEvent) error {
	r.results = append(r.results, event)
	return nil
}

type staticTool struct {
	definition   Definition
	result       Result
	delay        time.Duration
	onInvoke     func()
	invokeResult func(call Call) Result
}

func (t staticTool) Definition() Definition {
	return t.definition
}

func (t staticTool) Invoke(_ context.Context, call Call) (Result, error) {
	if t.onInvoke != nil {
		t.onInvoke()
	}
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	if t.invokeResult != nil {
		return t.invokeResult(call), nil
	}
	return t.result, nil
}

func registerKnownTestTool(registry *Registry, tool staticTool) {
	systemSpec := ToolSpec{
		Capability:      CapabilityDiagnosis,
		EvidenceSources: []string{EvidenceSourceSystemRecords},
		ExecutionMode:   ExecutionModeReadOnly,
		RiskLevel:       RiskLevelLow,
		ReadOnly:        true,
		Family:          "system",
	}
	webSpec := ToolSpec{
		Capability:      CapabilitySearch,
		EvidenceSources: []string{EvidenceSourceExternalWeb},
		ExecutionMode:   ExecutionModeReadOnly,
		RiskLevel:       RiskLevelLow,
		ReadOnly:        true,
		Family:          "web",
	}
	traceSpec := ToolSpec{
		Capability:      CapabilityDiagnosis,
		EvidenceSources: []string{EvidenceSourceRAGTrace},
		ExecutionMode:   ExecutionModeReadOnly,
		RiskLevel:       RiskLevelLow,
		ReadOnly:        true,
		Family:          "trace",
	}

	var (
		spec     ToolSpec
		behavior ToolBehavior
	)
	switch tool.definition.Name {
	case "think":
		spec = ToolSpec{Capability: CapabilityGeneral, ExecutionMode: ExecutionModeReadOnly, RiskLevel: RiskLevelLow, ReadOnly: true, Family: "system"}
		behavior = metamod.ThinkBehavior()
	case "web_search":
		spec = webSpec
		behavior = webmod.WebSearchBehavior()
	case "web_fetch":
		spec = webSpec
		spec.After = []string{"web_search"}
		behavior = webmod.WebFetchBehavior()
	case "external_evidence_workflow":
		spec = webSpec
		behavior = webmod.ExternalEvidenceWorkflowBehavior()
	case "trace_node_query":
		spec = traceSpec
		behavior = tracemod.TraceNodeQueryBehavior()
	case "trace_retrieval_diagnose":
		spec = traceSpec
		behavior = tracemod.TraceRetrievalDiagnoseBehavior()
	case "document_root_cause_diagnosis":
		spec = systemSpec
		behavior = graphmod.DocumentRootCauseDiagnosisBehavior()
	case "document_diagnose_with_search":
		spec = systemSpec
		behavior = graphmod.DocumentDiagnoseWithSearchBehavior()
	case "document_list":
		spec = systemSpec
		behavior = systemmod.DocumentListBehavior()
	case "task_list":
		spec = systemSpec
		behavior = systemmod.TaskListBehavior()
	case "document_query":
		spec = systemSpec
		behavior = systemmod.DocumentQueryBehavior()
	case "document_chunk_log_query":
		spec = systemSpec
		behavior = systemmod.DocumentChunkLogQueryBehavior()
	case "document_ingestion_diagnose":
		spec = systemSpec
		spec.After = []string{"document_query", "document_chunk_log_query"}
		behavior = systemmod.DocumentIngestionDiagnoseBehavior()
	case "ingestion_task_query":
		spec = systemSpec
		spec.After = []string{"document_ingestion_diagnose", "document_chunk_log_query", "task_ingestion_diagnose"}
		behavior = systemmod.IngestionTaskQueryBehavior()
	case "ingestion_task_node_query":
		spec = systemSpec
		spec.After = []string{"ingestion_task_query", "document_ingestion_diagnose", "task_ingestion_diagnose"}
		behavior = systemmod.IngestionTaskNodeQueryBehavior()
	case "task_ingestion_diagnose":
		spec = systemSpec
		behavior = systemmod.TaskIngestionDiagnoseBehavior()
	default:
		registry.MustRegister(tool)
		return
	}

	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(tool, spec, behavior).Module())
}

func TestAgentLoopRunsMultipleRounds(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_ingestion_diagnose",
			Description: "diagnose document",
			ReadOnly:    true,
			Parameters: []ParameterDefinition{
				{Name: "documentId", Type: ParamTypeString, Required: true},
			},
		},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "document=doc-1 confidence=high conclusion=document ingestion failed at node indexer latestTask=task-1 node=indexer",
			Data: map[string]any{
				"conclusion":      "document ingestion failed at node indexer",
				"confidence":      "high",
				"latestTaskId":    "task-1",
				"latestNodeId":    "indexer",
				"latestNodeError": "",
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Summary: "task=task-1 node=indexer error=embedding API rate limit exceeded",
			Data: map[string]any{
				"taskId":       "task-1",
				"nodeId":       "indexer",
				"errorMessage": "embedding API rate limit exceeded",
			},
		},
	})

	planner := &plannerStub{
		results: []PlanResult{
			{
				Calls: []Call{{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": "doc-1"}}},
			},
			{
				Calls: []Call{{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": "task-1", "nodeId": "indexer"}}},
			},
		},
	}
	sink := &eventSinkRecorder{}

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:  "doc-1 为什么失败了？",
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if !result.Used {
		t.Fatal("expected workflow result to be used")
	}
	if len(result.Rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(result.Rounds))
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(result.Calls))
	}
	if len(sink.starts) != 2 || len(sink.results) != 2 {
		t.Fatalf("expected 2 tool starts/results, got starts=%d results=%d", len(sink.starts), len(sink.results))
	}
	if len(sink.thinks) != 1 {
		t.Fatalf("expected 1 agent think event, got %d", len(sink.thinks))
	}
	if len(planner.inputs) != 1 {
		t.Fatalf("expected planner to run only in round 1, got %d calls", len(planner.inputs))
	}
	if result.Rounds[0].PlanningSource != "llm" {
		t.Fatalf("expected round 1 planningSource=llm, got %q", result.Rounds[0].PlanningSource)
	}
	if result.Rounds[1].PlanningSource != "hint_calls" {
		t.Fatalf("expected round 2 planningSource=hint_calls, got %q", result.Rounds[1].PlanningSource)
	}
	if !result.Rounds[1].LLMPlannerSkipped {
		t.Fatal("expected round 2 to skip llm planner when nextHintCalls exist")
	}
	if result.Rounds[0].NextHintCallCount != 1 {
		t.Fatalf("expected round 1 nextHintCallCount=1, got %d", result.Rounds[0].NextHintCallCount)
	}
	if len(result.Rounds) < 1 || !strings.HasPrefix(result.Rounds[0].NextHint, "tool:ingestion_task_node_query|") {
		t.Fatalf("expected structured next hint, got %q", result.Rounds[0].NextHint)
	}
	if len(result.Rounds[0].NextHintCalls) != 1 {
		t.Fatalf("expected round to preserve structured hint calls, got %+v", result.Rounds[0].NextHintCalls)
	}
}

func TestPlanCallsFromHintParsesStructuredHint(t *testing.T) {
	calls := ragruntime.PlanCallsFromHint("tool:ingestion_task_query|taskId=task-1|includeNodes=true", []Definition{{
		Name: "ingestion_task_query",
		Parameters: []ParameterDefinition{
			{Name: "taskId", Type: ParamTypeString, Required: true},
			{Name: "includeNodes", Type: ParamTypeBoolean, Required: false},
		},
	}})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "ingestion_task_query" {
		t.Fatalf("expected ingestion_task_query, got %q", calls[0].Name)
	}
	if got := readStringArg(calls[0].Arguments, "taskId"); got != "task-1" {
		t.Fatalf("expected taskId task-1, got %q", got)
	}
	includeNodes, ok := calls[0].Arguments["includeNodes"].(bool)
	if !ok || !includeNodes {
		t.Fatalf("expected includeNodes=true, got %#v", calls[0].Arguments["includeNodes"])
	}
}

func TestPlanCallsFromHintCallsUsesStructuredArguments(t *testing.T) {
	calls := ragruntime.PlanCallsFromHintCalls([]HintCall{{
		Name: "ingestion_task_query",
		Arguments: map[string]any{
			"taskId":       "task-1",
			"includeNodes": true,
		},
	}}, []Definition{{
		Name: "ingestion_task_query",
		Parameters: []ParameterDefinition{
			{Name: "taskId", Type: ParamTypeString, Required: true},
			{Name: "includeNodes", Type: ParamTypeBoolean, Required: false},
		},
	}})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "ingestion_task_query" {
		t.Fatalf("unexpected call name: %q", calls[0].Name)
	}
	includeNodes, ok := calls[0].Arguments["includeNodes"].(bool)
	if !ok || !includeNodes {
		t.Fatalf("expected includeNodes=true, got %#v", calls[0].Arguments["includeNodes"])
	}
}

func TestPlanCallsFromResultsFallsBackFromDocumentQuery(t *testing.T) {
	calls := ragruntime.PlanCallsFromResultsWithRegistry([]Result{
		{
			Name: "document_query",
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
			},
		},
	}, WorkflowInput{}, testRegistry)
	if len(calls) != 1 {
		t.Fatalf("expected 1 fallback call, got %d", len(calls))
	}
	if calls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected document_ingestion_diagnose, got %q", calls[0].Name)
	}
	if got := readStringArg(calls[0].Arguments, "documentId"); got != "doc-1" {
		t.Fatalf("expected documentId doc-1, got %q", got)
	}
}

func TestPlanWithBaseRulesUsesDocumentDiagnosisForRunningQuestion(t *testing.T) {
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question: "document doc_run_01 现在还在运行吗？",
	}, ragruntime.DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected first call to be document_root_cause_diagnosis, got %q", calls[0].Name)
	}
}

func TestPlanWithBaseRulesUsesDocumentDiagnosisForCurrentNodeQuestion(t *testing.T) {
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question: "帮我看看 doc_run_01 现在跑到哪个节点了",
	}, ragruntime.DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected first call to be document_root_cause_diagnosis, got %q", calls[0].Name)
	}
}

func TestPlanWithBaseRulesOpenEndedDocumentsFailed(t *testing.T) {
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question:         "最近有哪些文档导入失败了？",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragruntime.DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_list" {
		t.Fatalf("expected document_list, got %q", calls[0].Name)
	}
	if readStringArg(calls[0].Arguments, "status") != "failed" {
		t.Fatalf("expected status=failed, got %q", readStringArg(calls[0].Arguments, "status"))
	}
	if readStringArg(calls[0].Arguments, "knowledgeBaseId") != "kb-1" {
		t.Fatalf("expected knowledgeBaseId=kb-1, got %q", readStringArg(calls[0].Arguments, "knowledgeBaseId"))
	}
}

func TestPlanWithBaseRulesOpenEndedTasksRunning(t *testing.T) {
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question: "哪些ingestion任务还在运行中？",
	}, ragruntime.DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "task_list" {
		t.Fatalf("expected task_list, got %q", calls[0].Name)
	}
	if readStringArg(calls[0].Arguments, "status") != "running" {
		t.Fatalf("expected status=running, got %q", readStringArg(calls[0].Arguments, "status"))
	}
}

func TestPlanWithBaseRulesOpenEndedDocumentsFailedNoDefaultKB(t *testing.T) {
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question: "最近有哪些文档导入失败了？",
	}, ragruntime.DefaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_list" {
		t.Fatalf("expected document_list, got %q", calls[0].Name)
	}
	if readStringArg(calls[0].Arguments, "status") != "failed" {
		t.Fatalf("expected status=failed, got %q", readStringArg(calls[0].Arguments, "status"))
	}
	// Without a knowledge base in context, the arg should be omitted.
	if readStringArg(calls[0].Arguments, "knowledgeBaseId") != "" {
		t.Fatalf("expected no knowledgeBaseId when context is empty, got %q", readStringArg(calls[0].Arguments, "knowledgeBaseId"))
	}
}

func TestPlanWithBaseRulesOpenEndedNotTriggeredForSpecificID(t *testing.T) {
	// Should NOT fall into open-ended path when a specific doc ID is present.
	calls := ragruntime.PlanWithBaseRules(WorkflowInput{
		Question: "doc_fail_01 这个失败的文档什么情况？",
	}, ragruntime.DefaultMaxIterations)
	if len(calls) == 0 {
		t.Fatal("expected at least 1 call for specific doc ID")
	}
	for _, call := range calls {
		if call.Name == "document_list" {
			t.Fatal("expected document_list to NOT be planned when a specific doc ID is present")
		}
	}
}

func TestPlanCallsFromResultsFallsBackFromChunkLogToTaskQuery(t *testing.T) {
	calls := ragruntime.PlanCallsFromResultsWithRegistry([]Result{
		{
			Name: "document_chunk_log_query",
			Data: map[string]any{
				"documentId":      "doc-1",
				"latestTaskId":    "task-1",
				"latestStatus":    "running",
				"runningLogCount": 1,
			},
		},
	}, WorkflowInput{}, testRegistry)
	if len(calls) != 1 {
		t.Fatalf("expected 1 fallback call, got %d", len(calls))
	}
	if calls[0].Name != "ingestion_task_query" {
		t.Fatalf("expected ingestion_task_query, got %q", calls[0].Name)
	}
	if got := readStringArg(calls[0].Arguments, "taskId"); got != "task-1" {
		t.Fatalf("expected taskId task-1, got %q", got)
	}
	includeNodes, ok := calls[0].Arguments["includeNodes"].(bool)
	if !ok || !includeNodes {
		t.Fatalf("expected includeNodes=true, got %#v", calls[0].Arguments["includeNodes"])
	}
}

func TestPlanCallsFromResultsFallsBackFromTaskQueryToNodeQuery(t *testing.T) {
	calls := ragruntime.PlanCallsFromResultsWithRegistry([]Result{
		{
			Name: "ingestion_task_query",
			Data: map[string]any{
				"taskId": "task-1",
				"status": "running",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "fetcher", "status": "success"},
					{"nodeId": "indexer", "status": "running"},
				},
			},
		},
	}, WorkflowInput{}, testRegistry)
	if len(calls) != 1 {
		t.Fatalf("expected 1 fallback call, got %d", len(calls))
	}
	if calls[0].Name != "ingestion_task_node_query" {
		t.Fatalf("expected ingestion_task_node_query, got %q", calls[0].Name)
	}
	if got := readStringArg(calls[0].Arguments, "taskId"); got != "task-1" {
		t.Fatalf("expected taskId task-1, got %q", got)
	}
	if got := readStringArg(calls[0].Arguments, "nodeId"); got != "indexer" {
		t.Fatalf("expected nodeId indexer, got %q", got)
	}
}

func TestPlanCallsFromResultsFallsBackFromWebSearchToWebFetch(t *testing.T) {
	calls := ragruntime.PlanCallsFromResultsWithRegistry([]Result{
		{
			Name: "web_search",
			Data: map[string]any{
				"results": []map[string]any{
					{"url": "https://example.com/a", "policy": "deny"},
					{"url": "https://example.com/b", "policy": "allow"},
				},
			},
		},
	}, WorkflowInput{}, testRegistry)
	if len(calls) != 1 {
		t.Fatalf("expected 1 fallback call, got %d", len(calls))
	}
	if calls[0].Name != "web_fetch" {
		t.Fatalf("expected web_fetch, got %q", calls[0].Name)
	}
	urls, ok := calls[0].Arguments["urls"].([]string)
	if !ok {
		t.Fatalf("expected urls []string, got %#v", calls[0].Arguments["urls"])
	}
	if len(urls) != 1 || urls[0] != "https://example.com/b" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestObserveDocumentDiagnosisKeepsDrillingWhenOnlyLogErrorExists(t *testing.T) {
	observation, _ := systemmod.DocumentIngestionDiagnoseBehavior().Observe(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":     "document ingestion failed, but no failed node was captured",
			"confidence":     "medium",
			"latestTaskId":   "task-1",
			"latestLogError": "indexer failed after retries",
		},
	}, ObserveInput{})
	if observation.Done {
		t.Fatal("expected observer to continue when only task/log-level error exists")
	}
	if observation.State.NextHint != "tool:ingestion_task_query|taskId=task-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.State.NextHint)
	}
}

func TestObserveDocumentDiagnosisStopsOnNodeLevelError(t *testing.T) {
	observation, _ := systemmod.DocumentIngestionDiagnoseBehavior().Observe(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":      "document ingestion failed at node indexer",
			"confidence":      "high",
			"latestTaskId":    "task-1",
			"latestNodeId":    "indexer",
			"latestNodeError": "connection refused: vector store unavailable",
		},
	}, ObserveInput{})
	if !observation.Done {
		t.Fatal("expected observer to stop when node-level error already exists")
	}
}

func TestPlanCallsFromHintCallsSupportsNewToolDefinitionsWithoutHardcodedSwitch(t *testing.T) {
	calls := ragruntime.PlanCallsFromHintCalls([]HintCall{{
		Name: "metric_query",
		Arguments: map[string]any{
			"metricName": "ingestion_failures",
			"windowDays": "7",
		},
	}}, []Definition{{
		Name: "metric_query",
		Parameters: []ParameterDefinition{
			{Name: "metricName", Type: ParamTypeString, Required: true},
			{Name: "windowDays", Type: ParamTypeInteger, Required: true},
		},
	}})
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "metric_query" {
		t.Fatalf("unexpected call name: %q", calls[0].Name)
	}
	if got := readStringArg(calls[0].Arguments, "metricName"); got != "ingestion_failures" {
		t.Fatalf("expected metricName ingestion_failures, got %q", got)
	}
	windowDays, ok := calls[0].Arguments["windowDays"].(int)
	if !ok || windowDays != 7 {
		t.Fatalf("expected windowDays=7, got %#v", calls[0].Arguments["windowDays"])
	}
}

func TestObserveDocumentDiagnosisRunningKeepsVerifyingTaskState(t *testing.T) {
	observation, _ := systemmod.DocumentIngestionDiagnoseBehavior().Observe(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":   "document ingestion task is still running",
			"confidence":   "medium",
			"latestTaskId": "task-run-1",
		},
	}, ObserveInput{})
	if observation.Done {
		t.Fatal("expected running document diagnosis to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if observation.State.NextHint != "tool:ingestion_task_query|taskId=task-run-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.State.NextHint)
	}
}

func TestObserveTaskDiagnosisRunningKeepsVerifyingTaskState(t *testing.T) {
	observation, _ := systemmod.TaskIngestionDiagnoseBehavior().Observe(Result{
		Name: "task_ingestion_diagnose",
		Data: map[string]any{
			"taskId":     "task-run-1",
			"conclusion": "task is still running and no failed node has been confirmed yet",
			"confidence": "medium",
		},
	}, ObserveInput{})
	if observation.Done {
		t.Fatal("expected running task diagnosis to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if observation.State.NextHint != "tool:ingestion_task_query|taskId=task-run-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.State.NextHint)
	}
}

func TestObserveTaskQueryRunningNodeUsesVerificationInsteadOfFailureDrilldown(t *testing.T) {
	observation, _ := systemmod.IngestionTaskQueryBehavior().Observe(Result{
		Name: "ingestion_task_query",
		Data: map[string]any{
			"taskId": "task-run-1",
			"status": "running",
			"taskNodeSummary": []map[string]any{
				{"nodeId": "fetcher", "status": "success"},
				{"nodeId": "indexer", "status": "running"},
			},
		},
	}, ObserveInput{})
	if observation.Done {
		t.Fatal("expected running task query to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if !strings.Contains(observation.State.Hypothesis, "still running") {
		t.Fatalf("expected running hypothesis, got %q", observation.State.Hypothesis)
	}
	if observation.State.NextHint != "tool:ingestion_task_node_query|taskId=task-run-1|nodeId=indexer" {
		t.Fatalf("unexpected next hint: %q", observation.State.NextHint)
	}
}

func TestObserveWebFetchUsesCombinedTextFromPages(t *testing.T) {
	observation, _ := webmod.WebFetchBehavior().Observe(Result{
		Name:   "web_fetch",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"pages": []map[string]any{
				{
					"url":          "https://example.com/a",
					"text":         "This page explains the root cause and the fix.",
					"wasTruncated": true,
				},
			},
		},
	}, ObserveInput{})
	if !observation.Done {
		t.Fatal("expected web_fetch observation to finish")
	}
	if observation.State.Confidence != 0.7 {
		t.Fatalf("expected truncated fetch confidence=0.7, got %v", observation.State.Confidence)
	}
	if observation.State.Phase != "complete" {
		t.Fatalf("expected complete phase, got %q", observation.State.Phase)
	}
}

func TestAgentLoopDocRunScenarioStaysInVerificationPath(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_root_cause_diagnosis",
			Description: "diagnosis graph: chains diagnose → task_query → node_query",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_root_cause_diagnosis",
			Status:  CallStatusSuccess,
			Summary: "diagnosis chain completed: document is still running at node indexer",
			Data: map[string]any{
				"documentId":   "doc-run-1",
				"latestTaskId": "task-run-1",
				"latestNodeId": "indexer",
				"conclusion":   "document is still running at node indexer",
				"confidence":   "high",
				"chainLength":  3,
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_ingestion_diagnose",
			Description: "diagnose document ingestion",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "document ingestion is still running",
			Data: map[string]any{
				"documentId":   "doc-run-1",
				"latestTaskId": "task-run-1",
				"conclusion":   "document ingestion is still running",
				"confidence":   "high",
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Summary: "task task-run-1 is still running at node indexer",
			Data: map[string]any{
				"taskId": "task-run-1",
				"status": "running",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "fetcher", "status": "success"},
					{"nodeId": "indexer", "status": "running"},
				},
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Summary: "task task-run-1 node indexer is still running",
			Data: map[string]any{
				"taskId":  "task-run-1",
				"nodeId":  "indexer",
				"status":  "running",
				"summary": "indexer node is still running",
			},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-run-1 当前还在运行吗？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call from graph tool, got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0].Name != "document_root_cause_diagnosis" {
		t.Fatalf("expected graph tool, got %q", result.Calls[0].Name)
	}
}

func TestAgentLoopTaskRunScenarioStaysInVerificationPath(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "task_ingestion_diagnose",
			Description: "diagnose task ingestion",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "taskId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "task_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "ingestion task is still running",
			Data: map[string]any{
				"taskId":     "task-run-1",
				"conclusion": "ingestion task is still running",
				"confidence": "high",
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Summary: "task task-run-1 is still running at node indexer",
			Data: map[string]any{
				"taskId": "task-run-1",
				"status": "running",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "fetcher", "status": "success"},
					{"nodeId": "indexer", "status": "running"},
				},
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Summary: "task task-run-1 node indexer is still running",
			Data: map[string]any{
				"taskId":  "task-run-1",
				"nodeId":  "indexer",
				"status":  "running",
				"summary": "indexer node is still running",
			},
		},
	})

	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "task-run-1 当前还在运行吗？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "task_ingestion_diagnose" || result.Calls[1].Name != "ingestion_task_query" || result.Calls[2].Name != "ingestion_task_node_query" {
		t.Fatalf("unexpected call order: %+v", result.Calls)
	}
	if len(result.Rounds) < 2 {
		t.Fatalf("expected at least 2 rounds, got %d", len(result.Rounds))
	}
	if result.Rounds[0].State.Phase != "verification" {
		t.Fatalf("expected task diagnosis round to remain in verification, got %q", result.Rounds[0].State.Phase)
	}
	if result.Rounds[1].State.Phase != "verification" {
		t.Fatalf("expected task query round to remain in verification, got %q", result.Rounds[1].State.Phase)
	}
	if strings.Contains(strings.ToLower(result.Rounds[1].State.Hypothesis), "failed") {
		t.Fatalf("expected running scenario hypothesis to avoid failed wording, got %q", result.Rounds[1].State.Hypothesis)
	}
}

func TestAgentLoopThinkToolCapturesReasoningInTrace(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "think",
			Description: "Record a reasoning thought before taking action.",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "thought", Type: ParamTypeString, Required: true}},
		},
		onInvoke: func() {},
		invokeResult: func(call Call) Result {
			return Result{
				Name:    "think",
				Status:  CallStatusSuccess,
				Summary: strings.TrimSpace(readStringArg(call.Arguments, "thought")),
			}
		},
	})
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "document=doc-1 status=failed processMode=pipeline",
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_ingestion_diagnose",
			Description: "diagnose document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "document=doc-1 conclusion=indexer failed confidence=high latestTaskId=task-1 latestNodeError=connection refused",
			Data: map[string]any{
				"conclusion":      "document ingestion failed at node indexer",
				"confidence":      "high",
				"latestTaskId":    "task-1",
				"latestNodeError": "connection refused: vector store unavailable",
				"documentId":      "doc-1",
			},
		},
	})

	planner := &plannerStub{
		results: []PlanResult{
			{
				Calls: []Call{
					{Name: "think", Arguments: map[string]any{"thought": "用户问doc-1的导入状态。先查文档基础信息，如果状态是failed且pipeline模式，再深入诊断。"}},
					{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}},
				},
			},
			{
				Calls: []Call{{Name: "document_ingestion_diagnose", Arguments: map[string]any{"documentId": "doc-1"}}},
			},
		},
	}
	sink := &eventSinkRecorder{}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:  "doc-1 现在什么状态？",
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(result.Rounds))
	}
	// Round 1: think + document_query = 2 calls
	if len(result.Calls) != 3 {
		t.Fatalf("expected 3 total calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "think" {
		t.Fatalf("expected first call to be think, got %q", result.Calls[0].Name)
	}
	if result.Calls[0].Summary != "用户问doc-1的导入状态。先查文档基础信息，如果状态是failed且pipeline模式，再深入诊断。" {
		t.Fatalf("expected think thought in summary, got %q", result.Calls[0].Summary)
	}
	if result.Calls[1].Name != "document_query" {
		t.Fatalf("expected second call to be document_query, got %q", result.Calls[1].Name)
	}
	if result.Calls[2].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected third call to be document_ingestion_diagnose, got %q", result.Calls[2].Name)
	}
	// SSE: 3 tool_start + 3 tool_result
	if len(sink.starts) != 3 {
		t.Fatalf("expected 3 tool starts, got %d", len(sink.starts))
	}
	if sink.starts[0].Name != "think" {
		t.Fatalf("expected first start event to be think, got %q", sink.starts[0].Name)
	}
	if len(sink.results) != 3 {
		t.Fatalf("expected 3 tool results, got %d", len(sink.results))
	}
	if sink.results[0].Name != "think" || sink.results[0].Summary != "用户问doc-1的导入状态。先查文档基础信息，如果状态是failed且pipeline模式，再深入诊断。" {
		t.Fatalf("expected think result with thought summary, got name=%q summary=%q", sink.results[0].Name, sink.results[0].Summary)
	}
	// Observer decision should be based on document_query result (latest in round), not think.
	// Round 0 should have continued (Done=false) because document_query shows failed pipeline.
	if len(result.Rounds) < 1 {
		t.Fatal("expected at least 1 round")
	}
	if result.Rounds[0].Done {
		t.Fatal("expected round 1 not done since document_query shows failed pipeline")
	}
	// Round 1: 2 calls
	if result.Rounds[0].ToolCallCount != 2 {
		t.Fatalf("expected round 1 to have 2 calls, got %d", result.Rounds[0].ToolCallCount)
	}
	if len(planner.inputs) != 1 {
		t.Fatalf("expected planner to run only in round 1, got %d planner calls", len(planner.inputs))
	}
	if result.Rounds[1].PlanningSource != "hint_calls" {
		t.Fatalf("expected round 2 planningSource=hint_calls, got %q", result.Rounds[1].PlanningSource)
	}
	if !result.Rounds[1].LLMPlannerSkipped {
		t.Fatal("expected round 2 to skip llm planner after observer emitted nextHintCalls")
	}
}

func TestAgentLoopRejectsPlannerCallWithInventedNodeID(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "ingestion_task_query",
			Description: "query task",
			ReadOnly:    true,
			Parameters: []ParameterDefinition{
				{Name: "taskId", Type: ParamTypeString, Required: true},
				{Name: "includeNodes", Type: ParamTypeBoolean, Required: false},
			},
		},
		result: Result{
			Name:   "ingestion_task_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId": "task-1",
				"status": "failed",
				"taskNodeSummary": []map[string]any{
					{"nodeId": "fetcher", "status": "success"},
					{"nodeId": "indexer", "status": "failed"},
				},
			},
		},
	})
	registerKnownTestTool(registry, staticTool{
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
			Name:   "ingestion_task_node_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId": "task-1",
				"nodeId": "indexer",
				"status": "failed",
			},
		},
	})

	planner := &plannerStub{
		results: []PlanResult{
			{Calls: []Call{{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": "task-1", "includeNodes": true}}}},
			{Calls: []Call{{Name: "ingestion_task_node_query", Arguments: map[string]any{"taskId": "task-1", "nodeId": "node_0"}}}},
		},
	}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "task-1 why failed?",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(result.Calls))
	}
	if result.Calls[1].Name != "ingestion_task_node_query" {
		t.Fatalf("expected second call to be ingestion_task_node_query, got %q", result.Calls[1].Name)
	}
	if got := readStringArg(result.Calls[1].Arguments, "nodeId"); got != "indexer" {
		t.Fatalf("expected rule fallback to recover nodeId indexer, got %q", got)
	}
}

func TestAgentLoopParallelToolCallsPreserveEventOrder(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "matched doc-1",
		},
		delay: 40 * time.Millisecond,
	})
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "trace_node_query",
			Description: "query trace",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "traceId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "trace_node_query",
			Status:  CallStatusSuccess,
			Summary: "matched trace-1",
		},
		delay: 5 * time.Millisecond,
	})

	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{
				{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}},
				{Name: "trace_node_query", Arguments: map[string]any{"traceId": "trace-1"}},
			},
		}},
	}
	sink := &eventSinkRecorder{}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetParallelToolCalls(true, 2)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question:  "帮我同时看 doc-1 和 trace-1",
		TraceID:   "trace-1",
		EventSink: sink,
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(result.Calls))
	}
	if len(sink.starts) != 2 || len(sink.results) != 2 {
		t.Fatalf("expected 2 start/result events, got starts=%d results=%d", len(sink.starts), len(sink.results))
	}
	if sink.starts[0].Name != "document_query" || sink.starts[1].Name != "trace_node_query" {
		t.Fatalf("unexpected start event order: %+v", sink.starts)
	}
	if sink.results[0].Name != "document_query" || sink.results[1].Name != "trace_node_query" {
		t.Fatalf("unexpected result event order: %+v", sink.results)
	}
	if result.Calls[0].Name != "document_query" || result.Calls[1].Name != "trace_node_query" {
		t.Fatalf("unexpected round call order: %+v", result.Calls)
	}
}

func TestAgentLoopParallelToolCallsExecuteConcurrently(t *testing.T) {
	registry := NewRegistry()
	var inFlight int32
	var maxInFlight int32
	recordConcurrency := func() {
		current := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			observed := atomic.LoadInt32(&maxInFlight)
			if current <= observed || atomic.CompareAndSwapInt32(&maxInFlight, observed, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result:   Result{Name: "document_query", Status: CallStatusSuccess, Summary: "matched doc-1"},
		onInvoke: recordConcurrency,
	})
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:        "trace_node_query",
			Description: "query trace",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "traceId", Type: ParamTypeString, Required: true}},
		},
		result:   Result{Name: "trace_node_query", Status: CallStatusSuccess, Summary: "matched trace-1"},
		onInvoke: recordConcurrency,
	})

	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{
				{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}},
				{Name: "trace_node_query", Arguments: map[string]any{"traceId": "trace-1"}},
			},
		}},
	}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetParallelToolCalls(true, 2)

	if _, err := loop.Run(context.Background(), WorkflowInput{
		Question: "帮我同时看 doc-1 和 trace-1",
		TraceID:  "trace-1",
	}); err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if atomic.LoadInt32(&maxInFlight) < 2 {
		t.Fatalf("expected parallel execution to reach concurrency >= 2, got %d", maxInFlight)
	}
}

func TestAgentLoopParallelToolCallsImproveWallClockDuration(t *testing.T) {
	buildLoop := func(parallel bool) *ragruntime.AgentLoop {
		registry := NewRegistry()
		registry.MustRegister(staticTool{
			definition: Definition{
				Name:        "document_query",
				Description: "query document",
				ReadOnly:    true,
				Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
			},
			result: Result{Name: "document_query", Status: CallStatusSuccess, Summary: "matched doc-1"},
			delay:  40 * time.Millisecond,
		})
		registry.MustRegister(staticTool{
			definition: Definition{
				Name:        "trace_node_query",
				Description: "query trace",
				ReadOnly:    true,
				Parameters:  []ParameterDefinition{{Name: "traceId", Type: ParamTypeString, Required: true}},
			},
			result: Result{Name: "trace_node_query", Status: CallStatusSuccess, Summary: "matched trace-1"},
			delay:  40 * time.Millisecond,
		})
		planner := &plannerStub{
			results: []PlanResult{{
				Calls: []Call{
					{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}},
					{Name: "trace_node_query", Arguments: map[string]any{"traceId": "trace-1"}},
				},
			}},
		}
		loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
		loop.SetPlanner(planner)
		loop.SetParallelToolCalls(parallel, 2)
		return loop
	}

	serialLoop := buildLoop(false)
	serialStartedAt := time.Now()
	serialResult, err := serialLoop.Run(context.Background(), WorkflowInput{
		Question: "帮我同时看 doc-1 和 trace-1",
		TraceID:  "trace-1",
	})
	if err != nil {
		t.Fatalf("run serial loop: %v", err)
	}
	serialDuration := time.Since(serialStartedAt)

	parallelLoop := buildLoop(true)
	parallelStartedAt := time.Now()
	parallelResult, err := parallelLoop.Run(context.Background(), WorkflowInput{
		Question: "帮我同时看 doc-1 和 trace-1",
		TraceID:  "trace-1",
	})
	if err != nil {
		t.Fatalf("run parallel loop: %v", err)
	}
	parallelDuration := time.Since(parallelStartedAt)

	if len(serialResult.Rounds) == 0 || len(parallelResult.Rounds) == 0 {
		t.Fatalf("expected both runs to produce rounds, got serial=%d parallel=%d", len(serialResult.Rounds), len(parallelResult.Rounds))
	}
	if serialResult.Rounds[0].ExecutionMode != "serial" {
		t.Fatalf("expected serial execution mode, got %q", serialResult.Rounds[0].ExecutionMode)
	}
	if parallelResult.Rounds[0].ExecutionMode != "parallel" {
		t.Fatalf("expected parallel execution mode, got %q", parallelResult.Rounds[0].ExecutionMode)
	}
	if parallelResult.Rounds[0].WallClockDurationMs >= serialResult.Rounds[0].WallClockDurationMs {
		t.Fatalf("expected parallel wall clock duration to be lower, got serial=%d parallel=%d", serialResult.Rounds[0].WallClockDurationMs, parallelResult.Rounds[0].WallClockDurationMs)
	}
	t.Logf(
		"serial=%s (round wall=%dms totalTool=%dms), parallel=%s (round wall=%dms totalTool=%dms)",
		serialDuration,
		serialResult.Rounds[0].WallClockDurationMs,
		serialResult.Rounds[0].TotalToolDurationMs,
		parallelDuration,
		parallelResult.Rounds[0].WallClockDurationMs,
		parallelResult.Rounds[0].TotalToolDurationMs,
	)
	if parallelDuration >= serialDuration {
		t.Fatalf("expected parallel run to finish faster, got serial=%s parallel=%s", serialDuration, parallelDuration)
	}
	if parallelDuration > serialDuration-(15*time.Millisecond) {
		t.Fatalf("expected parallel run to save noticeable time, got serial=%s parallel=%s", serialDuration, parallelDuration)
	}
}

func TestAgentLoopDependencyLevelsOrdering(t *testing.T) {
	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "web_search",
			Description: "search the web",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "query", Type: ParamTypeString, Required: true}},
		},
		result: Result{Name: "web_search", Status: CallStatusSuccess, Summary: "found 3 results",
			Data: map[string]any{"urls": []string{"https://a.com", "https://b.com"}}},
		delay: 40 * time.Millisecond,
	})
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "web_fetch",
			Description: "fetch web pages",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "urls", Type: ParamTypeArray, Required: true}},
		},
		result: Result{Name: "web_fetch", Status: CallStatusSuccess, Summary: "fetched 2 pages"},
		delay:  40 * time.Millisecond,
	})
	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{
				{Name: "web_search", Arguments: map[string]any{"query": "error X"}},
				{Name: "web_fetch", Arguments: map[string]any{"urls": []string{"https://a.com"}}},
			},
		}},
	}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetParallelToolCalls(true, 2)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "search and fetch about error X",
		TraceID:  "trace-1",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) == 0 {
		t.Fatal("expected at least 1 round")
	}
	mode := result.Rounds[0].ExecutionMode
	if !strings.Contains(mode, "parallel_levels=") {
		t.Fatalf("expected dependency-ordered parallel mode, got %q", mode)
	}
	if result.Rounds[0].WallClockDurationMs < 70 {
		t.Fatalf("expected wall clock >= ~80ms (serialized by dependency levels), got %dms", result.Rounds[0].WallClockDurationMs)
	}
	t.Logf("dependency levels mode=%s wall=%dms totalTool=%dms", mode, result.Rounds[0].WallClockDurationMs, result.Rounds[0].TotalToolDurationMs)
}

func TestAgentLoopDependencyLevelsPreservesParallelism(t *testing.T) {
	var maxInFlight int32
	var currentInFlight int32
	recordConcurrency := func() {
		n := atomic.AddInt32(&currentInFlight, 1)
		if n > atomic.LoadInt32(&maxInFlight) {
			atomic.StoreInt32(&maxInFlight, n)
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&currentInFlight, -1)
	}

	registry := NewRegistry()
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result:   Result{Name: "document_query", Status: CallStatusSuccess, Summary: "found doc-1"},
		onInvoke: recordConcurrency,
	})
	registerKnownTestTool(registry, staticTool{
		definition: Definition{
			Name:        "trace_node_query",
			Description: "query trace",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "traceId", Type: ParamTypeString, Required: true}},
		},
		result:   Result{Name: "trace_node_query", Status: CallStatusSuccess, Summary: "found trace-1"},
		onInvoke: recordConcurrency,
	})
	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{
				{Name: "document_query", Arguments: map[string]any{"documentId": "doc-1"}},
				{Name: "trace_node_query", Arguments: map[string]any{"traceId": "trace-1"}},
			},
		}},
	}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetParallelToolCalls(true, 2)

	if _, err := loop.Run(context.Background(), WorkflowInput{
		Question: "check doc-1 and trace-1",
		TraceID:  "trace-1",
	}); err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if atomic.LoadInt32(&maxInFlight) < 2 {
		t.Fatalf("expected parallel execution (independent tools have no deps), got maxInFlight=%d", maxInFlight)
	}
	t.Logf("independent tools ran in parallel, maxInFlight=%d", maxInFlight)
}

func TestAgentLoopDependencyLevelsCycleFallback(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(ToolModule{
		Name:    "tool_a",
		Invoker: testInvoker{},
		Spec: ToolSpec{
			Definition: Definition{Name: "tool_a", ReadOnly: true},
			ReadOnly:   true,
			After:      []string{"tool_b"},
		},
	}.Normalize())
	registry.MustRegisterModule(ToolModule{
		Name:    "tool_b",
		Invoker: testInvoker{},
		Spec: ToolSpec{
			Definition: Definition{Name: "tool_b", ReadOnly: true},
			ReadOnly:   true,
			After:      []string{"tool_a"},
		},
	}.Normalize())

	planner := &plannerStub{
		results: []PlanResult{{
			Calls: []Call{
				{Name: "tool_a", Arguments: map[string]any{}},
				{Name: "tool_b", Arguments: map[string]any{}},
			},
		}},
	}
	loop := ragruntime.NewAgentLoop(ragruntime.NewExecutor(registry))
	loop.SetPlanner(planner)
	loop.SetParallelToolCalls(true, 2)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "test cycle",
		TraceID:  "trace-1",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Rounds) == 0 {
		t.Fatal("expected at least 1 round despite cycle")
	}
	if strings.Contains(result.Rounds[0].ExecutionMode, "parallel_levels=") {
		t.Fatalf("expected flat parallel fallback for cycle, got %q", result.Rounds[0].ExecutionMode)
	}
	t.Logf("cycle fallback: mode=%s", result.Rounds[0].ExecutionMode)
}
