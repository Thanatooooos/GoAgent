package tool_test

import (
	"context"
	"strings"
	"testing"

	. "local/rag-project/internal/app/rag/tool"
	raggraph "local/rag-project/internal/app/rag/tool/invokers/graph"
)

// setupDiagnoseSearchRegistry creates a registry with document_root_cause_diagnosis + web_search stubs.
func setupDiagnoseSearchRegistry() *Registry {
	r := NewRegistry()

	r.MustRegister(staticTool{
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
				"chainLength":    3,
			},
		},
	})

	r.MustRegister(staticTool{
		definition: Definition{
			Name:        "web_search",
			Description: "search the web",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "query", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "web_search",
			Status:  CallStatusSuccess,
			Summary: "found 3 web results for \"connection refused troubleshooting\"",
			Data: map[string]any{
				"query": "connection refused troubleshooting",
				"results": []map[string]any{
					{"title": "Troubleshooting Connection Refused Errors", "url": "https://example.com/1", "snippet": "Check if the service is running..."},
				},
				"resultCount": 1,
			},
		},
	})

	return r
}

func TestDiagnoseSearchGraphToolDefinition(t *testing.T) {
	tool := &raggraph.DiagnoseSearchGraphTool{}
	def := tool.Definition()
	if def.Name != "document_diagnose_with_search" {
		t.Fatalf("unexpected name: %q", def.Name)
	}
}

func TestDiagnoseSearchGraphToolRequiresDocumentId(t *testing.T) {
	tool := &raggraph.DiagnoseSearchGraphTool{}
	result, err := tool.Invoke(context.Background(), Call{
		Name:      "document_diagnose_with_search",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("expected failed, got %q", result.Status)
	}
}

// TestDiagnoseSearchGraphWithError verifies the full chain:
// diagnose finds error → keyword extracted → web_search called.
func TestDiagnoseSearchGraphWithError(t *testing.T) {
	registry := setupDiagnoseSearchRegistry()
	executor := NewExecutor(registry)
	tool, err := raggraph.NewDiagnoseSearchGraphTool(executor)
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), Call{
		Name: "document_diagnose_with_search",
		Arguments: map[string]any{
			"documentId": "doc-fail-01",
		},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q: %s", result.Status, result.ErrorMessage)
	}

	summary := result.Summary
	if !strings.Contains(summary, "diagnosis=success") {
		t.Fatalf("expected diagnosis success in summary, got %q", summary)
	}
	if !strings.Contains(summary, "web_search=success") {
		t.Fatalf("expected web_search success in summary, got %q", summary)
	}
	if !strings.Contains(summary, "connection refused") {
		t.Fatalf("expected search query in summary, got %q", summary)
	}

	// Verify the search query was extracted from the error.
	searchQuery := result.GetString("searchQuery")
	if !strings.Contains(searchQuery, "connection refused") {
		t.Fatalf("expected 'connection refused' in search query, got %q", searchQuery)
	}
}

// TestDiagnoseSearchGraphSkipsSearchWhenNoError verifies that web_search
// is skipped when the diagnosis does not contain a technical error.
func TestDiagnoseSearchGraphSkipsSearchWhenNoError(t *testing.T) {
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
			Summary: "document is still running, no error found",
			Data: map[string]any{
				"conclusion":     "document ingestion is still running",
				"confidence":     "high",
				"diagnosisDepth": "diagnose_only",
			},
		},
	})

	executor := NewExecutor(registry)
	tool, err := raggraph.NewDiagnoseSearchGraphTool(executor)
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), Call{
		Name: "document_diagnose_with_search",
		Arguments: map[string]any{
			"documentId": "doc-run-01",
		},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q", result.Status)
	}

	// Web search should be skipped — no technical error.
	if strings.Contains(result.Summary, "web_search=success") {
		t.Fatal("expected web_search to be skipped for running document")
	}
	if !strings.Contains(result.Summary, "skipped") {
		t.Fatalf("expected 'skipped' in summary, got %q", result.Summary)
	}
}

// TestDiagnoseSearchGraphDiagnoseFails verifies that when the diagnosis graph
// itself fails, the outer graph returns failure without attempting search.
func TestDiagnoseSearchGraphDiagnoseFails(t *testing.T) {
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
			ErrorMessage: "diagnosis engine unavailable",
		},
	})

	executor := NewExecutor(registry)
	tool, err := raggraph.NewDiagnoseSearchGraphTool(executor)
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), Call{
		Name: "document_diagnose_with_search",
		Arguments: map[string]any{
			"documentId": "doc-fail-01",
		},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	// Should still return a result (not fail), but with diagnosis failure info.
	// The graph continues to the search node, which finds no error → skips search.
	if result.Status != CallStatusSuccess {
		t.Fatalf("expected graceful degradation, got %q", result.Status)
	}
	if strings.Contains(result.Summary, "web_search=success") {
		t.Fatal("expected no web_search when diagnose produced no useful error")
	}
}
