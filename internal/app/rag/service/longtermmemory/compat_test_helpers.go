package longtermmemory

import (
	"local/rag-project/internal/app/rag/service/longtermmemory/governance"
	"local/rag-project/internal/app/rag/service/longtermmemory/recall"
)

const (
	memoryHitSourceKeyword           = "keyword"
	memoryHitSourceVector            = "vector"
	memoryContributionKeywordOnly    = "keyword_only"
	memoryContributionVectorOnly     = "vector_only"
	memoryContributionHybrid         = "hybrid"
	memoryContributionNoDirectSignal = "none"
)

func normalizeSaveExplicitMemoryInput(input SaveExplicitMemoryInput) governance.NormalizedSaveInput {
	return governance.NormalizeSaveExplicitMemoryInput(input)
}

func buildRecallSearchTokens(query string) []string {
	return recall.BuildRecallSearchTokens(query)
}

func scoreMemoryText(query string, text string) (int, bool) {
	return recall.ScoreMemoryText(query, text)
}
