package recall

import (
	"strings"

	"local/rag-project/internal/app/rag/domain"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
)

const (
	memoryHitSourceKeyword           = "keyword"
	memoryHitSourceVector            = "vector"
	memoryContributionKeywordOnly    = "keyword_only"
	memoryContributionVectorOnly     = "vector_only"
	memoryContributionHybrid         = "hybrid"
	memoryContributionNoDirectSignal = "none"
)

func trimMemoryValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func containsMemoryValue(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func normalizeRecallStatuses(values []string) []string {
	values = trimMemoryValues(values)
	if len(values) == 0 {
		return []string{domain.MemoryStatusActive}
	}
	return values
}

func isDefaultRecallStatuses(values []string) bool {
	values = normalizeRecallStatuses(values)
	return len(values) == 1 && values[0] == domain.MemoryStatusActive
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func memoryScopePriority(scopeType string) int {
	if scopeType == domain.MemoryScopeKB {
		return 1000
	}
	return 500
}

func memoryTypePriority(memoryType string) int {
	switch memoryType {
	case domain.MemoryTypePreference:
		return 300
	case domain.MemoryTypeFeedback:
		return 250
	case domain.MemoryTypeKnowledge:
		return 200
	default:
		return 0
	}
}

func summarizeMemoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if value == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = memorytypes.DefaultMemorySummaryRunes
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func BuildRecallSearchTokens(query string) []string {
	return buildRecallSearchTokens(query)
}

func ScoreMemoryText(query string, text string) (int, bool) {
	return scoreMemoryText(query, text)
}
