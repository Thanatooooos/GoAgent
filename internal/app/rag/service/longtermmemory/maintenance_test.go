package longtermmemory

import (
	"context"
	"errors"
	"testing"
	"time"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

func TestMemoryServiceRunMaintenanceExpiresDueItemsAndDeletesStaleHistory(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	var listFilter port.MemoryItemListFilter
	var expiredIDs []string
	var expiredBy string
	var expiredAt time.Time
	var deletedStatuses []string
	var deletedBefore time.Time
	var deletedLimit int

	cache := &recallCacheStub{}
	metrics := ragcachemetrics.NewService()
	service := NewMemoryService(memoryItemRepoStub{
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			listFilter = filter
			return []domain.MemoryItem{
				{
					ID:         "mem-global-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Status:     domain.MemoryStatusActive,
				},
				{
					ID:         "mem-kb-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeKB,
					ScopeID:    "kb-ops",
					MemoryType: domain.MemoryTypeKnowledge,
					Status:     domain.MemoryStatusPending,
				},
			}, nil
		},
		expireByIDsFn: func(_ context.Context, ids []string, updatedBy string, at time.Time) (int64, error) {
			expiredIDs = append(expiredIDs, ids...)
			expiredBy = updatedBy
			expiredAt = at
			return int64(len(ids)), nil
		},
		deleteBeforeFn: func(_ context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error) {
			deletedStatuses = append(deletedStatuses, statuses...)
			deletedBefore = updatedBefore
			deletedLimit = limit
			return 3, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true})
	service.SetCacheMetrics(metrics)

	result, err := service.RunMaintenance(context.Background(), MaintenanceInput{
		UpdatedBy:       "system",
		DeleteRetention: 30 * 24 * time.Hour,
		ExpireBatchSize: 50,
		DeleteBatchSize: 25,
	})
	if err != nil {
		t.Fatalf("RunMaintenance returned error: %v", err)
	}
	if result.ExpiredCount != 2 || result.DeletedCount != 3 {
		t.Fatalf("unexpected maintenance result: %+v", result)
	}
	if len(expiredIDs) != 2 || expiredIDs[0] != "mem-global-1" || expiredIDs[1] != "mem-kb-1" {
		t.Fatalf("unexpected expired ids: %+v", expiredIDs)
	}
	if expiredBy != "system" || !expiredAt.Equal(now) {
		t.Fatalf("unexpected expire mutation args: updatedBy=%q at=%v", expiredBy, expiredAt)
	}
	if listFilter.ExpiresBefore == nil || !listFilter.ExpiresBefore.Equal(now) {
		t.Fatalf("expected expires_before filter, got %+v", listFilter)
	}
	if len(listFilter.Statuses) != 2 || listFilter.Statuses[0] != domain.MemoryStatusActive || listFilter.Statuses[1] != domain.MemoryStatusPending {
		t.Fatalf("unexpected maintenance list statuses: %+v", listFilter.Statuses)
	}
	if listFilter.Limit != 50 {
		t.Fatalf("expected expire batch size limit, got %+v", listFilter)
	}
	if len(deletedStatuses) != 2 || deletedStatuses[0] != domain.MemoryStatusExpired || deletedStatuses[1] != domain.MemoryStatusSuperseded {
		t.Fatalf("unexpected delete statuses: %+v", deletedStatuses)
	}
	if !deletedBefore.Equal(now.Add(-30*24*time.Hour)) || deletedLimit != 25 {
		t.Fatalf("unexpected delete cutoff args: before=%v limit=%d", deletedBefore, deletedLimit)
	}
	if len(cache.globalBumps) != 1 || cache.globalBumps[0] != "user-1" {
		t.Fatalf("expected one global cache bump, got %+v", cache.globalBumps)
	}
	if len(cache.kbBumps) != 1 || cache.kbBumps[0] != "user-1:kb-ops" {
		t.Fatalf("expected one kb cache bump, got %+v", cache.kbBumps)
	}
	if snapshot := metrics.Snapshot(); snapshot.MaintenanceRuns != 1 || snapshot.MaintenanceExpiredCount != 2 || snapshot.MaintenanceDeletedCount != 3 {
		t.Fatalf("unexpected maintenance metrics snapshot: %+v", snapshot)
	}
}

func TestMemoryServiceRunMaintenanceDefaultsAndSkipsEmptyExpireBatch(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	var deleteStatuses []string
	var deleteBefore time.Time
	var deleteLimit int
	service := NewMemoryService(memoryItemRepoStub{
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if filter.Limit != defaultMemoryMaintenanceExpireBatchSize {
				t.Fatalf("expected default expire batch size, got %+v", filter)
			}
			return nil, nil
		},
		expireByIDsFn: func(context.Context, []string, string, time.Time) (int64, error) {
			t.Fatal("did not expect expire mutation for empty candidate list")
			return 0, nil
		},
		deleteBeforeFn: func(_ context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error) {
			deleteStatuses = append(deleteStatuses, statuses...)
			deleteBefore = updatedBefore
			deleteLimit = limit
			return 0, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	result, err := service.RunMaintenance(context.Background(), MaintenanceInput{})
	if err != nil {
		t.Fatalf("RunMaintenance returned error: %v", err)
	}
	if result.ExpiredCount != 0 || result.DeletedCount != 0 {
		t.Fatalf("unexpected maintenance result: %+v", result)
	}
	if !deleteBefore.Equal(now.Add(-defaultMemoryMaintenanceDeleteRetention)) {
		t.Fatalf("unexpected default delete cutoff: %v", deleteBefore)
	}
	if deleteLimit != defaultMemoryMaintenanceDeleteBatchSize {
		t.Fatalf("unexpected default delete batch size: %d", deleteLimit)
	}
	if len(deleteStatuses) != 2 {
		t.Fatalf("expected expired/superseded delete statuses, got %+v", deleteStatuses)
	}
}

func TestMemoryServiceRunMaintenanceWrapsExpireFailures(t *testing.T) {
	boom := errors.New("boom")
	metrics := ragcachemetrics.NewService()
	service := NewMemoryService(memoryItemRepoStub{
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{{ID: "mem-1", UserID: "user-1", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypeKnowledge}}, nil
		},
		expireByIDsFn: func(context.Context, []string, string, time.Time) (int64, error) {
			return 0, boom
		},
	}, MemoryServiceOptions{})
	service.SetCacheMetrics(metrics)

	_, err := service.RunMaintenance(context.Background(), MaintenanceInput{})
	if err == nil || err.Error() != "failed to expire due memory items" || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped expire error, got %v", err)
	}
	if snapshot := metrics.Snapshot(); snapshot.MaintenanceFailures != 1 {
		t.Fatalf("expected maintenance failure metric, got %+v", snapshot)
	}
}

func TestMemoryServiceRunMaintenanceWrapsDeleteFailures(t *testing.T) {
	boom := errors.New("delete boom")
	metrics := ragcachemetrics.NewService()
	service := NewMemoryService(memoryItemRepoStub{
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return nil, nil
		},
		deleteBeforeFn: func(context.Context, []string, time.Time, int) (int64, error) {
			return 0, boom
		},
	}, MemoryServiceOptions{})
	service.SetCacheMetrics(metrics)

	_, err := service.RunMaintenance(context.Background(), MaintenanceInput{})
	if err == nil || err.Error() != "failed to delete stale memory items" || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped delete error, got %v", err)
	}
	if snapshot := metrics.Snapshot(); snapshot.MaintenanceFailures != 1 {
		t.Fatalf("expected maintenance failure metric, got %+v", snapshot)
	}
}
