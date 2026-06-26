package chat

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

func TestApplyToolContextBudgetRetainsTruncationStats(t *testing.T) {
	service := &RagChatService{
		chatContextBudget: ChatContextBudgetOptions{
			ToolTokens: 20,
			Estimator:  tokenbudget.RuneEstimator{},
		},
	}
	result := service.applyToolContextBudget(context.Background(), "", ragChatToolStageResult{
		result: ragtool.WorkflowResult{Context: strings.Repeat("x", 80)},
	})
	if !result.result.ContextBudget.Truncated {
		t.Fatalf("context budget stats = %+v, want truncated", result.result.ContextBudget)
	}
	if result.result.ContextBudget.TokensAfter > 20 {
		t.Fatalf("tokens after = %d, want <= 20", result.result.ContextBudget.TokensAfter)
	}
}
