package tool

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestAgentLoopRunsMultipleRounds(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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

	loop := NewAgentLoop(NewExecutor(registry))
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
	if planner.inputs[1].AgentState.Empty() {
		t.Fatal("expected second planner call to receive agent state")
	}
	if len(planner.inputs[1].PreviousResults) != 1 {
		t.Fatalf("expected second planner call to receive previous results, got %d", len(planner.inputs[1].PreviousResults))
	}
	if len(planner.inputs[1].AgentState.NextHintCalls) != 1 {
		t.Fatalf("expected structured hint calls, got %+v", planner.inputs[1].AgentState.NextHintCalls)
	}
	if planner.inputs[1].AgentState.NextHint != "tool:ingestion_task_node_query|taskId=task-1|nodeId=indexer" {
		t.Fatalf("expected structured agent state hint, got %q", planner.inputs[1].AgentState.NextHint)
	}
	if planner.inputs[1].AgentState.NextHintCalls[0].Name != "ingestion_task_node_query" {
		t.Fatalf("unexpected hint call name: %+v", planner.inputs[1].AgentState.NextHintCalls[0])
	}
	if len(result.Rounds) < 1 || !strings.HasPrefix(result.Rounds[0].NextHint, "tool:ingestion_task_node_query|") {
		t.Fatalf("expected structured next hint, got %q", result.Rounds[0].NextHint)
	}
	if len(result.Rounds[0].NextHintCalls) != 1 {
		t.Fatalf("expected round to preserve structured hint calls, got %+v", result.Rounds[0].NextHintCalls)
	}
}

func TestPlanCallsFromHintParsesStructuredHint(t *testing.T) {
	calls := planCallsFromHint("tool:ingestion_task_query|taskId=task-1|includeNodes=true", []Definition{{
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
	calls := planCallsFromHintCalls([]HintCall{{
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
	calls := planCallsFromResults([]Result{
		{
			Name: "document_query",
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
			},
		},
	})
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
	calls := planWithBaseRules(WorkflowInput{
		Question: "document doc_run_01 现在还在运行吗？",
	}, defaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected first call to be document_ingestion_diagnose, got %q", calls[0].Name)
	}
}

func TestPlanWithBaseRulesUsesDocumentDiagnosisForCurrentNodeQuestion(t *testing.T) {
	calls := planWithBaseRules(WorkflowInput{
		Question: "帮我看看 doc_run_01 现在跑到哪个节点了",
	}, defaultMaxIterations)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected first call to be document_ingestion_diagnose, got %q", calls[0].Name)
	}
}

func TestPlanWithBaseRulesOpenEndedDocumentsFailed(t *testing.T) {
	calls := planWithBaseRules(WorkflowInput{
		Question:         "最近有哪些文档导入失败了？",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, defaultMaxIterations)
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
	calls := planWithBaseRules(WorkflowInput{
		Question: "哪些ingestion任务还在运行中？",
	}, defaultMaxIterations)
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
	calls := planWithBaseRules(WorkflowInput{
		Question: "最近有哪些文档导入失败了？",
	}, defaultMaxIterations)
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
	calls := planWithBaseRules(WorkflowInput{
		Question: "doc_fail_01 这个失败的文档什么情况？",
	}, defaultMaxIterations)
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
	calls := planCallsFromResults([]Result{
		{
			Name: "document_chunk_log_query",
			Data: map[string]any{
				"documentId":      "doc-1",
				"latestTaskId":    "task-1",
				"latestStatus":    "running",
				"runningLogCount": 1,
			},
		},
	})
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
	calls := planCallsFromResults([]Result{
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
	})
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

func TestObserveDocumentDiagnosisKeepsDrillingWhenOnlyLogErrorExists(t *testing.T) {
	observation := observeDocumentDiagnosis(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":     "document ingestion failed, but no failed node was captured",
			"confidence":     "medium",
			"latestTaskId":   "task-1",
			"latestLogError": "indexer failed after retries",
		},
	})
	if observation.Done {
		t.Fatal("expected observer to continue when only task/log-level error exists")
	}
	if observation.NextHint != "tool:ingestion_task_query|taskId=task-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.NextHint)
	}
}

func TestObserveDocumentDiagnosisStopsOnNodeLevelError(t *testing.T) {
	observation := observeDocumentDiagnosis(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":      "document ingestion failed at node indexer",
			"confidence":      "high",
			"latestTaskId":    "task-1",
			"latestNodeId":    "indexer",
			"latestNodeError": "connection refused: vector store unavailable",
		},
	})
	if !observation.Done {
		t.Fatal("expected observer to stop when node-level error already exists")
	}
}

func TestPlanCallsFromHintCallsSupportsNewToolDefinitionsWithoutHardcodedSwitch(t *testing.T) {
	calls := planCallsFromHintCalls([]HintCall{{
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
	observation := observeDocumentDiagnosis(Result{
		Name: "document_ingestion_diagnose",
		Data: map[string]any{
			"conclusion":   "document ingestion task is still running",
			"confidence":   "medium",
			"latestTaskId": "task-run-1",
		},
	})
	if observation.Done {
		t.Fatal("expected running document diagnosis to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if observation.NextHint != "tool:ingestion_task_query|taskId=task-run-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.NextHint)
	}
}

func TestObserveTaskDiagnosisRunningKeepsVerifyingTaskState(t *testing.T) {
	observation := observeTaskDiagnosis(Result{
		Name: "task_ingestion_diagnose",
		Data: map[string]any{
			"taskId":     "task-run-1",
			"conclusion": "task is still running and no failed node has been confirmed yet",
			"confidence": "medium",
		},
	})
	if observation.Done {
		t.Fatal("expected running task diagnosis to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if observation.NextHint != "tool:ingestion_task_query|taskId=task-run-1|includeNodes=true" {
		t.Fatalf("unexpected next hint: %q", observation.NextHint)
	}
}

func TestObserveTaskQueryRunningNodeUsesVerificationInsteadOfFailureDrilldown(t *testing.T) {
	observation := observeTaskQuery(Result{
		Name: "ingestion_task_query",
		Data: map[string]any{
			"taskId": "task-run-1",
			"status": "running",
			"taskNodeSummary": []map[string]any{
				{"nodeId": "fetcher", "status": "success"},
				{"nodeId": "indexer", "status": "running"},
			},
		},
	})
	if observation.Done {
		t.Fatal("expected running task query to continue")
	}
	if observation.State.Phase != "verification" {
		t.Fatalf("expected verification phase, got %q", observation.State.Phase)
	}
	if !strings.Contains(observation.State.Hypothesis, "still running") {
		t.Fatalf("expected running hypothesis, got %q", observation.State.Hypothesis)
	}
	if observation.NextHint != "tool:ingestion_task_node_query|taskId=task-run-1|nodeId=indexer" {
		t.Fatalf("unexpected next hint: %q", observation.NextHint)
	}
}

func TestAgentLoopDocRunScenarioStaysInVerificationPath(t *testing.T) {
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
			Summary: "document doc-run-1 is running in pipeline mode",
			Data: map[string]any{
				"documentId":  "doc-run-1",
				"status":      "running",
				"processMode": "pipeline",
			},
		},
	})
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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

	loop := NewAgentLoop(NewExecutor(registry))
	loop.SetMaxIterations(4)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "document doc-run-1 当前还在运行吗？",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "document_ingestion_diagnose" ||
		result.Calls[1].Name != "ingestion_task_query" ||
		result.Calls[2].Name != "ingestion_task_node_query" {
		t.Fatalf("unexpected call order: %+v", result.Calls)
	}
	if len(result.Rounds) < 3 {
		t.Fatalf("expected at least 3 rounds, got %d", len(result.Rounds))
	}
	if result.Rounds[0].State.Phase != "verification" {
		t.Fatalf("expected first round to enter verification, got %q", result.Rounds[0].State.Phase)
	}
	if result.Rounds[1].State.Phase != "verification" {
		t.Fatalf("expected task query round to stay in verification, got %q", result.Rounds[1].State.Phase)
	}
	if strings.Contains(strings.ToLower(result.Rounds[1].State.Hypothesis), "failed") {
		t.Fatalf("expected running scenario hypothesis to avoid failed wording, got %q", result.Rounds[1].State.Hypothesis)
	}
}

func TestAgentLoopTaskRunScenarioStaysInVerificationPath(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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

	loop := NewAgentLoop(NewExecutor(registry))

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
	registry.MustRegister(staticTool{
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
			Summary: "document=doc-1 status=failed processMode=pipeline",
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
			},
		},
	})
	registry.MustRegister(staticTool{
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
	loop := NewAgentLoop(NewExecutor(registry))
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
	// Second planner should see think's thought in previous results.
	if len(planner.inputs) != 2 {
		t.Fatalf("expected 2 planner calls, got %d", len(planner.inputs))
	}
	if len(planner.inputs[1].PreviousResults) != 2 {
		t.Fatalf("expected round 2 planner to see 2 previous results, got %d", len(planner.inputs[1].PreviousResults))
	}
	if planner.inputs[1].PreviousResults[0].Name != "think" {
		t.Fatalf("expected planner to see think as first previous result, got %q", planner.inputs[1].PreviousResults[0].Name)
	}
}

func TestAgentLoopRejectsPlannerCallWithInventedNodeID(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
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
	registry.MustRegister(staticTool{
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
	loop := NewAgentLoop(NewExecutor(registry))
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
	loop := NewAgentLoop(NewExecutor(registry))
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
	loop := NewAgentLoop(NewExecutor(registry))
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
	buildLoop := func(parallel bool) *AgentLoop {
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
		loop := NewAgentLoop(NewExecutor(registry))
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
