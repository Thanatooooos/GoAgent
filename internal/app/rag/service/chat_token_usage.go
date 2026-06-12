package service

import aichat "local/rag-project/internal/infra-ai/chat"

func tokenUsageTraceExtra(usage aichat.TokenUsage, source string) map[string]any {
	usage = usage.Normalized()
	if usage.IsZero() {
		return nil
	}
	extra := map[string]any{
		"promptTokens":     usage.PromptTokens,
		"completionTokens": usage.CompletionTokens,
		"totalTokens":      usage.TotalTokens,
	}
	if source != "" {
		extra["tokenUsageSource"] = source
	}
	return extra
}
