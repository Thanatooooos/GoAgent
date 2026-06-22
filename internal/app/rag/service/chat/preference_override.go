package chat

import (
	"context"
	"strings"

	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
)

type preferenceOverrideSignal struct {
	CanonicalKey    string
	HistoricalValue string
	CurrentValue    string
}

func emitPreferenceOverrideObservability(ctx context.Context, question string, memoryContext string) {
	signal, ok := detectPreferenceOverride(question, memoryContext)
	if !ok {
		return
	}
	longtermmemoryobs.LogRecallOverriddenByCurrentTurn(ctx, signal.CanonicalKey, signal.HistoricalValue, signal.CurrentValue)
}

func detectPreferenceOverride(question string, memoryContext string) (preferenceOverrideSignal, bool) {
	historicalLanguage := detectExplicitLanguagePreference(memoryContext)
	currentLanguage := detectExplicitLanguagePreference(question)
	if historicalLanguage == "" || currentLanguage == "" || historicalLanguage == currentLanguage {
		return preferenceOverrideSignal{}, false
	}
	return preferenceOverrideSignal{
		CanonicalKey:    "response.language",
		HistoricalValue: historicalLanguage,
		CurrentValue:    currentLanguage,
	}, true
}

func detectExplicitLanguagePreference(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}

	englishMarkers := []string{
		"answer in english",
		"reply in english",
		"respond in english",
		"english",
		"英文回答",
		"用英文",
	}
	for _, marker := range englishMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "english"
		}
	}

	chineseMarkers := []string{
		"answer in chinese",
		"reply in chinese",
		"respond in chinese",
		"chinese",
		"中文回答",
		"用中文",
	}
	for _, marker := range chineseMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return "chinese"
		}
	}
	return ""
}
