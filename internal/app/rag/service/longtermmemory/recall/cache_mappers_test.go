package recall

import (
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

func TestMemoryItemsToCachedRoundTripsDomainFields(t *testing.T) {
	t.Parallel()

	confirmedAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	updateTime := confirmedAt.Add(time.Hour)
	items := []domain.MemoryItem{
		{
			ID:              " mem-1 ",
			UserID:          " user-1 ",
			ScopeType:       " kb ",
			ScopeID:         " kb-1 ",
			Namespace:       " project ",
			MemoryType:      " knowledge ",
			Category:        " project ",
			CanonicalKey:    " project.messaging.main_bus ",
			ValueType:       " text ",
			ValueJSON:       " removed ",
			DisplayValue:    " Removed ",
			Content:         " Main bus removed ",
			Summary:         " Main bus removed ",
			Status:          " active ",
			Importance:      80,
			LastConfirmedAt: &confirmedAt,
			UpdateTime:      updateTime,
		},
	}

	cached := memoryItemsToCached(items)
	roundTrip := cachedMemoryItemsToDomainItems(cached)

	if len(roundTrip) != 1 {
		t.Fatalf("expected 1 item, got %d", len(roundTrip))
	}
	got := roundTrip[0]
	if got.ID != "mem-1" || got.UserID != "user-1" || got.ScopeID != "kb-1" {
		t.Fatalf("unexpected trimmed round-trip item: %+v", got)
	}
	if got.LastConfirmedAt == nil || !got.LastConfirmedAt.Equal(confirmedAt) {
		t.Fatalf("expected LastConfirmedAt to survive round trip: %+v", got.LastConfirmedAt)
	}
}

func TestFactProjectionCacheMappersPreserveSignals(t *testing.T) {
	t.Parallel()

	updateTime := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	items := []memoryRecallProjection{
		{
			item: domain.MemoryItem{
				ID:           "mem-1",
				ScopeType:    domain.MemoryScopeKB,
				ScopeID:      "kb-1",
				Namespace:    "project",
				MemoryType:   domain.MemoryTypeKnowledge,
				Category:     "project",
				CanonicalKey: "project.messaging.main_bus",
				DisplayValue: "Removed",
				UpdateTime:   updateTime,
			},
			summary:        "Main bus removed",
			detail:         "RocketMQ has been removed",
			keywordMatched: true,
			vectorMatched:  true,
			keywordScore:   9,
			vectorScore:    0.8,
			finalScore:     90,
		},
	}

	cached := runtimeFactProjectionsToCached(items)
	roundTrip := cachedFactProjectionsToRuntime(cached)

	if len(roundTrip) != 1 {
		t.Fatalf("expected 1 item, got %d", len(roundTrip))
	}
	got := roundTrip[0]
	if got.item.ID != "mem-1" || !got.keywordMatched || !got.vectorMatched {
		t.Fatalf("unexpected round-trip projection: %+v", got)
	}
	if got.keywordScore != 9 || got.vectorScore != 0.8 || got.finalScore != 90 {
		t.Fatalf("unexpected scores after round trip: %+v", got)
	}
	if got.searchableText != normalizeRecallText("Main bus removed RocketMQ has been removed") {
		t.Fatalf("unexpected searchableText: %q", got.searchableText)
	}
}
