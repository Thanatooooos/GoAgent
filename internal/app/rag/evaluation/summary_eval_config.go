package evaluation

func fixedSummaryFieldJudgeConfig() JudgeConfig {
	return JudgeConfig{
		Temperature: 0,
		MaxTokens:   900,
	}
}

func fixedSummaryEquivalenceJudgeConfig() JudgeConfig {
	return JudgeConfig{
		Temperature: 0,
		MaxTokens:   700,
	}
}

func fixedSummaryAnswerConfig() SummaryAnswerConfig {
	return SummaryAnswerConfig{
		Temperature:                0,
		MaxTokens:                  512,
		EnableRetrieval:            false,
		EnableTools:                false,
		EnableExternalCompensation: false,
	}
}
