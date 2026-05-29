package recall

import (
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

func TestSortRuleMemoryItemsOrdersByScopeImportanceAndFreshness(t *testing.T) {
	t.Parallel()

	older := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)
	confirmed := newer.Add(2 * time.Hour)

	items := []domain.MemoryItem{
		{ID: "global-high", ScopeType: domain.MemoryScopeGlobal, Importance: 90, UpdateTime: newer},
		{ID: "kb-low", ScopeType: domain.MemoryScopeKB, Importance: 10, UpdateTime: older},
		{ID: "kb-high-confirmed", ScopeType: domain.MemoryScopeKB, Importance: 90, UpdateTime: older, LastConfirmedAt: &confirmed},
		{ID: "kb-high-newer", ScopeType: domain.MemoryScopeKB, Importance: 90, UpdateTime: newer},
	}

	sortRuleMemoryItems(items)

	got := []string{items[0].ID, items[1].ID, items[2].ID, items[3].ID}
	want := []string{"kb-high-confirmed", "kb-high-newer", "kb-low", "global-high"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted ids = %v, want %v", got, want)
		}
	}
}

func TestBestMemoryProjectionSegmentPrefersQueryMatchingDetail(t *testing.T) {
	t.Parallel()

	content := "Main bus removed;\nVector store is unavailable;\nUse Redis cache"
	got := bestMemoryProjectionSegment("vector unavailable", content)
	if got != "Vector store is unavailable" {
		t.Fatalf("bestMemoryProjectionSegment() = %q", got)
	}
}

func TestBuildMemoryRecallProjectionUsesDisplayValueAsFallbackDetail(t *testing.T) {
	t.Parallel()

	item := domain.MemoryItem{
		ID:           "mem-1",
		MemoryType:   domain.MemoryTypeKnowledge,
		DisplayValue: "RocketMQ removed",
		Summary:      "Main bus removed",
		Content:      "",
	}

	got := buildMemoryRecallProjection("rocketmq", item)
	if got.detail != "RocketMQ removed" {
		t.Fatalf("detail = %q, want %q", got.detail, "RocketMQ removed")
	}
}
