package rag

import (
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

func TestMemoryItemMapperRoundTripsGovernanceFields(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	item := domain.MemoryItem{
		ID:               "mem-1",
		UserID:           "user-1",
		ScopeType:        domain.MemoryScopeKB,
		ScopeID:          "kb-1",
		Namespace:        "kb:kb-1",
		MemoryType:       domain.MemoryTypeKnowledge,
		Category:         domain.MemoryCategoryProject,
		CanonicalKey:     "project.integrations",
		ValueType:        domain.MemoryValueTypeText,
		ValueJSON:        "github",
		DisplayValue:     "GitHub",
		SourceMessageID:  "msg-1",
		Content:          "项目集成 GitHub",
		Summary:          "项目集成 GitHub",
		Confidence:       1,
		Importance:       70,
		Status:           domain.MemoryStatusSuperseded,
		LastConfirmedAt:  &now,
		LastUsedAt:       &now,
		ExpiresAt:        &now,
		SupersedesID:     "mem-0",
		ExtractionMethod: domain.MemoryExtractionMethodManual,
		CreatedBy:        "user-1",
		UpdatedBy:        "user-1",
		CreateTime:       now,
		UpdateTime:       now,
	}

	model := toMemoryItemModel(item)
	roundTrip := toMemoryItemDomain(model)

	if roundTrip.Namespace != item.Namespace || roundTrip.Category != item.Category || roundTrip.CanonicalKey != item.CanonicalKey {
		t.Fatalf("unexpected governance fields after round trip: %+v", roundTrip)
	}
	if roundTrip.ValueType != item.ValueType || roundTrip.ValueJSON != item.ValueJSON || roundTrip.DisplayValue != item.DisplayValue {
		t.Fatalf("unexpected value fields after round trip: %+v", roundTrip)
	}
	if roundTrip.Importance != item.Importance || roundTrip.SupersedesID != item.SupersedesID || roundTrip.ExtractionMethod != item.ExtractionMethod {
		t.Fatalf("unexpected lifecycle fields after round trip: %+v", roundTrip)
	}
}

func TestTrimNonEmptyRemovesBlankValues(t *testing.T) {
	values := trimNonEmpty([]string{" project ", "", "  ", "memory "})
	if len(values) != 2 || values[0] != "project" || values[1] != "memory" {
		t.Fatalf("unexpected trimmed values: %+v", values)
	}
}
