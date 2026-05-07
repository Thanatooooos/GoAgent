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
				"evidence":    []string{"document.status=failed", "failedNode=indexer"},
				"suggestions": []string{"check vector store connectivity"},
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
}
