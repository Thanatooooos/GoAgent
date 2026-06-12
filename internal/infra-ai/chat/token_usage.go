package chat

// TokenUsage captures provider-reported or estimated token consumption.
type TokenUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

func (u TokenUsage) IsZero() bool {
	return u.PromptTokens <= 0 && u.CompletionTokens <= 0 && u.TotalTokens <= 0
}

func (u TokenUsage) Normalized() TokenUsage {
	if u.TotalTokens <= 0 && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

func EstimatedTokenUsage(promptTokens int, completionTokens int) TokenUsage {
	usage := TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}
