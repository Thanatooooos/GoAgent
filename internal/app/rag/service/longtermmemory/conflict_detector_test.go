package longtermmemory

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

func TestDetectMemoryConflictSingleValuedSupersedesPreviousValue(t *testing.T) {
	now := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	repo := memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{{
				ID:           "mem-old",
				UserID:       "user-1",
				ScopeType:    domain.MemoryScopeGlobal,
				Namespace:    "global:global",
				MemoryType:   domain.MemoryTypePreference,
				Category:     domain.MemoryCategoryResponse,
				CanonicalKey: "response.language",
				ValueType:    domain.MemoryValueTypeEnum,
				ValueJSON:    "en-US",
				DisplayValue: "English",
				Content:      "以后都用英文回答",
				Status:       domain.MemoryStatusActive,
				UpdateTime:   now.Add(-time.Hour),
			}}, nil
		},
	}
	candidate := domain.MemoryItem{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		Namespace:    "global:global",
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "中文",
		Content:      "以后都用中文回答",
		UpdatedBy:    "user-1",
	}
	resolution, err := detectMemoryConflict(context.Background(), repo, func() time.Time { return now }, GateDecision{
		Action: GateDecisionCreate,
		Input: normalizedSaveInput{
			UserID:       "user-1",
			ScopeType:    domain.MemoryScopeGlobal,
			CanonicalKey: "response.language",
			MemoryType:   domain.MemoryTypePreference,
		},
		Spec: &MemoryKeySpec{Cardinality: MemoryCardinalitySingle},
	}, candidate)
	if err != nil {
		t.Fatalf("detectMemoryConflict returned error: %v", err)
	}
	if resolution.Action != GateDecisionCreate {
		t.Fatalf("expected create action, got %+v", resolution)
	}
	if resolution.UpdatedExisting == nil || resolution.UpdatedExisting.Status != domain.MemoryStatusSuperseded {
		t.Fatalf("expected superseded existing memory, got %+v", resolution)
	}
	if resolution.CreateCandidate == nil || resolution.CreateCandidate.SupersedesID != "mem-old" {
		t.Fatalf("expected create candidate to link superseded id, got %+v", resolution)
	}
}

func TestDetectMemoryConflictMultiValuedSameValueMerges(t *testing.T) {
	now := time.Date(2026, 5, 22, 9, 30, 0, 0, time.UTC)
	repo := memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{{
				ID:              "mem-1",
				UserID:          "user-1",
				ScopeType:       domain.MemoryScopeKB,
				ScopeID:         "kb-1",
				Namespace:       "kb:kb-1",
				MemoryType:      domain.MemoryTypeKnowledge,
				Category:        domain.MemoryCategoryProject,
				CanonicalKey:    "project.integrations",
				ValueType:       domain.MemoryValueTypeText,
				ValueJSON:       "slack",
				DisplayValue:    "Slack",
				Content:         "项目集成 Slack",
				Status:          domain.MemoryStatusActive,
				LastConfirmedAt: ptrTime(now.Add(-time.Hour)),
				UpdateTime:      now.Add(-time.Hour),
			}}, nil
		},
	}
	candidate := domain.MemoryItem{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeKB,
		ScopeID:      "kb-1",
		Namespace:    "kb:kb-1",
		MemoryType:   domain.MemoryTypeKnowledge,
		Category:     domain.MemoryCategoryProject,
		CanonicalKey: "project.integrations",
		ValueType:    domain.MemoryValueTypeText,
		ValueJSON:    "slack",
		DisplayValue: "Slack",
		Content:      "项目已经集成 Slack",
		UpdatedBy:    "user-1",
	}
	resolution, err := detectMemoryConflict(context.Background(), repo, func() time.Time { return now }, GateDecision{
		Action: GateDecisionCreate,
		Input: normalizedSaveInput{
			UserID:       "user-1",
			ScopeType:    domain.MemoryScopeKB,
			ScopeID:      "kb-1",
			CanonicalKey: "project.integrations",
			MemoryType:   domain.MemoryTypeKnowledge,
		},
		Spec: &MemoryKeySpec{Cardinality: MemoryCardinalityMulti},
	}, candidate)
	if err != nil {
		t.Fatalf("detectMemoryConflict returned error: %v", err)
	}
	if resolution.Action != GateDecisionMerge {
		t.Fatalf("expected merge action, got %+v", resolution)
	}
	if resolution.CreateCandidate != nil {
		t.Fatalf("expected no create candidate on merge, got %+v", resolution)
	}
	if resolution.UpdatedExisting == nil || resolution.UpdatedExisting.ID != "mem-1" {
		t.Fatalf("expected updated existing memory, got %+v", resolution)
	}
}

func TestMemoryItemsEquivalentTreatsJSONObjectsAsStructurallyEqual(t *testing.T) {
	left := domain.MemoryItem{
		ValueType: domain.MemoryValueTypeJSON,
		ValueJSON: `{"allow":false,"mode":"offline"}`,
	}
	right := domain.MemoryItem{
		ValueType: domain.MemoryValueTypeJSON,
		ValueJSON: `{"mode":"offline","allow":false}`,
	}
	if !memoryItemsEquivalent(left, right) {
		t.Fatalf("expected JSON objects with reordered keys to be equivalent")
	}
}

func TestMemoryItemsEquivalentDoesNotMergeDifferentContentOnlyByDisplayValue(t *testing.T) {
	left := domain.MemoryItem{
		CanonicalKey: "project.integrations",
		ValueType:    domain.MemoryValueTypeText,
		DisplayValue: "GitHub",
		Content:      "项目集成 GitHub Enterprise",
	}
	right := domain.MemoryItem{
		CanonicalKey: "project.integrations",
		ValueType:    domain.MemoryValueTypeText,
		DisplayValue: "GitHub",
		Content:      "项目集成 GitHub Actions",
	}
	if memoryItemsEquivalent(left, right) {
		t.Fatalf("expected different content with same display value to stay distinct")
	}
}
