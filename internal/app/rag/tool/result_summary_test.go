package tool

import (
	"strings"
	"testing"
)

func TestSummarizeResultDataForLLMIncludesDiagnosticFields(t *testing.T) {
	summary := SummarizeResultDataForLLM(map[string]any{
		"documentId":     "doc-1",
		"conclusion":     "document failed at node indexer",
		"suggestions":    []string{"check vector store connectivity", "retry after recovery"},
		"riskHints":      []string{"retries may keep failing"},
		"facts":          []any{"latest task is task-1", "failed node is indexer"},
		"inferences":     []string{"vector store unavailable is the most likely root cause"},
		"nextActions":    []string{"inspect vector store health"},
		"diagnosisScope": "document",
	})

	for _, part := range []string{
		"documentId=doc-1",
		"conclusion=document failed at node indexer",
		"suggestions=check vector store connectivity",
		"riskHints=retries may keep failing",
		"facts=latest task is task-1",
		"inferences=vector store unavailable is the most likely root cause",
		"nextActions=inspect vector store health",
		"diagnosisScope=document",
	} {
		if !strings.Contains(summary, part) {
			t.Fatalf("expected summary to contain %q, got %q", part, summary)
		}
	}
}

func TestSummarizeResultDataForLLMIncludesTaskNodeSummary(t *testing.T) {
	summary := SummarizeResultDataForLLM(map[string]any{
		"taskId": "task-1",
		"taskNodeSummary": []map[string]any{
			{"nodeId": "fetcher", "status": "success"},
			{"nodeId": "indexer", "status": "failed", "nodeType": "indexer", "errorMessage": "connection refused: vector store unavailable"},
		},
	})

	if !strings.Contains(summary, "taskNodeSummary=fetcher|status=success; indexer|status=failed|type=indexer|error=connection refused: vector store unavailable") {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestSummarizeResultDataForLLMIncludesUnknownUsefulFieldsButSkipsNoise(t *testing.T) {
	summary := SummarizeResultDataForLLM(map[string]any{
		"taskId":    "task-1",
		"rootCause": "vector store unavailable",
		"rawBody":   `{"huge":"payload"}`,
		"fullText":  "very long original text that should not be summarized directly",
	})

	if !strings.Contains(summary, "rootCause=vector store unavailable") {
		t.Fatalf("expected summary to include unknown useful field, got %q", summary)
	}
	if strings.Contains(summary, "rawBody=") {
		t.Fatalf("expected summary to skip rawBody noise field, got %q", summary)
	}
	if strings.Contains(summary, "fullText=") {
		t.Fatalf("expected summary to skip fullText noise field, got %q", summary)
	}
}
