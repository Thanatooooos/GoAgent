package tool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type workflowToolStub struct {
	definition Definition
	result     Result
	err        error
}

func (s workflowToolStub) Definition() Definition {
	return s.definition
}

func (s workflowToolStub) Invoke(ctx context.Context, call Call) (Result, error) {
	return s.result, s.err
}

func TestLocalWorkflowRunsPlannedCalls(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "document_query"},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "document doc-1 status=success",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "trace_node_query"},
		result: Result{
			Name:    "trace_node_query",
			Status:  CallStatusSuccess,
			Summary: "trace trace-1 nodes=2",
		},
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "show me document doc-1 and trace trace-1 status",
	})
	if err != nil {
		t.Fatalf("run local workflow: %v", err)
	}
	if !result.Used {
		t.Fatal("expected workflow to be used")
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(result.Calls))
	}
	if !strings.Contains(result.Context, "document doc-1") {
		t.Fatalf("unexpected context: %q", result.Context)
	}
}

func TestLocalWorkflowUsesCurrentTraceIDFallback(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "trace_node_query"},
		result: Result{
			Name:    "trace_node_query",
			Status:  CallStatusSuccess,
			Summary: "trace trace-current nodes=1",
		},
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "show current trace retrieval chain",
		TraceID:  "trace-current",
	})
	if err != nil {
		t.Fatalf("run local workflow with fallback trace: %v", err)
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "trace_node_query" {
		t.Fatalf("unexpected tool name: %q", result.Calls[0].Name)
	}
}

func TestLocalWorkflowDegradesWhenToolFails(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "ingestion_task_query"},
		result: Result{
			Name:         "ingestion_task_query",
			Status:       CallStatusFailed,
			ErrorMessage: "backend unavailable",
		},
		err: errors.New("backend unavailable"),
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "why did ingestion task task-1 fail",
	})
	if err != nil {
		t.Fatalf("run local workflow with failure: %v", err)
	}
	if !result.Degraded {
		t.Fatal("expected workflow to degrade")
	}
	if !strings.Contains(result.DegradeReason, "backend unavailable") {
		t.Fatalf("unexpected degrade reason: %q", result.DegradeReason)
	}
}

func TestLocalWorkflowSkipsWhenNoPlanMatches(t *testing.T) {
	workflow := NewLocalWorkflow(NewExecutor(NewRegistry()))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "summarize this issue for me",
	})
	if err != nil {
		t.Fatalf("run local workflow without plan: %v", err)
	}
	if result.Used {
		t.Fatal("expected workflow to skip tool usage")
	}
}

func TestLocalWorkflowPlansDocumentChunkLogQuery(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "document_ingestion_diagnose"},
		result: Result{
			Name:    "document_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "document=doc-1 confidence=high conclusion=document ingestion failed at node indexer",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "document_chunk_log_query"},
		result: Result{
			Name:    "document_chunk_log_query",
			Status:  CallStatusSuccess,
			Summary: "document=doc-1 chunkLogs=1 latestStatus=failed",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "document_query"},
		result: Result{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "document doc-1 status=failed",
		},
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "please diagnose document doc-1 chunk log ingestion failure",
	})
	if err != nil {
		t.Fatalf("run local workflow with document chunk log query: %v", err)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("unexpected first tool name: %q", result.Calls[0].Name)
	}
	if result.Calls[1].Name != "document_chunk_log_query" {
		t.Fatalf("unexpected second tool name: %q", result.Calls[1].Name)
	}
}

func TestLocalWorkflowPlansTaskIngestionDiagnose(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "task_ingestion_diagnose"},
		result: Result{
			Name:    "task_ingestion_diagnose",
			Status:  CallStatusSuccess,
			Summary: "task=task-1 confidence=high conclusion=ingestion task failed at node indexer",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "ingestion_task_node_query"},
		result: Result{
			Name:    "ingestion_task_node_query",
			Status:  CallStatusSuccess,
			Summary: "task=task-1 totalNodes=3 failed=[indexer(connection refused)]",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "ingestion_task_query"},
		result: Result{
			Name:    "ingestion_task_query",
			Status:  CallStatusSuccess,
			Summary: "ingestion task task-1 status=failed pipelineId=pipe-1 sourceType=file nodes=3",
		},
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "please diagnose why ingestion task task-1 failed at which node",
	})
	if err != nil {
		t.Fatalf("run local workflow with task diagnosis: %v", err)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "task_ingestion_diagnose" {
		t.Fatalf("unexpected first tool name: %q", result.Calls[0].Name)
	}
}

func TestLocalWorkflowPlansTraceRetrievalDiagnose(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "trace_retrieval_diagnose"},
		result: Result{
			Name:    "trace_retrieval_diagnose",
			Status:  CallStatusSuccess,
			Summary: "trace=trace-1 confidence=high conclusion=trace retrieval returned no chunks",
		},
	})
	registry.MustRegister(workflowToolStub{
		definition: Definition{Name: "trace_node_query"},
		result: Result{
			Name:    "trace_node_query",
			Status:  CallStatusSuccess,
			Summary: "trace trace-1 status=success conversationId=conv-1 nodes=3",
		},
	})

	workflow := NewLocalWorkflow(NewExecutor(registry))
	result, err := workflow.Run(context.Background(), WorkflowInput{
		Question: "please diagnose why trace trace-1 retrieval was poor",
	})
	if err != nil {
		t.Fatalf("run local workflow with trace diagnosis: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(result.Calls))
	}
	if result.Calls[0].Name != "trace_retrieval_diagnose" {
		t.Fatalf("unexpected first tool name: %q", result.Calls[0].Name)
	}
}
