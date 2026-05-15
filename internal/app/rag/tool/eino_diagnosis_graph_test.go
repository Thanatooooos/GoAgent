package tool_test

import (
	"context"
	"strings"
	"testing"

	. "local/rag-project/internal/app/rag/tool"
	raggraph "local/rag-project/internal/app/rag/tool/invokers/graph"
)

func TestDiagnosisGraphToolDefinition(t *testing.T) {
	tool := &raggraph.DiagnosisGraphTool{}
	def := tool.Definition()
	if def.Name != "document_root_cause_diagnosis" {
		t.Fatalf("unexpected name: %q", def.Name)
	}
	if len(def.Parameters) != 1 || def.Parameters[0].Name != "documentId" {
		t.Fatalf("unexpected parameters: %+v", def.Parameters)
	}
}

func TestDiagnosisGraphToolRequiresExecutor(t *testing.T) {
	_, err := raggraph.NewDiagnosisGraphTool(nil)
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
}

func TestDiagnosisGraphToolRequiresDocumentId(t *testing.T) {
	tool := &raggraph.DiagnosisGraphTool{}
	result, err := tool.Invoke(context.Background(), Call{
		Name:      "document_root_cause_diagnosis",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("expected failed, got %q", result.Status)
	}
}

func TestDiagnosisGraphToolChainExecutes(t *testing.T) {
	registry := NewRegistry()

	// Register the 3 tools the graph needs.
	_ = registry.Register(&stubTool{
		name: "document_ingestion_diagnose",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			return Result{
				Name:    "document_ingestion_diagnose",
				Status:  CallStatusSuccess,
				Summary: "diagnose complete",
				Data: map[string]any{
					"documentId":     "doc-1",
					"latestTaskId":   "task-1",
					"latestLogError": "something went wrong",
					"conclusion":     "document processing failed",
					"confidence":     "medium",
				},
			}, nil
		},
	})
	_ = registry.Register(&stubTool{
		name: "ingestion_task_query",
		invoke: func(ctx context.Context, call Call) (Result, error) {
			return Result{
				Name:    "ingestion_task_query",
				Status:  CallStatusSuccess,
				Summary: "task query complete",
				Data: map[string]any{
					"taskId": "task-1",
					"status": "failed",
					"taskNodeSummary": []map[string]any{
						{"nodeId": "indexer", "status": "failed", "nodeType": "indexer"},
					},
				},
			}, nil
		},
	})
	_ = registry.Register(&stubTool{
		name: "ingestion_task_node_query",
		invoke: func(ctx context.Context, call Call) (Result, error) {
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

	executor := NewExecutor(registry)
	tool, err := raggraph.NewDiagnosisGraphTool(executor)
	if err != nil {
		t.Fatalf("create diagnosis graph tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), Call{
		Name: "document_root_cause_diagnosis",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q: %s", result.Status, result.ErrorMessage)
	}

	chainLength := result.GetInt("chainLength")
	if chainLength != 3 {
		t.Fatalf("expected 3 hops, got %d", chainLength)
	}

	conclusion := result.GetString("conclusion")
	if !strings.Contains(conclusion, "connection refused") {
		t.Fatalf("expected node-level conclusion, got %q", conclusion)
	}

	taskID := result.GetString("latestTaskId")
	if taskID != "task-1" {
		t.Fatalf("expected task-1, got %q", taskID)
	}

	nodeID := result.GetString("latestNodeId")
	if nodeID != "indexer" {
		t.Fatalf("expected indexer, got %q", nodeID)
	}
}

type stubTool struct {
	name   string
	invoke func(ctx context.Context, call Call) (Result, error)
}

func (s *stubTool) Definition() Definition {
	return Definition{Name: s.name, Parameters: []ParameterDefinition{}}
}

func (s *stubTool) Invoke(ctx context.Context, call Call) (Result, error) {
	return s.invoke(ctx, call)
}
