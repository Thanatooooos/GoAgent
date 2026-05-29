package longtermmemory

import (
	"context"
	"errors"
	"testing"
	"time"

	ragcache "local/rag-project/internal/app/rag/cache"
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

func TestMemoryServiceSaveExplicitMemoryEmbeddingFailuresRecordMetrics(t *testing.T) {
	genMetrics := ragcachemetrics.NewService()
	genService := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	genService.SetEmbeddingSupport(&embeddingServiceStub{err: errors.New("embed failed")}, memoryItemEmbeddingRepoStub{})
	genService.SetCacheMetrics(genMetrics)

	if _, err := genService.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:  "user-1",
		Content: "Remember that we use the custom chunker.",
	}); err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if snapshot := genMetrics.Snapshot(); snapshot.EmbeddingGenerationFailures != 1 || snapshot.EmbeddingPersistFailures != 0 {
		t.Fatalf("unexpected embedding generation metrics: %+v", snapshot)
	}

	persistMetrics := ragcachemetrics.NewService()
	persistService := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	persistService.SetEmbeddingSupport(&embeddingServiceStub{vector: []float32{0.1, 0.2}}, memoryItemEmbeddingRepoStub{
		upsertFn: func(context.Context, []domain.MemoryItemEmbedding) error {
			return errors.New("persist failed")
		},
	})
	persistService.SetCacheMetrics(persistMetrics)

	if _, err := persistService.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:  "user-1",
		Content: "Remember that we use the custom chunker.",
	}); err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if snapshot := persistMetrics.Snapshot(); snapshot.EmbeddingGenerationFailures != 0 || snapshot.EmbeddingPersistFailures != 1 {
		t.Fatalf("unexpected embedding persist metrics: %+v", snapshot)
	}
}

func TestMemoryServiceRecallMemoriesFailOpenMetricsAreRecorded(t *testing.T) {
	metrics := ragcachemetrics.NewService()
	cache := &recallCacheStub{scopeErr: errors.New("scope versions unavailable")}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				return []domain.MemoryItem{{
					ID:         "mem-global-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Prefer concise answers.",
					Content:    "Prefer concise answers.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
				}}, nil
			}
			return nil, nil
		},
		touchFn: func(context.Context, string, []string, time.Time) error {
			return errors.New("touch failed")
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true, RequestScopeEnabled: true})
	service.SetCacheMetrics(metrics)

	ctx := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result, err := service.RecallMemories(ctx, RecallMemoriesInput{UserID: "user-1", Query: "how should you answer?"})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if !result.Used {
		t.Fatalf("expected memory recall result, got %+v", result)
	}
	snapshot := metrics.Snapshot()
	if snapshot.TouchLastUsedFailures != 1 {
		t.Fatalf("expected one touch failure metric, got %+v", snapshot)
	}
	if snapshot.ScopeVersionLookupFailures == 0 {
		t.Fatalf("expected scope version lookup failure metrics, got %+v", snapshot)
	}
}
