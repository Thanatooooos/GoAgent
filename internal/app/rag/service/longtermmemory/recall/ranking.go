package recall

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
)

func projectOrderedMemoryItems(query string, items []domain.MemoryItem) []memoryRecallProjection {
	if len(items) == 0 {
		return nil
	}
	result := make([]memoryRecallProjection, 0, len(items))
	for _, item := range items {
		result = append(result, buildMemoryRecallProjection(query, item))
	}
	return result
}

func sortRuleMemoryItems(items []domain.MemoryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return compareRuleMemoryOrder(items[i], items[j])
	})
}

func sortRuleMemoryProjections(items []memoryRecallProjection) {
	sort.SliceStable(items, func(i, j int) bool {
		return compareRuleMemoryOrder(items[i].item, items[j].item)
	})
}

func compareRuleMemoryOrder(left domain.MemoryItem, right domain.MemoryItem) bool {
	if memoryScopePriority(left.ScopeType) != memoryScopePriority(right.ScopeType) {
		return memoryScopePriority(left.ScopeType) > memoryScopePriority(right.ScopeType)
	}
	if left.Importance != right.Importance {
		return left.Importance > right.Importance
	}
	leftConfirmedAt, leftHasConfirmedAt := timeValue(left.LastConfirmedAt)
	rightConfirmedAt, rightHasConfirmedAt := timeValue(right.LastConfirmedAt)
	if leftHasConfirmedAt != rightHasConfirmedAt {
		return leftHasConfirmedAt
	}
	if leftHasConfirmedAt && !leftConfirmedAt.Equal(rightConfirmedAt) {
		return leftConfirmedAt.After(rightConfirmedAt)
	}
	if !left.UpdateTime.Equal(right.UpdateTime) {
		return left.UpdateTime.After(right.UpdateTime)
	}
	return left.ID > right.ID
}

func timeValue(value *time.Time) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	return *value, true
}

type scoredMemoryItem struct {
	item       domain.MemoryItem
	projection memoryRecallProjection
}

func rankRecallMemories(query string, items []domain.MemoryItem, vectorScores map[string]float32) []memoryRecallProjection {
	scored := make([]scoredMemoryItem, 0, len(items))
	queryPresent := strings.TrimSpace(query) != ""
	for _, item := range items {
		projection := buildMemoryRecallProjection(query, item)
		matchScore, matched := scoreMemoryText(query, projection.searchableText)
		vectorScore := vectorScores[strings.TrimSpace(item.ID)]
		if item.MemoryType == domain.MemoryTypeKnowledge && query != "" && !matched && vectorScore <= 0 {
			continue
		}
		score := matchScore + memoryScopePriority(item.ScopeType) + memoryTypePriority(item.MemoryType)
		if item.LastConfirmedAt != nil {
			score += 5
		}
		projection.keywordMatched = queryPresent && matched
		projection.keywordScore = matchScore
		if vectorScore > 0 {
			projection.vectorMatched = true
			projection.vectorScore = vectorScore
		}
		projection.finalScore = score
		scored = append(scored, scoredMemoryItem{item: item, projection: projection})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		left := scored[i].projection
		right := scored[j].projection
		if left.finalScore != right.finalScore {
			return left.finalScore > right.finalScore
		}
		if left.vectorScore != right.vectorScore {
			return left.vectorScore > right.vectorScore
		}
		return compareRuleMemoryOrder(scored[i].item, scored[j].item)
	})

	result := make([]memoryRecallProjection, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.projection)
	}
	return result
}

func rerankRecallMemoriesWithVectorScores(items []memoryRecallProjection, vectorScores map[string]float32) []memoryRecallProjection {
	if len(items) == 0 || len(vectorScores) == 0 {
		return items
	}
	result := append([]memoryRecallProjection(nil), items...)
	for idx := range result {
		vectorScore := vectorScores[strings.TrimSpace(result[idx].item.ID)]
		if vectorScore > 0 {
			result[idx].vectorMatched = true
			result[idx].vectorScore = vectorScore
			result[idx].finalScore = computeFusedMemoryScore(result[idx], vectorScore)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].finalScore != result[j].finalScore {
			return result[i].finalScore > result[j].finalScore
		}
		if result[i].vectorScore != result[j].vectorScore {
			return result[i].vectorScore > result[j].vectorScore
		}
		return compareRuleMemoryOrder(result[i].item, result[j].item)
	})
	return result
}

func computeFusedMemoryScore(item memoryRecallProjection, vectorScore float32) int {
	fused := item.keywordScore + memoryScopePriority(item.item.ScopeType) + memoryTypePriority(item.item.MemoryType)
	if item.item.LastConfirmedAt != nil {
		fused += 5
	}
	if vectorScore > 0 {
		fused += int(vectorScore * 100)
	}
	return fused
}

func buildMemoryRecallProjection(query string, item domain.MemoryItem) memoryRecallProjection {
	summary := strings.TrimSpace(item.Summary)
	if summary == "" {
		summary = summarizeMemoryText(item.Content, memorytypes.DefaultMemorySummaryRunes)
	}
	detail := pickMemoryProjectionDetail(query, item, summary)
	searchable := normalizeRecallText(strings.Join([]string{
		strings.TrimSpace(item.DisplayValue),
		strings.TrimSpace(item.Summary),
		strings.TrimSpace(item.Content),
	}, " "))
	return memoryRecallProjection{
		item:           item,
		summary:        summary,
		detail:         detail,
		searchableText: searchable,
	}
}

func scoreMemoryText(query string, text string) (int, bool) {
	queryTokens := buildRecallSearchTokens(query)
	searchText := normalizeRecallText(text)
	if len(queryTokens) == 0 || searchText == "" {
		return 0, false
	}
	score := 0
	matched := false
	for _, token := range queryTokens {
		if strings.Contains(searchText, token) {
			matched = true
			score += utf8.RuneCountInString(token) + 2
		}
	}
	return score, matched
}

func pickMemoryProjectionDetail(query string, item domain.MemoryItem, summary string) string {
	if detail := bestMemoryProjectionSegment(query, item.Content); detail != "" && detail != strings.TrimSpace(summary) {
		return detail
	}
	if displayValue := strings.TrimSpace(item.DisplayValue); displayValue != "" && displayValue != strings.TrimSpace(summary) {
		return displayValue
	}
	return ""
}

func bestMemoryProjectionSegment(query string, content string) string {
	segments := splitMemoryProjectionSegments(content)
	if len(segments) == 0 {
		return ""
	}
	best := ""
	bestScore := -1
	for _, segment := range segments {
		score, _ := scoreMemoryText(query, segment)
		if score > bestScore {
			best = segment
			bestScore = score
		}
	}
	best = summarizeMemoryText(best, memorytypes.DefaultMemoryDetailRunes)
	if best == "" && len(segments) > 0 {
		return summarizeMemoryText(segments[0], memorytypes.DefaultMemoryDetailRunes)
	}
	return best
}

func splitMemoryProjectionSegments(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '\n', '\r', '\t', ';', '；':
			return true
		default:
			return false
		}
	})
	segments := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			segments = append(segments, field)
		}
	}
	return segments
}

func mergeMemorySearchHits(items []domain.MemoryItem, hits []domain.MemoryItemSearchHit, vectorScores map[string]float32) []domain.MemoryItem {
	if len(hits) == 0 {
		return items
	}
	existing := make(map[string]int, len(items))
	for idx, item := range items {
		existing[strings.TrimSpace(item.ID)] = idx
	}
	for _, hit := range hits {
		id := strings.TrimSpace(hit.MemoryItem.ID)
		if id == "" {
			continue
		}
		if current, ok := existing[id]; ok {
			items[current] = hit.MemoryItem
		} else {
			existing[id] = len(items)
			items = append(items, hit.MemoryItem)
		}
		if hit.Score > vectorScores[id] {
			vectorScores[id] = hit.Score
		}
	}
	return items
}

func normalizeRecallText(value string) string {
	return compactLowerString(strings.TrimSpace(value))
}
