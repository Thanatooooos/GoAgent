package rag

import (
	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/framework/config"
)

func buildSummaryBudgetOptions(cfg *config.Config) raghistory.SummaryBudgetOptions {
	var options raghistory.SummaryBudgetOptions
	if cfg == nil {
		return options
	}
	options.SmallMaxChars = readSummaryBudgetMaxChars(cfg.Rag.Memory.SummaryBudget.SmallMaxChars, cfg.Rag.Memory.SummaryMaxChars)
	options.MediumMaxChars = readSummaryBudgetMaxChars(cfg.Rag.Memory.SummaryBudget.MediumMaxChars, cfg.Rag.Memory.SummaryMaxChars)
	options.LargeMaxChars = readSummaryBudgetMaxChars(cfg.Rag.Memory.SummaryBudget.LargeMaxChars, cfg.Rag.Memory.SummaryMaxChars)
	options.MediumMessageCountMin = cfg.Rag.Memory.SummaryBudget.MediumMessageCountMin
	options.LargeMessageCountMin = cfg.Rag.Memory.SummaryBudget.LargeMessageCountMin
	return options
}

func readSummaryBudgetMaxChars(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
