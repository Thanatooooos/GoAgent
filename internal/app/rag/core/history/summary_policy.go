package history

import "strings"

type SummaryBudgetInput struct {
	MessageCount int
	TotalChars   int
	TotalTokens  int
	Messages     []string
}

type SummaryBudgetTier struct {
	Name     string
	MaxChars int
}

func SelectSummaryBudget(input SummaryBudgetInput, options SummaryBudgetOptions) SummaryBudgetTier {
	options = normalizeSummaryBudgetOptions(options)

	if input.TotalTokens > 0 {
		if input.TotalTokens >= 4000 {
			return SummaryBudgetTier{Name: "large", MaxChars: options.LargeMaxChars}
		}
		if input.TotalTokens >= 2000 || containsDenseTechnicalSignals(input.Messages) {
			return SummaryBudgetTier{Name: "medium", MaxChars: options.MediumMaxChars}
		}
		return SummaryBudgetTier{Name: "small", MaxChars: options.SmallMaxChars}
	}

	if options.LargeMessageCountMin > 0 && input.MessageCount >= options.LargeMessageCountMin {
		return SummaryBudgetTier{Name: "large", MaxChars: options.LargeMaxChars}
	}
	if (options.MediumMessageCountMin > 0 && input.MessageCount >= options.MediumMessageCountMin) ||
		containsDenseTechnicalSignals(input.Messages) {
		return SummaryBudgetTier{Name: "medium", MaxChars: options.MediumMaxChars}
	}
	return SummaryBudgetTier{Name: "small", MaxChars: options.SmallMaxChars}
}

func normalizeSummaryBudgetOptions(options SummaryBudgetOptions) SummaryBudgetOptions {
	if options.SmallMaxChars <= 0 {
		options.SmallMaxChars = 400
	}
	if options.MediumMaxChars <= 0 {
		options.MediumMaxChars = maxInt(options.SmallMaxChars, 600)
	}
	if options.LargeMaxChars <= 0 {
		options.LargeMaxChars = maxInt(options.MediumMaxChars, 800)
	}
	return options
}

func containsDenseTechnicalSignals(messages []string) bool {
	signals := []string{
		"error",
		"failed",
		"exception",
		"timeout",
		"refused",
		"unavailable",
		"config",
		"vector",
		"document id",
		"summary-max-chars",
		"doc_",
		"task_",
		"trace_",
	}
	for _, message := range messages {
		lower := strings.ToLower(strings.TrimSpace(message))
		for _, signal := range signals {
			if strings.Contains(lower, signal) {
				return true
			}
		}
	}
	return false
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
