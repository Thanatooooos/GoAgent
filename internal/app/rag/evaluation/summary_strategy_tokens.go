package evaluation

import (
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
)

type strategyTokenUsageInput struct {
	FullMessages          []SummaryMessage
	SummaryText           string
	TailMessages          []SummaryMessage
	Estimator             tokenbudget.Estimator
	MessageOverheadTokens int
}

type strategyTokenUsage struct {
	BaselineTokens int
	StrategyTokens int
}

func estimateStrategyTokenUsage(input strategyTokenUsageInput) strategyTokenUsage {
	estimator := input.Estimator
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	baseline := estimateSummaryMessagesTokens(input.FullMessages, estimator, input.MessageOverheadTokens)
	strategy := estimator.EstimateTokens(strings.TrimSpace(input.SummaryText)) +
		estimateSummaryMessagesTokens(input.TailMessages, estimator, input.MessageOverheadTokens)
	return strategyTokenUsage{
		BaselineTokens: baseline,
		StrategyTokens: strategy,
	}
}

func estimateSummaryMessagesTokens(
	messages []SummaryMessage,
	estimator tokenbudget.Estimator,
	messageOverheadTokens int,
) int {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	if messageOverheadTokens < 0 {
		messageOverheadTokens = 0
	}
	total := 0
	for _, message := range messages {
		if content := strings.TrimSpace(message.Content); content != "" {
			total += estimator.EstimateTokens(content) + messageOverheadTokens
		}
	}
	return total
}
