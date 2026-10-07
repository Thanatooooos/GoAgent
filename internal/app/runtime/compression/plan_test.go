package compression

import (
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
)

func TestSplitRetainsWholeNewestTurns(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "one"}, {Role: "assistant", Content: "one"},
		{Role: "user", Content: "two"}, {Role: "assistant", Content: "two"},
	}
	plan := Split(history, 2, tokenbudget.FixedEstimator(1))
	if !plan.NeedsSummary || len(plan.Past) != 2 || len(plan.Recent) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Recent[0].Content != "two" {
		t.Fatalf("recent = %+v", plan.Recent)
	}
}
