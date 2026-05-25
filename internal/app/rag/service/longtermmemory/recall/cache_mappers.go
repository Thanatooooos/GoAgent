package recall

import (
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

func memoryItemsToCached(items []domain.MemoryItem) []port.CachedMemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]port.CachedMemoryItem, 0, len(items))
	for _, item := range items {
		cached := port.CachedMemoryItem{
			ID:           strings.TrimSpace(item.ID),
			UserID:       strings.TrimSpace(item.UserID),
			ScopeType:    strings.TrimSpace(item.ScopeType),
			ScopeID:      strings.TrimSpace(item.ScopeID),
			Namespace:    strings.TrimSpace(item.Namespace),
			MemoryType:   strings.TrimSpace(item.MemoryType),
			Category:     strings.TrimSpace(item.Category),
			CanonicalKey: strings.TrimSpace(item.CanonicalKey),
			ValueType:    strings.TrimSpace(item.ValueType),
			ValueJSON:    strings.TrimSpace(item.ValueJSON),
			DisplayValue: strings.TrimSpace(item.DisplayValue),
			Content:      strings.TrimSpace(item.Content),
			Summary:      strings.TrimSpace(item.Summary),
			Status:       strings.TrimSpace(item.Status),
			Importance:   item.Importance,
			UpdateTime:   item.UpdateTime,
		}
		if item.LastConfirmedAt != nil {
			cached.LastConfirmedAt = *item.LastConfirmedAt
		}
		result = append(result, cached)
	}
	return result
}

func cachedMemoryItemsToDomainItems(items []port.CachedMemoryItem) []domain.MemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.MemoryItem, 0, len(items))
	for _, item := range items {
		current := domain.MemoryItem{
			ID:           strings.TrimSpace(item.ID),
			UserID:       strings.TrimSpace(item.UserID),
			ScopeType:    strings.TrimSpace(item.ScopeType),
			ScopeID:      strings.TrimSpace(item.ScopeID),
			Namespace:    strings.TrimSpace(item.Namespace),
			MemoryType:   strings.TrimSpace(item.MemoryType),
			Category:     strings.TrimSpace(item.Category),
			CanonicalKey: strings.TrimSpace(item.CanonicalKey),
			ValueType:    strings.TrimSpace(item.ValueType),
			ValueJSON:    strings.TrimSpace(item.ValueJSON),
			DisplayValue: strings.TrimSpace(item.DisplayValue),
			Content:      strings.TrimSpace(item.Content),
			Summary:      strings.TrimSpace(item.Summary),
			Status:       strings.TrimSpace(item.Status),
			Importance:   item.Importance,
			UpdateTime:   item.UpdateTime,
		}
		if !item.LastConfirmedAt.IsZero() {
			lastConfirmedAt := item.LastConfirmedAt
			current.LastConfirmedAt = &lastConfirmedAt
		}
		result = append(result, current)
	}
	return result
}

func runtimeFactProjectionsToCached(items []memoryRecallProjection) []port.CachedFactProjection {
	if len(items) == 0 {
		return nil
	}
	result := make([]port.CachedFactProjection, 0, len(items))
	for _, item := range items {
		result = append(result, port.CachedFactProjection{
			MemoryID:       strings.TrimSpace(item.item.ID),
			ScopeType:      strings.TrimSpace(item.item.ScopeType),
			ScopeID:        strings.TrimSpace(item.item.ScopeID),
			Namespace:      strings.TrimSpace(item.item.Namespace),
			MemoryType:     strings.TrimSpace(item.item.MemoryType),
			Category:       strings.TrimSpace(item.item.Category),
			CanonicalKey:   strings.TrimSpace(item.item.CanonicalKey),
			DisplayValue:   strings.TrimSpace(item.item.DisplayValue),
			Summary:        strings.TrimSpace(item.summary),
			Detail:         strings.TrimSpace(item.detail),
			KeywordMatched: item.keywordMatched,
			VectorMatched:  item.vectorMatched,
			KeywordScore:   item.keywordScore,
			VectorScore:    item.vectorScore,
			FinalScore:     item.finalScore,
			UpdateTime:     item.item.UpdateTime,
		})
	}
	return result
}

func cachedFactProjectionsToRuntime(items []port.CachedFactProjection) []memoryRecallProjection {
	if len(items) == 0 {
		return nil
	}
	result := make([]memoryRecallProjection, 0, len(items))
	for _, item := range items {
		result = append(result, memoryRecallProjection{
			item: domain.MemoryItem{
				ID:           strings.TrimSpace(item.MemoryID),
				ScopeType:    strings.TrimSpace(item.ScopeType),
				ScopeID:      strings.TrimSpace(item.ScopeID),
				Namespace:    strings.TrimSpace(item.Namespace),
				MemoryType:   strings.TrimSpace(item.MemoryType),
				Category:     strings.TrimSpace(item.Category),
				CanonicalKey: strings.TrimSpace(item.CanonicalKey),
				DisplayValue: strings.TrimSpace(item.DisplayValue),
				UpdateTime:   item.UpdateTime,
			},
			summary:        strings.TrimSpace(item.Summary),
			detail:         strings.TrimSpace(item.Detail),
			searchableText: normalizeRecallText(strings.TrimSpace(item.Summary) + " " + strings.TrimSpace(item.Detail)),
			keywordMatched: item.KeywordMatched,
			vectorMatched:  item.VectorMatched,
			keywordScore:   item.KeywordScore,
			vectorScore:    item.VectorScore,
			finalScore:     item.FinalScore,
		})
	}
	return result
}
