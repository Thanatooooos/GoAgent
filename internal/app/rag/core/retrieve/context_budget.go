package retrieve

import (
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

type ContextBudgetStats struct {
	CandidateChunks int  `json:"candidateChunks"`
	RetainedChunks  int  `json:"retainedChunks"`
	TokensBefore    int  `json:"tokensBefore"`
	TokensAfter     int  `json:"tokensAfter"`
	Truncated       bool `json:"truncated"`
}

func BuildKnowledgeContextWithinBudget(
	chunks []convention.RetrievedChunk,
	budget int,
	estimator tokenbudget.Estimator,
) (string, ContextBudgetStats) {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	stats := ContextBudgetStats{
		CandidateChunks: len(chunks),
		TokensBefore:    estimator.EstimateTokens(BuildKnowledgeContext(chunks)),
	}
	if len(chunks) == 0 || budget <= 0 {
		stats.Truncated = len(chunks) > 0
		return "", stats
	}

	parts := make([]string, 0, len(chunks))
	used := 0
	for idx, chunk := range chunks {
		part := formatKnowledgeChunk(idx, chunk)
		separatorTokens := 0
		if len(parts) > 0 {
			separatorTokens = estimator.EstimateTokens("\n\n")
		}
		remaining := budget - used - separatorTokens
		if remaining <= 0 {
			stats.Truncated = true
			break
		}
		partTokens := estimator.EstimateTokens(part)
		if partTokens <= remaining {
			parts = append(parts, part)
			used += separatorTokens + partTokens
			stats.RetainedChunks++
			continue
		}

		prefix := formatKnowledgeChunkPrefix(idx, chunk)
		prefixTokens := estimator.EstimateTokens(prefix)
		truncated := ""
		if prefixTokens <= remaining {
			textBudget := remaining - prefixTokens - estimator.EstimateTokens(" ")
			truncatedText, _ := tokenbudget.TruncateText(strings.TrimSpace(chunk.Text), textBudget, estimator)
			truncated = strings.TrimSpace(prefix + " " + truncatedText)
		} else {
			truncated, _ = tokenbudget.TruncateText(prefix, remaining, estimator)
		}
		if truncated != "" {
			parts = append(parts, truncated)
			used += separatorTokens + estimator.EstimateTokens(truncated)
			stats.RetainedChunks++
		}
		stats.Truncated = true
		break
	}

	result := strings.Join(parts, "\n\n")
	stats.TokensAfter = estimator.EstimateTokens(result)
	stats.Truncated = stats.Truncated || stats.RetainedChunks < stats.CandidateChunks
	return result, stats
}

func formatKnowledgeChunk(index int, chunk convention.RetrievedChunk) string {
	return formatKnowledgeChunkPrefix(index, chunk) + " " + strings.TrimSpace(chunk.Text)
}

func formatKnowledgeChunkPrefix(index int, chunk convention.RetrievedChunk) string {
	sourceID := strings.TrimSpace(chunk.DocumentID)
	if sourceID == "" {
		sourceID = strings.TrimSpace(chunk.ID)
	}
	prefix := fmt.Sprintf("[%d]", index+1)
	if sourceID != "" {
		prefix = fmt.Sprintf("[%d:%s]", index+1, sourceID)
	}
	if section, ok := chunk.Metadata["section"].(string); ok && strings.TrimSpace(section) != "" {
		prefix += " (" + strings.TrimSpace(section) + ")"
	}
	return prefix
}
