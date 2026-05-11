package tool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type toolStub struct {
	definition Definition
	result     Result
	err        error
	lastCall   Call
}

func TestBuildAnswerGuidancePrefersDeeperNodeEvidence(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":   "document ingestion failed, but no failed node was captured",
				"confidence":   "medium",
				"facts":        []string{"文档当前状态为失败。", "最近一次关联任务为 task_fail_01。"},
				"inferences":   []string{"推断：失败发生在某个尚未被完整记录的阶段。"},
				"nextActions":  []string{"check task log"},
				"latestTaskId": "task_fail_01",
			},
		},
		{
			Name:   "ingestion_task_node_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId":       "task_fail_01",
				"nodeId":       "indexer",
				"nodeOrder":    4,
				"status":       "failed",
				"durationMs":   5210,
				"errorMessage": "connection refused: vector store unavailable",
			},
		},
	})
	if !strings.Contains(guidance, "失败发生在 indexer 节点") {
		t.Fatalf("expected upgraded conclusion from node evidence, got %q", guidance)
	}
	if !strings.Contains(guidance, "connection refused: vector store unavailable") {
		t.Fatalf("expected node error to appear in guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前置信度：high") {
		t.Fatalf("expected confidence to be upgraded to high, got %q", guidance)
	}
	if strings.Contains(guidance, "尚未被完整记录") {
		t.Fatalf("expected stale inference to be replaced, got %q", guidance)
	}
}

func TestAgentStatePromptStringIncludesStructuredHintCalls(t *testing.T) {
	state := AgentState{
		Phase: "deep_dive",
		NextHintCalls: []HintCall{{
			Name: "ingestion_task_query",
			Arguments: map[string]any{
				"taskId":       "task-1",
				"includeNodes": true,
			},
		}},
	}.Normalize()

	prompt := state.PromptString()
	if !strings.Contains(prompt, "\"nextHintCalls\"") {
		t.Fatalf("expected prompt to include nextHintCalls, got %q", prompt)
	}
	if !strings.Contains(prompt, "\"name\":\"ingestion_task_query\"") {
		t.Fatalf("expected prompt to include hint call name, got %q", prompt)
	}
	if state.NextHint != "tool:ingestion_task_query|taskId=task-1|includeNodes=true" {
		t.Fatalf("expected legacy nextHint to remain available, got %q", state.NextHint)
	}
}

func (s *toolStub) Definition() Definition {
	return s.definition
}

func (s *toolStub) Invoke(ctx context.Context, call Call) (Result, error) {
	s.lastCall = call
	return s.result, s.err
}

func TestDefinitionValidate(t *testing.T) {
	if err := (Definition{}).Validate(); err == nil {
		t.Fatal("expected error for empty tool name")
	}

	err := (Definition{
		Name: "document_query",
		Parameters: []ParameterDefinition{
			{Name: "", Type: ParamTypeString},
		},
	}).Validate()
	if err == nil {
		t.Fatal("expected error for empty parameter name")
	}
}

func TestRegistryRegisterAndListDefinitions(t *testing.T) {
	registry := NewRegistry()
	docTool := &toolStub{
		definition: Definition{Name: "document_query", Description: "query document"},
	}
	traceTool := &toolStub{
		definition: Definition{Name: "trace_node_query", Description: "query trace node"},
	}

	if err := registry.Register(traceTool); err != nil {
		t.Fatalf("register trace tool: %v", err)
	}
	if err := registry.Register(docTool); err != nil {
		t.Fatalf("register doc tool: %v", err)
	}
	if err := registry.Register(docTool); err == nil {
		t.Fatal("expected duplicate register error")
	}

	items := registry.ListDefinitions()
	if len(items) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(items))
	}
	if items[0].Name != "document_query" || items[1].Name != "trace_node_query" {
		t.Fatalf("expected sorted definitions, got %+v", items)
	}
}

func TestExecutorExecuteSuccess(t *testing.T) {
	registry := NewRegistry()
	tool := &toolStub{
		definition: Definition{Name: "document_query", Description: "query document"},
		result: Result{
			Summary: "matched doc-1",
			Data: map[string]any{
				"documentId": "doc-1",
			},
		},
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	executor := NewExecutor(registry)
	result, err := executor.Execute(context.Background(), Call{
		Name: "document_query",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if result.Name != "document_query" {
		t.Fatalf("unexpected result name: %q", result.Name)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if tool.lastCall.Arguments["documentId"] != "doc-1" {
		t.Fatalf("unexpected invoke arguments: %+v", tool.lastCall.Arguments)
	}
}

func TestExecutorExecuteFailure(t *testing.T) {
	registry := NewRegistry()
	tool := &toolStub{
		definition: Definition{Name: "trace_node_query", Description: "query trace node"},
		result: Result{
			Summary: "trace lookup failed",
		},
		err: errors.New("repo unavailable"),
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	executor := NewExecutor(registry)
	result, err := executor.Execute(context.Background(), Call{Name: "trace_node_query"})
	if err == nil {
		t.Fatal("expected execution error")
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("unexpected failed status: %q", result.Status)
	}
	if result.ErrorMessage != "repo unavailable" {
		t.Fatalf("unexpected error message: %q", result.ErrorMessage)
	}
}

func TestExecutorExecuteUnknownTool(t *testing.T) {
	executor := NewExecutor(NewRegistry())
	result, err := executor.Execute(context.Background(), Call{Name: "missing_tool"})
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
}

func TestRenderContextAndToCallSummaries(t *testing.T) {
	results := []Result{
		{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "matched doc-1",
		},
		{
			Name:         "trace_node_query",
			Status:       CallStatusFailed,
			ErrorMessage: "trace not found",
		},
	}

	contextText := RenderContext(results)
	if contextText == "" {
		t.Fatal("expected rendered context")
	}
	if summaries := ToCallSummaries(results); len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
}

func TestBuildAnswerGuidanceFromDiagnosisResult(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":  "document ingestion failed at node indexer",
				"confidence":  "high",
				"facts":       []string{"文档当前状态为失败。", "失败节点是 indexer。"},
				"rawEvidence": []string{"document.status=failed", "failedNode=indexer"},
				"inferences":  []string{"document ingestion failed at node indexer"},
				"nextActions": []string{"check vector store connectivity"},
			},
		},
	})
	if guidance == "" {
		t.Fatal("expected non-empty guidance")
	}
	if !strings.Contains(guidance, "结论 / 证据 / 建议") {
		t.Fatalf("unexpected guidance: %q", guidance)
	}
	if !strings.Contains(guidance, "document ingestion failed at node indexer") {
		t.Fatalf("missing diagnosis conclusion: %q", guidance)
	}
	if !strings.Contains(guidance, "推断") {
		t.Fatalf("expected inference boundary in guidance: %q", guidance)
	}
}

func TestBuildAnswerGuidanceResolvesStatusConflictDiagnoseFailedTaskRunning(t *testing.T) {
	// doc_run_01 scenario: document_ingestion_diagnose says failed,
	// but ingestion_task_query and ingestion_task_node_query both show running.
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":   "document ingestion failed at node indexer",
				"confidence":   "high",
				"facts":        []string{"文档当前状态为失败。", "最近一次关联任务为 task_run_01。"},
				"latestTaskId": "task_run_01",
			},
		},
		{
			Name:   "ingestion_task_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId": "task_run_01",
				"status": "running",
			},
		},
		{
			Name:   "ingestion_task_node_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId":       "task_run_01",
				"nodeId":       "indexer",
				"nodeOrder":    4,
				"status":       "running",
				"durationMs":   15200,
				"errorMessage": "",
			},
		},
	})
	if !strings.Contains(guidance, "仍在处理中") {
		t.Fatalf("expected conclusion to override to running state, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前置信度：high") {
		t.Fatalf("expected confidence to be high after conflict resolution, got %q", guidance)
	}
	if !strings.Contains(guidance, "异步更新") {
		t.Fatalf("expected conflict explanation about async state lag, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前建议结论：文档仍在处理中") {
		t.Fatalf("expected conclusion to say document is still processing, got %q", guidance)
	}
	if !strings.Contains(guidance, "状态不一致") {
		t.Fatalf("expected risk hint about status inconsistency, got %q", guidance)
	}
}

func TestFirstMatchedIDRequiresStructuredIdentifiers(t *testing.T) {
	if got := firstMatchedID(documentIDPattern, "document doc_run_01 对应的最新 ingestion task 现在是什么状态？"); got != "doc_run_01" {
		t.Fatalf("expected doc_run_01, got %q", got)
	}
	if got := firstMatchedID(documentIDPattern, "document 当前状态是什么"); got != "" {
		t.Fatalf("expected plain keyword document to not be treated as id, got %q", got)
	}
	if got := firstMatchedID(taskIDPattern, "task task_run_01 当前还在运行吗"); got != "task_run_01" {
		t.Fatalf("expected task_run_01, got %q", got)
	}
	if got := firstMatchedID(taskIDPattern, "task 当前状态是什么"); got != "" {
		t.Fatalf("expected plain keyword task to not be treated as id, got %q", got)
	}
	if got := firstMatchedID(traceIDPattern, "trace trace_bad_01 为什么检索效果差"); got != "trace_bad_01" {
		t.Fatalf("expected trace_bad_01, got %q", got)
	}
	if got := firstMatchedID(traceIDPattern, "trace 当前情况如何"); got != "" {
		t.Fatalf("expected plain keyword trace to not be treated as id, got %q", got)
	}
}
