// Package compression plans the durable-history portion of a runtime context.
package compression

import "local/rag-project/internal/app/rag/core/tokenbudget"

const DefaultRecentBudget = 8000

type Plan struct {
	Past         []Message
	Recent       []Message
	RecentBudget int
	NeedsSummary bool
}

type Message struct {
	Role    string
	Content string
}

// Split keeps the newest complete user/assistant turns within the budget. The
// caller accounts for system, tools, current input, journal, and output before
// passing availableHistoryTokens.
func Split(history []Message, availableHistoryTokens int, estimator tokenbudget.Estimator) Plan {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	budget := availableHistoryTokens
	if budget > DefaultRecentBudget {
		budget = DefaultRecentBudget
	}
	if budget < 0 {
		budget = 0
	}
	start, used := len(history), 0
	for start > 0 {
		turnStart := start - 1
		if history[turnStart].Role == "assistant" && turnStart > 0 && history[turnStart-1].Role == "user" {
			turnStart--
		}
		turnTokens := 0
		for _, message := range history[turnStart:start] {
			turnTokens += estimator.EstimateTokens(message.Content)
		}
		if used+turnTokens > budget {
			break
		}
		used += turnTokens
		start = turnStart
	}
	return Plan{Past: append([]Message(nil), history[:start]...), Recent: append([]Message(nil), history[start:]...), RecentBudget: budget, NeedsSummary: start > 0}
}
