package longtermmemory

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
)

func buildMemoryRecallContext(items []memoryRecallProjection, maxItems int, maxChars int) ([]memoryRecallProjection, string, bool) {
	if len(items) == 0 || maxItems <= 0 || maxChars <= 0 {
		return nil, "", false
	}
	selected := make([]memoryRecallProjection, 0, minMemoryInt(len(items), maxItems))
	truncated := false
	for _, item := range items {
		if len(selected) >= maxItems {
			truncated = true
			break
		}
		candidate := append(append([]memoryRecallProjection(nil), selected...), item)
		contextText := renderMemoryRecallContext(candidate)
		if strings.TrimSpace(contextText) == "" {
			continue
		}
		if utf8.RuneCountInString(contextText) > maxChars {
			truncated = true
			break
		}
		selected = append(selected, item)
	}
	return selected, renderMemoryRecallContext(selected), truncated
}

func renderMemoryRecallContext(items []memoryRecallProjection) string {
	if len(items) == 0 {
		return ""
	}

	sections := make([]string, 0, 2)
	for _, scopeType := range []string{domain.MemoryScopeKB, domain.MemoryScopeGlobal} {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item.item.ScopeType) != scopeType {
				continue
			}
			line := renderMemoryContextEntry(item)
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			continue
		}
		sections = append(sections, renderMemoryScopeSection(scopeType)+"\n"+strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func renderMemoryContextEntry(item memoryRecallProjection) string {
	summary := strings.TrimSpace(item.summary)
	if summary == "" {
		return ""
	}

	scope := renderMemoryScopeLabel(item.item)
	lines := []string{
		fmt.Sprintf("- [memory_id=%s scope=%s type=%s] %s", strings.TrimSpace(item.item.ID), scope, strings.TrimSpace(item.item.MemoryType), summary),
	}
	if detail := strings.TrimSpace(item.detail); detail != "" {
		lines = append(lines, "  Detail: "+detail)
	}
	return strings.Join(lines, "\n")
}

func renderMemoryScopeSection(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case domain.MemoryScopeKB:
		return "KB-Scoped Memories:"
	case domain.MemoryScopeGlobal:
		return "Global Memories:"
	default:
		return "Other Memories:"
	}
}

func renderMemoryScopeLabel(item domain.MemoryItem) string {
	scope := strings.TrimSpace(item.ScopeType)
	scopeID := strings.TrimSpace(item.ScopeID)
	if scope != "" && scopeID != "" {
		return scope + ":" + scopeID
	}
	if scope != "" {
		return scope
	}
	if scopeID != "" {
		return scopeID
	}
	return "unknown"
}

func projectedMemoryItems(items []memoryRecallProjection) []domain.MemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.MemoryItem, 0, len(items))
	for _, item := range items {
		result = append(result, item.item)
	}
	return result
}

func projectedMemoryEntries(items []memoryRecallProjection) []RecallMemoryEntry {
	if len(items) == 0 {
		return nil
	}
	result := make([]RecallMemoryEntry, 0, len(items))
	for _, item := range items {
		result = append(result, RecallMemoryEntry{
			ID:           strings.TrimSpace(item.item.ID),
			ScopeType:    strings.TrimSpace(item.item.ScopeType),
			ScopeID:      strings.TrimSpace(item.item.ScopeID),
			MemoryType:   strings.TrimSpace(item.item.MemoryType),
			Summary:      strings.TrimSpace(item.summary),
			Detail:       strings.TrimSpace(item.detail),
			HitSources:   memoryHitSources(item),
			KeywordScore: item.keywordScore,
			VectorScore:  item.vectorScore,
			FinalScore:   item.finalScore,
		})
	}
	return result
}

func projectedScopeCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[strings.TrimSpace(item.item.ScopeType)]++
	}
	return counts
}

func projectedTypeCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[strings.TrimSpace(item.item.MemoryType)]++
	}
	return counts
}

func projectedSourceCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		for _, source := range memoryHitSources(item) {
			counts[source]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func projectedContributionCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[memoryContributionKind(item)]++
	}
	return counts
}

func projectedMemoryIDs(items []memoryRecallProjection) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, strings.TrimSpace(item.item.ID))
	}
	return result
}

func memoryHitSources(item memoryRecallProjection) []string {
	sources := make([]string, 0, 2)
	if item.keywordMatched {
		sources = append(sources, memoryHitSourceKeyword)
	}
	if item.vectorMatched {
		sources = append(sources, memoryHitSourceVector)
	}
	return sources
}

func memoryContributionKind(item memoryRecallProjection) string {
	switch {
	case item.keywordMatched && item.vectorMatched:
		return memoryContributionHybrid
	case item.keywordMatched:
		return memoryContributionKeywordOnly
	case item.vectorMatched:
		return memoryContributionVectorOnly
	default:
		return memoryContributionNoDirectSignal
	}
}
