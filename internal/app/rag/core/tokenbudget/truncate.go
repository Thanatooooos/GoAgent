package tokenbudget

import (
	"sort"
	"strings"
)

const truncatedMarker = "\n...[truncated]"

func TruncateText(text string, budget int, estimator Estimator) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || budget <= 0 {
		return "", text != ""
	}
	if estimator == nil {
		estimator = NewDefaultEstimator()
	}
	if estimator.EstimateTokens(text) <= budget {
		return text, false
	}

	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		candidate := strings.TrimSpace(string(runes[:mid])) + truncatedMarker
		if estimator.EstimateTokens(candidate) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low == 0 {
		return "", true
	}
	return strings.TrimSpace(string(runes[:low])) + truncatedMarker, true
}

type Section struct {
	Name     string
	Text     string
	Priority int
	Required bool
}

type TruncationStats struct {
	TokensBefore     int  `json:"tokensBefore"`
	TokensAfter      int  `json:"tokensAfter"`
	RetainedSections int  `json:"retainedSections"`
	DroppedSections  int  `json:"droppedSections"`
	Truncated        bool `json:"truncated"`
}

func JoinSectionsWithinBudget(
	sections []Section,
	budget int,
	estimator Estimator,
	hardCapChars int,
) (string, TruncationStats) {
	if estimator == nil {
		estimator = NewDefaultEstimator()
	}
	fullParts := make([]string, 0, len(sections))
	for _, section := range sections {
		if text := strings.TrimSpace(section.Text); text != "" {
			fullParts = append(fullParts, text)
		}
	}
	stats := TruncationStats{
		TokensBefore: estimator.EstimateTokens(strings.Join(fullParts, "\n\n")),
	}
	if budget <= 0 {
		stats.DroppedSections = len(fullParts)
		stats.Truncated = len(fullParts) > 0
		return "", stats
	}

	indexes := make([]int, 0, len(sections))
	for idx := range sections {
		if strings.TrimSpace(sections[idx].Text) != "" {
			indexes = append(indexes, idx)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		left := sections[indexes[i]]
		right := sections[indexes[j]]
		if left.Required != right.Required {
			return left.Required
		}
		return left.Priority > right.Priority
	})

	selected := make(map[int]string, len(indexes))
	used := 0
	for _, idx := range indexes {
		text := strings.TrimSpace(sections[idx].Text)
		separatorTokens := 0
		if len(selected) > 0 {
			separatorTokens = estimator.EstimateTokens("\n\n")
		}
		remaining := budget - used - separatorTokens
		if remaining <= 0 {
			continue
		}
		if tokens := estimator.EstimateTokens(text); tokens <= remaining {
			selected[idx] = text
			used += separatorTokens + tokens
			continue
		}
		truncated, _ := TruncateText(text, remaining, estimator)
		if truncated != "" {
			selected[idx] = truncated
			used += separatorTokens + estimator.EstimateTokens(truncated)
		}
	}

	ordered := make([]string, 0, len(selected))
	for idx := range sections {
		if text, ok := selected[idx]; ok {
			ordered = append(ordered, text)
		}
	}
	result := strings.Join(ordered, "\n\n")
	if hardCapChars > 0 && len(result) > hardCapChars {
		result = strings.TrimSpace(result[:hardCapChars-3]) + "..."
	}
	stats.RetainedSections = len(ordered)
	stats.DroppedSections = len(indexes) - stats.RetainedSections
	stats.TokensAfter = estimator.EstimateTokens(result)
	stats.Truncated = stats.TokensAfter < stats.TokensBefore || stats.DroppedSections > 0
	return result, stats
}
