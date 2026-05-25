package governance

import (
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

func refreshExistingMemory(existing domain.MemoryItem, candidate domain.MemoryItem, now time.Time) domain.MemoryItem {
	existing.LastConfirmedAt = &now
	existing.UpdatedBy = candidate.UpdatedBy
	existing.UpdateTime = now
	if candidate.ExpiresAt != nil {
		existing.ExpiresAt = candidate.ExpiresAt
	}
	if len(strings.TrimSpace(candidate.Summary)) > len(strings.TrimSpace(existing.Summary)) {
		existing.Summary = candidate.Summary
	}
	if len(strings.TrimSpace(candidate.Content)) > len(strings.TrimSpace(existing.Content)) {
		existing.Content = candidate.Content
	}
	if len(strings.TrimSpace(candidate.DisplayValue)) > len(strings.TrimSpace(existing.DisplayValue)) {
		existing.DisplayValue = candidate.DisplayValue
	}
	if len(strings.TrimSpace(candidate.ValueJSON)) > len(strings.TrimSpace(existing.ValueJSON)) {
		existing.ValueJSON = candidate.ValueJSON
	}
	if candidate.Importance > existing.Importance {
		existing.Importance = candidate.Importance
	}
	if strings.TrimSpace(existing.Namespace) == "" {
		existing.Namespace = candidate.Namespace
	}
	if strings.TrimSpace(existing.Category) == "" {
		existing.Category = candidate.Category
	}
	if strings.TrimSpace(existing.ValueType) == "" {
		existing.ValueType = candidate.ValueType
	}
	if strings.TrimSpace(existing.ExtractionMethod) == "" {
		existing.ExtractionMethod = candidate.ExtractionMethod
	}
	return existing
}

func markMemorySuperseded(existing domain.MemoryItem, updatedBy string, now time.Time) domain.MemoryItem {
	existing.Status = domain.MemoryStatusSuperseded
	existing.UpdatedBy = strings.TrimSpace(updatedBy)
	existing.UpdateTime = now
	return existing
}

func MarkMemoryExpired(existing domain.MemoryItem, updatedBy string, now time.Time) domain.MemoryItem {
	existing.Status = domain.MemoryStatusExpired
	existing.UpdatedBy = strings.TrimSpace(updatedBy)
	existing.UpdateTime = now
	existing.ExpiresAt = &now
	return existing
}
