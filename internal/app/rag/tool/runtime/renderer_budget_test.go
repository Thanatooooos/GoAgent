package runtime

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	. "local/rag-project/internal/app/rag/tool/core"
)

func TestRenderContextWithinBudgetKeepsConclusionWithinTokenLimit(t *testing.T) {
	results := []Result{
		{Name: "diagnose", Status: CallStatusSuccess, Summary: "root cause confirmed"},
		{Name: "detail", Status: CallStatusSuccess, Summary: strings.Repeat("verbose ", 50)},
	}

	rendered, stats := RenderContextWithinBudget(results, 40, tokenbudget.RuneEstimator{})
	if !strings.Contains(rendered, "root cause confirmed") {
		t.Fatalf("rendered context missing conclusion: %q", rendered)
	}
	if tokens := (tokenbudget.RuneEstimator{}).EstimateTokens(rendered); tokens > 40 {
		t.Fatalf("rendered tokens = %d, want <= 40; context=%q", tokens, rendered)
	}
	if !stats.Truncated {
		t.Fatalf("stats = %+v, want truncated", stats)
	}
}
