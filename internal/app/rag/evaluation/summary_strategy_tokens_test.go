package evaluation

import (
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/app/rag/service/sessionrecall"
)

func TestEstimateSummaryMessagesTokensAddsMessageOverhead(t *testing.T) {
	got := estimateSummaryMessagesTokens(
		[]SummaryMessage{
			{Role: "user", Content: "a"},
			{Role: "assistant", Content: "b"},
		},
		tokenbudget.FixedEstimator(10),
		4,
	)
	if got != 28 {
		t.Fatalf("tokens = %d, want 28", got)
	}
}

func TestEstimateStrategyTokenUsageUsesSameOverheadForBaselineAndTail(t *testing.T) {
	usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
		FullMessages: []SummaryMessage{
			{Role: "user", Content: "a"},
			{Role: "assistant", Content: "b"},
		},
		SummaryText:           "summary",
		TailMessages:          []SummaryMessage{{Role: "assistant", Content: "b"}},
		Estimator:             tokenbudget.FixedEstimator(10),
		MessageOverheadTokens: 4,
	})
	if usage.BaselineTokens != 28 {
		t.Fatalf("BaselineTokens = %d, want 28", usage.BaselineTokens)
	}
	if usage.StrategyTokens != 24 {
		t.Fatalf("StrategyTokens = %d, want 24", usage.StrategyTokens)
	}
}

func TestEstimateStrategyTokenUsageCountsSummaryPlusTail(t *testing.T) {
	estimator := sessionrecall.RoughTokenEstimator{}
	usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
		FullMessages: []SummaryMessage{
			{Role: "user", Content: "discuss summary strategy mode"},
			{Role: "assistant", Content: "we should compare token savings and quality"},
			{Role: "user", Content: "continue with threshold sweep"},
			{Role: "assistant", Content: "need to keep downstream quality stable"},
		},
		SummaryText: "Goal: finish summary strategy mode",
		TailMessages: []SummaryMessage{
			{Role: "user", Content: "continue with threshold sweep"},
			{Role: "assistant", Content: "need to keep downstream quality stable"},
		},
		Estimator:             estimator,
		MessageOverheadTokens: 4,
	})
	if usage.StrategyTokens <= 0 {
		t.Fatalf("StrategyTokens = %d, want > 0", usage.StrategyTokens)
	}
	if usage.BaselineTokens <= usage.StrategyTokens {
		t.Fatalf("BaselineTokens = %d, StrategyTokens = %d, want baseline > strategy", usage.BaselineTokens, usage.StrategyTokens)
	}
}
