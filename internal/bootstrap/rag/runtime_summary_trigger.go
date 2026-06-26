package rag

import "local/rag-project/internal/framework/config"

const (
	defaultSummaryTriggerMaxPromptTokens  = 8000
	defaultSummaryTriggerMinHistoryBudget = 1000
)

func computeSummaryTriggerTokens(cfg *config.Config) int {
	if cfg == nil {
		return defaultSummaryTriggerMinHistoryBudget
	}
	if cfg.Rag.Memory.SummaryTriggerTokens > 0 {
		return cfg.Rag.Memory.SummaryTriggerTokens
	}

	maxPrompt := cfg.Rag.Memory.ChatContext.MaxPromptTokens
	if maxPrompt <= 0 {
		maxPrompt = defaultSummaryTriggerMaxPromptTokens
	}

	overhead := cfg.Rag.Memory.SummaryOverheadReserveTokens
	stageBudget := cfg.Rag.Memory.ChatContext.StageBudget
	if overhead <= 0 &&
		(cfg.Rag.Memory.ChatContext.FixedReserveTokens > 0 ||
			cfg.Rag.Memory.ChatContext.SafetyReserveTokens > 0 ||
			stageBudget.MemoryTokens > 0 ||
			stageBudget.SessionRecallTokens > 0 ||
			stageBudget.RetrieveTokens > 0 ||
			stageBudget.ToolTokens > 0) {
		overhead = cfg.Rag.Memory.ChatContext.FixedReserveTokens +
			cfg.Rag.Memory.ChatContext.SafetyReserveTokens +
			stageBudget.MemoryTokens +
			stageBudget.SessionRecallTokens +
			stageBudget.RetrieveTokens +
			stageBudget.ToolTokens
	}
	if overhead <= 0 {
		overhead = 500 // system prompt
		if cfg.Rag.Memory.ExplicitRecall.MaxContextChars > 0 {
			overhead += cfg.Rag.Memory.ExplicitRecall.MaxContextChars / 2
		}
		if cfg.Rag.Memory.SessionRecall.MaxPromptTokens > 0 {
			overhead += cfg.Rag.Memory.SessionRecall.MaxPromptTokens
		}
		overhead += 1500 // retrieve reserve
		overhead += 500  // tool reserve
		overhead += 500  // question + policy/guidance reserve
	}

	historyBudget := maxPrompt - overhead
	if historyBudget < defaultSummaryTriggerMinHistoryBudget {
		historyBudget = defaultSummaryTriggerMinHistoryBudget
	}
	return historyBudget
}
