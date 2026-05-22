package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type memoryItemRepoStub struct {
	createFn func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	updateFn func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	getByID  func(context.Context, string) (domain.MemoryItem, error)
	listFn   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error)
}

func (s memoryItemRepoStub) Create(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	return s.createFn(ctx, item)
}

func (s memoryItemRepoStub) Update(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	return s.updateFn(ctx, item)
}

func (s memoryItemRepoStub) GetByID(ctx context.Context, id string) (domain.MemoryItem, error) {
	return s.getByID(ctx, id)
}

func (s memoryItemRepoStub) List(ctx context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
	return s.listFn(ctx, filter)
}

type memoryItemEmbeddingRepoStub struct {
	upsertFn func(context.Context, []domain.MemoryItemEmbedding) error
	searchFn func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error)
}

func (s memoryItemEmbeddingRepoStub) UpsertBatch(ctx context.Context, embeddings []domain.MemoryItemEmbedding) error {
	if s.upsertFn == nil {
		return nil
	}
	return s.upsertFn(ctx, embeddings)
}

func (s memoryItemEmbeddingRepoStub) SearchByVector(ctx context.Context, vector []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
	if s.searchFn == nil {
		return nil, nil
	}
	return s.searchFn(ctx, vector, filter)
}

type embeddingServiceStub struct {
	vector    []float32
	err       error
	lastText  string
	callCount int
}

func (s *embeddingServiceStub) Embed(text string) ([]float32, error) {
	s.callCount++
	s.lastText = text
	if s.err != nil {
		return nil, s.err
	}
	return append([]float32(nil), s.vector...), nil
}

func (s *embeddingServiceStub) EmbedWithModel(string, string) ([]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) EmbedBatch([]string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) EmbedBatchWithModel([]string, string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) Dimension() int {
	return len(s.vector)
}

func TestMemoryServiceSaveExplicitMemoryDefaultsAndPersists(t *testing.T) {
	var created domain.MemoryItem
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	service.now = func() time.Time {
		return time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	}

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:  "user-1",
		Content: "We always use Chinese for external responses.",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if item.ID == "" || created.ID == "" {
		t.Fatalf("expected generated memory id, got %+v", item)
	}
	if created.ScopeType != domain.MemoryScopeGlobal {
		t.Fatalf("expected default global scope, got %+v", created)
	}
	if created.MemoryType != domain.MemoryTypeKnowledge {
		t.Fatalf("expected default knowledge type, got %+v", created)
	}
	if created.Status != domain.MemoryStatusActive {
		t.Fatalf("expected active status, got %+v", created)
	}
	if created.Summary == "" {
		t.Fatalf("expected generated summary, got %+v", created)
	}
}

func TestMemoryServiceExpireMemoryUpdatesStatus(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	var updated domain.MemoryItem
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			updated = item
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) {
			return domain.MemoryItem{
				ID:         "mem-1",
				UserID:     "user-1",
				Status:     domain.MemoryStatusActive,
				UpdateTime: now.Add(-time.Hour),
			}, nil
		},
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	item, err := service.ExpireMemory(context.Background(), "user-1", "mem-1")
	if err != nil {
		t.Fatalf("ExpireMemory returned error: %v", err)
	}
	if item.Status != domain.MemoryStatusExpired || updated.Status != domain.MemoryStatusExpired {
		t.Fatalf("expected expired status, got item=%+v updated=%+v", item, updated)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(now) {
		t.Fatalf("expected expires_at to be set, got %+v", updated)
	}
}

func TestMemoryServiceRecallMemoriesPrioritizesKBAndPreferences(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItem{
					{
						ID:         "mem-kb-1",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeKB,
						ScopeID:    "kb-1",
						MemoryType: domain.MemoryTypeKnowledge,
						Summary:    "This project uses a custom chunker.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
					},
				}, nil
			}
			return []domain.MemoryItem{
				{
					ID:         "mem-global-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Always answer in Chinese.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1000})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "How should we tune the custom chunker?",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if !result.Used {
		t.Fatalf("expected recall to be used, got %+v", result)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 recalled memories, got %+v", result.Items)
	}
	if result.Items[0].ID != "mem-kb-1" {
		t.Fatalf("expected kb memory first, got %+v", result.Items)
	}
	if result.Items[1].ID != "mem-global-1" {
		t.Fatalf("expected global preference second, got %+v", result.Items)
	}
	if result.Context == "" {
		t.Fatalf("expected non-empty memory context, got %+v", result)
	}
	if result.ScopeCounts[domain.MemoryScopeKB] != 1 || result.ScopeCounts[domain.MemoryScopeGlobal] != 1 {
		t.Fatalf("unexpected scope counts: %+v", result.ScopeCounts)
	}
	if len(result.SelectedMemoryIDs) != 2 || result.SelectedMemoryIDs[0] != "mem-kb-1" {
		t.Fatalf("unexpected selected memory ids: %+v", result.SelectedMemoryIDs)
	}
}

func TestMemoryServiceRecallMemoriesBuildsProjectionAwareScopedContext(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItem{
					{
						ID:         "mem-kb-1",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeKB,
						ScopeID:    "kb-ops",
						MemoryType: domain.MemoryTypeKnowledge,
						Summary:    "Ops troubleshooting note.",
						Content:    "When ingestion fails with connection refused, check vector store connectivity before retrying the pipeline.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
					},
				}, nil
			}
			return []domain.MemoryItem{
				{
					ID:         "mem-global-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Answer with the action order first.",
					Content:    "For incident-style questions, lead with investigation steps before background explanation.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "How should we troubleshoot connection refused for ingestion?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if !result.Used || len(result.Items) != 2 {
		t.Fatalf("expected two recalled memories, got %+v", result)
	}
	if result.Items[0].ID != "mem-kb-1" {
		t.Fatalf("expected KB memory to rank first, got %+v", result.Items)
	}
	if !strings.Contains(result.Context, "KB-Scoped Memories:") {
		t.Fatalf("expected KB-scoped section, got %q", result.Context)
	}
	if !strings.Contains(result.Context, "Global Memories:") {
		t.Fatalf("expected global section, got %q", result.Context)
	}
	if !strings.Contains(result.Context, "vector store connectivity") {
		t.Fatalf("expected projection detail from content, got %q", result.Context)
	}
	if !strings.Contains(result.Context, "memory_id=mem-kb-1") {
		t.Fatalf("expected rendered memory id metadata, got %q", result.Context)
	}
	if len(result.SelectedEntries) != 2 {
		t.Fatalf("expected 2 selected entries, got %+v", result.SelectedEntries)
	}
	if result.SelectedEntries[0].ScopeID != "kb-ops" || !strings.Contains(result.SelectedEntries[0].Detail, "vector store") {
		t.Fatalf("unexpected first selected entry: %+v", result.SelectedEntries[0])
	}
	if len(result.SelectedEntries[0].HitSources) != 1 || result.SelectedEntries[0].HitSources[0] != memoryHitSourceKeyword {
		t.Fatalf("expected keyword-only selected entry, got %+v", result.SelectedEntries[0])
	}
	if result.SourceCounts[memoryHitSourceKeyword] != 2 {
		t.Fatalf("expected two keyword-backed memories, got %+v", result.SourceCounts)
	}
	if result.ContributionCounts[memoryContributionKeywordOnly] != 2 {
		t.Fatalf("expected keyword-only contribution counts, got %+v", result.ContributionCounts)
	}
}

func TestMemoryServiceSaveExplicitMemoryPersistsEmbeddingWhenEnabled(t *testing.T) {
	embedding := &embeddingServiceStub{vector: []float32{0.1, 0.2, 0.3}}
	var upserted []domain.MemoryItemEmbedding
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		upsertFn: func(_ context.Context, embeddings []domain.MemoryItemEmbedding) error {
			upserted = append(upserted, embeddings...)
			return nil
		},
	})
	service.now = func() time.Time {
		return time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	}

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:    "user-1",
		ScopeType: domain.MemoryScopeKB,
		ScopeID:   "kb-ops",
		Content:   "Check vector store connectivity before retrying the pipeline.",
		Summary:   "Retry vector store connectivity first.",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if embedding.callCount != 1 {
		t.Fatalf("expected one embedding call, got %d", embedding.callCount)
	}
	if !strings.Contains(embedding.lastText, "scope: kb:kb-ops") || !strings.Contains(embedding.lastText, "summary: Retry vector store connectivity first.") {
		t.Fatalf("unexpected embedding text: %q", embedding.lastText)
	}
	if len(upserted) != 1 || upserted[0].MemoryItemID != item.ID {
		t.Fatalf("unexpected upserted embeddings: %+v", upserted)
	}
}

func TestMemoryServiceRecallMemoriesTracksKeywordVectorAndHybridContributions(t *testing.T) {
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItem{
					{
						ID:         "mem-kb-hybrid",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeKB,
						ScopeID:    "kb-ops",
						MemoryType: domain.MemoryTypeKnowledge,
						Summary:    "Ops troubleshooting note.",
						Content:    "When ingestion fails with connection refused, check vector store connectivity before retrying the pipeline.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 21, 8, 30, 0, 0, time.UTC),
					},
				}, nil
			}
			return []domain.MemoryItem{
				{
					ID:         "mem-global-keyword",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Troubleshoot connection refused incidents step by step.",
					Content:    "For connection refused incidents, lead with investigation steps before background explanation.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 4})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(_ context.Context, vector []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItemSearchHit{
					{
						MemoryItem: domain.MemoryItem{
							ID:         "mem-kb-hybrid",
							UserID:     "user-1",
							ScopeType:  domain.MemoryScopeKB,
							ScopeID:    "kb-ops",
							MemoryType: domain.MemoryTypeKnowledge,
							Summary:    "Ops troubleshooting note.",
							Content:    "When ingestion fails with connection refused, check vector store connectivity before retrying the pipeline.",
							Status:     domain.MemoryStatusActive,
							UpdateTime: time.Date(2026, 5, 21, 8, 30, 0, 0, time.UTC),
						},
						Score: 0.88,
					},
					{
						MemoryItem: domain.MemoryItem{
							ID:         "mem-kb-vector-only",
							UserID:     "user-1",
							ScopeType:  domain.MemoryScopeKB,
							ScopeID:    "kb-ops",
							MemoryType: domain.MemoryTypeKnowledge,
							Summary:    "Dependency outage note.",
							Content:    "If the pipeline falls over after the storage cluster flaps, inspect backend availability and retry windows.",
							Status:     domain.MemoryStatusActive,
							UpdateTime: time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC),
						},
						Score: 0.92,
					},
				}, nil
			}
			return nil, nil
		},
	})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "How should we troubleshoot connection refused for ingestion?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected hybrid, vector-only, and keyword-only memories, got %+v", result.Items)
	}
	entryByID := map[string]RecallMemoryEntry{}
	for _, entry := range result.SelectedEntries {
		entryByID[entry.ID] = entry
	}
	if got := entryByID["mem-kb-vector-only"]; len(got.HitSources) != 1 || got.HitSources[0] != memoryHitSourceVector || got.KeywordScore != 0 || got.VectorScore <= 0 {
		t.Fatalf("expected vector-only entry metadata, got %+v", got)
	}
	hybrid := entryByID["mem-kb-hybrid"]
	if len(hybrid.HitSources) != 2 || hybrid.HitSources[0] != memoryHitSourceKeyword || hybrid.HitSources[1] != memoryHitSourceVector {
		t.Fatalf("expected hybrid entry metadata, got %+v", hybrid)
	}
	keywordOnly := entryByID["mem-global-keyword"]
	if len(keywordOnly.HitSources) != 1 || keywordOnly.HitSources[0] != memoryHitSourceKeyword || keywordOnly.VectorScore != 0 {
		t.Fatalf("expected keyword-only entry metadata, got %+v", keywordOnly)
	}
	if result.SourceCounts[memoryHitSourceKeyword] != 2 || result.SourceCounts[memoryHitSourceVector] != 2 {
		t.Fatalf("unexpected source counts: %+v", result.SourceCounts)
	}
	if result.ContributionCounts[memoryContributionHybrid] != 1 || result.ContributionCounts[memoryContributionVectorOnly] != 1 || result.ContributionCounts[memoryContributionKeywordOnly] != 1 {
		t.Fatalf("unexpected contribution counts: %+v", result.ContributionCounts)
	}
	if !strings.Contains(result.Context, "memory_id=mem-kb-vector-only") {
		t.Fatalf("expected vector-backed memory to appear in context, got %q", result.Context)
	}
}
