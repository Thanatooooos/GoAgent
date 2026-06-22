package evaluation

import (
	"strings"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestRenderSummarySearchTextIncludesPriorityHierarchyFields(t *testing.T) {
	text := renderSummarySearchText(raghistory.StructuredSummary{
		SchemaVersion: 1,
		Goal:          "起草 summary 样本",
		ActivePriorities: []string{
			"明确 must_cover 和 critical_contract 的边界",
		},
		BackgroundIssues: []string{
			"CI flaky 不是当前重点",
		},
	})

	if !strings.Contains(text, "must_cover") {
		t.Fatalf("expected search text to include active priorities, got %q", text)
	}
	if !strings.Contains(text, "ci flaky") {
		t.Fatalf("expected search text to include background issues, got %q", text)
	}
}
