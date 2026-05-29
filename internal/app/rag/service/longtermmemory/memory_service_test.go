package longtermmemory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	ragcache "local/rag-project/internal/app/rag/cache"
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type memoryItemRepoStub struct {
	createFn              func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	updateFn              func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	getByID               func(context.Context, string) (domain.MemoryItem, error)
	listFn                func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error)
	listActiveByKeyFn     func(context.Context, string, string, string, string) ([]domain.MemoryItem, error)
	listActiveConflictsFn func(context.Context, []string) ([]port.ActiveMemoryConflict, error)
	touchFn               func(context.Context, string, []string, time.Time) error
	expireByIDsFn         func(context.Context, []string, string, time.Time) (int64, error)
	deleteBeforeFn        func(context.Context, []string, time.Time, int) (int64, error)
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

func (s memoryItemRepoStub) ListActiveByCanonicalKey(ctx context.Context, userID string, scopeType string, scopeID string, canonicalKey string) ([]domain.MemoryItem, error) {
	if s.listActiveByKeyFn != nil {
		return s.listActiveByKeyFn(ctx, userID, scopeType, scopeID, canonicalKey)
	}
	if s.listFn == nil {
		return nil, nil
	}
	filter := port.MemoryItemListFilter{
		UserID:        userID,
		ScopeTypes:    []string{scopeType},
		CanonicalKeys: []string{canonicalKey},
		Statuses:      []string{domain.MemoryStatusActive},
	}
	if strings.TrimSpace(scopeType) == domain.MemoryScopeKB {
		filter.ScopeIDs = []string{scopeID}
	}
	return s.listFn(ctx, filter)
}

func (s memoryItemRepoStub) ListActiveSingleValueConflicts(ctx context.Context, canonicalKeys []string) ([]port.ActiveMemoryConflict, error) {
	if s.listActiveConflictsFn == nil {
		return nil, nil
	}
	return s.listActiveConflictsFn(ctx, canonicalKeys)
}

func (s memoryItemRepoStub) TouchLastUsed(ctx context.Context, userID string, ids []string, at time.Time) error {
	if s.touchFn == nil {
		return nil
	}
	return s.touchFn(ctx, userID, ids, at)
}

func (s memoryItemRepoStub) ExpireByIDs(ctx context.Context, ids []string, updatedBy string, at time.Time) (int64, error) {
	if s.expireByIDsFn == nil {
		return 0, nil
	}
	return s.expireByIDsFn(ctx, ids, updatedBy, at)
}

func (s memoryItemRepoStub) DeleteByStatusesUpdatedBefore(ctx context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error) {
	if s.deleteBeforeFn == nil {
		return 0, nil
	}
	return s.deleteBeforeFn(ctx, statuses, updatedBefore, limit)
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

type recallCacheStub struct {
	scopeVersions ScopeVersions
	scopeErr      error
	ruleValue     RuleMemoryCacheValue
	ruleHit       bool
	factValue     FactRankingCacheValue
	factHit       bool
	embedding     []float32
	embeddingHit  bool
	globalBumps   []string
	kbBumps       []string
}

func (s *recallCacheStub) GetRuleMemories(context.Context, RuleMemoryCacheKey) (RuleMemoryCacheValue, bool, error) {
	return s.ruleValue, s.ruleHit, nil
}

func (s *recallCacheStub) SetRuleMemories(context.Context, RuleMemoryCacheKey, RuleMemoryCacheValue, time.Duration) error {
	return nil
}

func (s *recallCacheStub) GetFactRankings(context.Context, FactRankingCacheKey) (FactRankingCacheValue, bool, error) {
	return s.factValue, s.factHit, nil
}

func (s *recallCacheStub) SetFactRankings(context.Context, FactRankingCacheKey, FactRankingCacheValue, time.Duration) error {
	return nil
}

func (s *recallCacheStub) GetQueryEmbedding(context.Context, QueryEmbeddingCacheKey) ([]float32, bool, error) {
	return append([]float32(nil), s.embedding...), s.embeddingHit, nil
}

func (s *recallCacheStub) SetQueryEmbedding(context.Context, QueryEmbeddingCacheKey, []float32, time.Duration) error {
	return nil
}

func (s *recallCacheStub) IncrGlobalVersion(_ context.Context, userID string) error {
	s.globalBumps = append(s.globalBumps, userID)
	return nil
}

func (s *recallCacheStub) IncrKBVersion(_ context.Context, userID string, kbID string) error {
	s.kbBumps = append(s.kbBumps, userID+":"+kbID)
	return nil
}

func (s *recallCacheStub) GetScopeVersions(context.Context, string, []string) (ScopeVersions, error) {
	return s.scopeVersions, s.scopeErr
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
	if created.Namespace != "global:global" {
		t.Fatalf("expected default namespace, got %+v", created)
	}
	if created.MemoryType != domain.MemoryTypeKnowledge {
		t.Fatalf("expected default knowledge type, got %+v", created)
	}
	if created.Category != domain.MemoryCategoryGeneral {
		t.Fatalf("expected default general category, got %+v", created)
	}
	if created.ValueType != domain.MemoryValueTypeText {
		t.Fatalf("expected default text value type, got %+v", created)
	}
	if created.ValueJSON != "We always use Chinese for external responses." {
		t.Fatalf("expected default value json from content, got %+v", created)
	}
	if created.DisplayValue == "" {
		t.Fatalf("expected display value, got %+v", created)
	}
	if created.Status != domain.MemoryStatusActive {
		t.Fatalf("expected active status, got %+v", created)
	}
	if created.Summary == "" {
		t.Fatalf("expected generated summary, got %+v", created)
	}
}

func TestMemoryServiceSaveExplicitMemoryResolvesSingleValueUniqueConflictToExistingRecord(t *testing.T) {
	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	listCalls := 0
	existing := domain.MemoryItem{
		ID:           "mem-existing",
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		ScopeID:      "",
		Namespace:    "global:global",
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "zh-CN",
		Content:      "以后都用中文回答",
		Summary:      "以后都用中文回答",
		Status:       domain.MemoryStatusActive,
		UpdateTime:   now,
	}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			return domain.MemoryItem{}, &pgconn.PgError{Code: "23505", ConstraintName: "uk_memory_item_single_active"}
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			t.Fatal("did not expect update path before unique conflict retry")
			return domain.MemoryItem{}, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:  func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
		listActiveByKeyFn: func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
			listCalls++
			if listCalls == 1 {
				return nil, nil
			}
			return []domain.MemoryItem{existing}, nil
		},
	}, MemoryServiceOptions{})

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "zh-CN",
		Content:      "以后都用中文回答",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if item.ID != existing.ID {
		t.Fatalf("expected unique conflict to converge to existing active item, got %+v", item)
	}
}

func TestMemoryServiceSaveExplicitMemoryDoesNotTreatPlainWrappedErrorAsUniqueConflict(t *testing.T) {
	listCalls := 0
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			return domain.MemoryItem{}, errors.New("write failed after checking uk_memory_item_single_active state")
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			t.Fatal("did not expect update path")
			return domain.MemoryItem{}, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:  func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
		listActiveByKeyFn: func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
			listCalls++
			if listCalls == 1 {
				return nil, nil
			}
			return []domain.MemoryItem{{ID: "mem-existing"}}, nil
		},
	}, MemoryServiceOptions{})

	_, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "zh-CN",
		Content:      "以后都用中文回答",
	})
	if err == nil {
		t.Fatal("expected create error to be returned")
	}
	if strings.Contains(err.Error(), "reload active memory items after unique conflict") {
		t.Fatalf("expected plain wrapped error not to enter unique-conflict recovery, got %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected only the normal pre-create lookup, got %d calls", listCalls)
	}
}

func TestMemoryServiceSaveExplicitMemoryReturnsConcurrentWinnerAfterSingleValueUniqueConflict(t *testing.T) {
	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	listCalls := 0
	existing := domain.MemoryItem{
		ID:           "mem-existing",
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		Namespace:    "global:global",
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "en-US",
		DisplayValue: "en-US",
		Content:      "以后都用英文回答",
		Summary:      "以后都用英文回答",
		Status:       domain.MemoryStatusActive,
		UpdateTime:   now,
	}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			return domain.MemoryItem{}, &pgconn.PgError{Code: "23505", ConstraintName: "uk_memory_item_single_active"}
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
			t.Fatal("did not expect update path before unique conflict retry")
			return domain.MemoryItem{}, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:  func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
		listActiveByKeyFn: func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
			listCalls++
			if listCalls == 1 {
				return nil, nil
			}
			return []domain.MemoryItem{existing}, nil
		},
	}, MemoryServiceOptions{})

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "zh-CN",
		Content:      "以后都用中文回答",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if item.ID != existing.ID || item.ValueJSON != "en-US" {
		t.Fatalf("expected concurrent winner to be returned after unique conflict, got %+v", item)
	}
}

func TestMemoryServiceSaveExplicitMemoryRejectsSingleValueCanonicalKeyWithMultipleActiveRecords(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
		listActiveByKeyFn: func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{
				{ID: "mem-1", UserID: "user-1", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference, CanonicalKey: "response.language", Status: domain.MemoryStatusActive},
				{ID: "mem-2", UserID: "user-1", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference, CanonicalKey: "response.language", Status: domain.MemoryStatusActive},
			}, nil
		},
	}, MemoryServiceOptions{})

	_, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		MemoryType:   domain.MemoryTypePreference,
		Category:     domain.MemoryCategoryResponse,
		CanonicalKey: "response.language",
		ValueType:    domain.MemoryValueTypeEnum,
		ValueJSON:    "zh-CN",
		DisplayValue: "zh-CN",
		Content:      "以后都用中文回答",
	})
	if err == nil || !strings.Contains(err.Error(), "multiple active memory items") {
		t.Fatalf("expected duplicate active single-value data to be rejected, got %v", err)
	}
}

func TestNormalizeSaveExplicitMemoryInputLeavesInvalidJSONValueEmpty(t *testing.T) {
	normalized := normalizeSaveExplicitMemoryInput(SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		CanonicalKey: "project.integrations",
		ValueType:    domain.MemoryValueTypeJSON,
		Content:      "slack, jira, notion",
	})

	if normalized.ValueType != domain.MemoryValueTypeJSON {
		t.Fatalf("expected json value type, got %+v", normalized)
	}
	if normalized.ValueJSON != "" {
		t.Fatalf("expected invalid json fallback to stay empty, got %+v", normalized)
	}
}

func TestNormalizeSaveExplicitMemoryInputCanonicalizesJSONObjectFallback(t *testing.T) {
	normalized := normalizeSaveExplicitMemoryInput(SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeGlobal,
		CanonicalKey: "project.integrations",
		ValueType:    domain.MemoryValueTypeJSON,
		Content:      "{ \"b\": 2, \"a\": 1 }",
	})

	if normalized.ValueJSON != "{\"a\":1,\"b\":2}" {
		t.Fatalf("expected canonicalized json fallback, got %+v", normalized)
	}
}

func TestMemoryServiceRecallMemoriesUsesCachedFactRankingWhenAvailable(t *testing.T) {
	var preferenceListCalls int
	var knowledgeListCalls int
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	cache := &recallCacheStub{
		scopeVersions: ScopeVersions{
			GlobalVersion: 3,
			KBVersions:    map[string]int64{"kb-ops": 7},
		},
		factHit: true,
		factValue: FactRankingCacheValue{
			CandidateCount: 1,
			Items: []CachedFactProjection{
				{
					MemoryID:       "mem-kb-1",
					ScopeType:      domain.MemoryScopeKB,
					ScopeID:        "kb-ops",
					MemoryType:     domain.MemoryTypeKnowledge,
					Category:       domain.MemoryCategoryProject,
					CanonicalKey:   "project.constraint.network",
					Summary:        "This service runs inside the internal network.",
					Detail:         "The ingestion service cannot access the public internet directly.",
					KeywordMatched: true,
					KeywordScore:   120,
					FinalScore:     1320,
					UpdateTime:     time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				preferenceListCalls++
			}
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				knowledgeListCalls++
			}
			return nil, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 6})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			t.Fatal("expected fact ranking cache hit to avoid vector recall")
			return nil, nil
		},
	})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "Can this service access public websites directly?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if knowledgeListCalls != 0 {
		t.Fatalf("expected knowledge list calls to be skipped on fact cache hit, got %d", knowledgeListCalls)
	}
	if preferenceListCalls != 2 {
		t.Fatalf("expected preference lists to still load for kb + global scopes, got %d", preferenceListCalls)
	}
	if embedding.callCount != 0 {
		t.Fatalf("expected no embedding calls on fact cache hit, got %d", embedding.callCount)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "mem-kb-1" {
		t.Fatalf("expected cached fact memory result, got %+v", result.Items)
	}
}

func TestMemoryServiceRecallMemoriesOrdersRuleMemoriesByScopeThenImportanceThenFreshness(t *testing.T) {
	cache := &recallCacheStub{
		scopeVersions: ScopeVersions{GlobalVersion: 1},
		ruleHit:       true,
		ruleValue: RuleMemoryCacheValue{
			Items: []CachedMemoryItem{
				{
					ID:         "mem-global-high",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Always answer in Chinese.",
					Content:    "Always answer in Chinese.",
					Status:     domain.MemoryStatusActive,
					Importance: 100,
					UpdateTime: time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
				},
				{
					ID:         "mem-kb-low",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeKB,
					ScopeID:    "kb-ops",
					MemoryType: domain.MemoryTypePreference,
					Summary:    "For this KB, start with action items.",
					Content:    "For this KB, start with action items.",
					Status:     domain.MemoryStatusActive,
					Importance: 40,
					UpdateTime: time.Date(2026, 5, 24, 7, 0, 0, 0, time.UTC),
				},
				{
					ID:         "mem-global-mid-new",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Keep answers concise.",
					Content:    "Keep answers concise.",
					Status:     domain.MemoryStatusActive,
					Importance: 60,
					UpdateTime: time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
				},
				{
					ID:         "mem-global-mid-old",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Check code before proposing changes.",
					Content:    "Check code before proposing changes.",
					Status:     domain.MemoryStatusActive,
					Importance: 60,
					UpdateTime: time.Date(2026, 5, 24, 6, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{MaxRecallItems: 6, MaxRecallChars: 1600})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "How should you answer?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if len(result.RuleMemoryIDs) != 4 {
		t.Fatalf("expected 4 rule memories, got %+v", result.RuleMemoryIDs)
	}
	expected := []string{"mem-kb-low", "mem-global-high", "mem-global-mid-new", "mem-global-mid-old"}
	for idx, id := range expected {
		if result.RuleMemoryIDs[idx] != id {
			t.Fatalf("expected rule memory order %v, got %v", expected, result.RuleMemoryIDs)
		}
	}
	if !strings.Contains(result.Context, "memory_id=mem-kb-low") || !strings.Contains(result.Context, "memory_id=mem-global-high") {
		t.Fatalf("expected ordered rule memories in context, got %q", result.Context)
	}
}

func TestMemoryServiceRecallMemoriesSkipsKBScopedMemoriesWithoutKnowledgeBaseIDs(t *testing.T) {
	var kbListCalls int
	var globalListCalls int
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				kbListCalls++
				return []domain.MemoryItem{{
					ID:         "mem-kb-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeKB,
					ScopeID:    "kb-ops",
					MemoryType: domain.MemoryTypeKnowledge,
					Summary:    "KB-only fact that should stay out when no KB is selected.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
				}}, nil
			}
			globalListCalls++
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				return nil, nil
			}
			return []domain.MemoryItem{{
				ID:         "mem-global-1",
				UserID:     "user-1",
				ScopeType:  domain.MemoryScopeGlobal,
				MemoryType: domain.MemoryTypePreference,
				Summary:    "Prefer concise answers.",
				Status:     domain.MemoryStatusActive,
				UpdateTime: time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
			}}, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1000})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID: "user-1",
		Query:  "How should you answer?",
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if kbListCalls != 0 {
		t.Fatalf("expected no KB-scoped list calls without knowledge base ids, got %d", kbListCalls)
	}
	if globalListCalls == 0 {
		t.Fatalf("expected global memories to still be queried")
	}
	if len(result.Items) != 1 || result.Items[0].ID != "mem-global-1" {
		t.Fatalf("expected only global memory to be recalled, got %+v", result.Items)
	}
}

func TestMemoryServiceRequestScopeCacheSharesFactRankingBetweenRecallAndRetrieve(t *testing.T) {
	var knowledgeListCalls int
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				return nil, nil
			}
			knowledgeListCalls++
			return []domain.MemoryItem{
				{
					ID:         "mem-kb-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeKB,
					ScopeID:    "kb-ops",
					MemoryType: domain.MemoryTypeKnowledge,
					Summary:    "The service runs in the internal network.",
					Content:    "The ingestion service cannot access the public internet directly.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 6})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			return nil, nil
		},
	})
	service.SetRecallCache(nil, RecallCacheOptions{Enabled: true, RequestScopeEnabled: true})

	ctx := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(32))
	if _, err := service.RecallMemories(ctx, RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "Can this service reach public websites?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	}); err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}

	retriever := service.FactRetriever()
	if retriever == nil {
		t.Fatal("expected fact retriever")
	}
	if _, err := retriever.SearchFacts(ctx, ragretrieve.FactMemorySearchRequest{
		UserID:           "user-1",
		Query:            "Can this service reach public websites?",
		KnowledgeBaseIDs: []string{"kb-ops"},
		TopK:             1,
	}); err != nil {
		t.Fatalf("SearchFacts returned error: %v", err)
	}

	if knowledgeListCalls != 2 {
		t.Fatalf("expected one kb and one global fact list across both calls, got %d", knowledgeListCalls)
	}
	if embedding.callCount != 1 {
		t.Fatalf("expected one shared embedding call across both paths, got %d", embedding.callCount)
	}
}

func TestMemoryServiceRecallMemoriesFallbackWritesRuleRequestCacheWhenScopeVersionUnavailable(t *testing.T) {
	var preferenceListCalls int
	cache := &recallCacheStub{scopeErr: errors.New("scope versions unavailable")}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				preferenceListCalls++
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
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true, RequestScopeEnabled: true})

	ctx := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	cache.scopeVersions = ScopeVersions{}
	first, err := service.RecallMemories(ctx, RecallMemoriesInput{UserID: "user-1", Query: "how should you answer?"})
	if err != nil {
		t.Fatalf("first RecallMemories returned error: %v", err)
	}
	second, err := service.RecallMemories(ctx, RecallMemoriesInput{UserID: "user-1", Query: "how should you answer?"})
	if err != nil {
		t.Fatalf("second RecallMemories returned error: %v", err)
	}
	if preferenceListCalls != 1 {
		t.Fatalf("expected fallback request cache to avoid duplicate rule list calls, got %d", preferenceListCalls)
	}
	if first.RuleCacheLayer != "fallback" || second.RuleCacheLayer != "request" {
		t.Fatalf("expected fallback then request cache layers, got first=%s second=%s", first.RuleCacheLayer, second.RuleCacheLayer)
	}
}

func TestMemoryServiceFactRetrieverFallbackWritesFactRequestCacheWhenScopeVersionUnavailable(t *testing.T) {
	var knowledgeListCalls int
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	cache := &recallCacheStub{scopeErr: errors.New("scope versions unavailable")}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				knowledgeListCalls++
				return []domain.MemoryItem{{
					ID:           "mem-kb-1",
					UserID:       "user-1",
					ScopeType:    domain.MemoryScopeKB,
					ScopeID:      "kb-ops",
					MemoryType:   domain.MemoryTypeKnowledge,
					CanonicalKey: "project.constraint.network",
					Summary:      "This service runs inside the internal network.",
					Content:      "The ingestion service cannot access the public internet directly.",
					Status:       domain.MemoryStatusActive,
					UpdateTime:   time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
				}}, nil
			}
			return nil, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 6})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			return nil, nil
		},
	})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true, RequestScopeEnabled: true})

	ctx := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(32))
	retriever := service.FactRetriever()
	if retriever == nil {
		t.Fatal("expected fact retriever")
	}
	first, err := retriever.SearchFacts(ctx, ragretrieve.FactMemorySearchRequest{
		UserID:           "user-1",
		Query:            "Can this service access public websites directly?",
		KnowledgeBaseIDs: []string{"kb-ops"},
		TopK:             1,
	})
	if err != nil {
		t.Fatalf("first SearchFacts returned error: %v", err)
	}
	second, err := retriever.SearchFacts(ctx, ragretrieve.FactMemorySearchRequest{
		UserID:           "user-1",
		Query:            "Can this service access public websites directly?",
		KnowledgeBaseIDs: []string{"kb-ops"},
		TopK:             1,
	})
	if err != nil {
		t.Fatalf("second SearchFacts returned error: %v", err)
	}
	if knowledgeListCalls != 2 {
		t.Fatalf("expected one kb and one global fact list across fallback request caching, got %d", knowledgeListCalls)
	}
	if embedding.callCount != 1 {
		t.Fatalf("expected fallback request cache to avoid duplicate embeddings, got %d", embedding.callCount)
	}
	if len(first.Chunks) != 1 || len(second.Chunks) != 1 {
		t.Fatalf("expected projected chunks on both calls, got first=%+v second=%+v", first, second)
	}
}

func TestMemoryServiceFactRetrieverUsesCachedFactRankingWhenAvailable(t *testing.T) {
	var knowledgeListCalls int
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	cache := &recallCacheStub{
		scopeVersions: ScopeVersions{
			GlobalVersion: 2,
			KBVersions:    map[string]int64{"kb-ops": 5},
		},
		factHit: true,
		factValue: FactRankingCacheValue{
			CandidateCount: 1,
			Items: []CachedFactProjection{
				{
					MemoryID:       "mem-kb-1",
					ScopeType:      domain.MemoryScopeKB,
					ScopeID:        "kb-ops",
					MemoryType:     domain.MemoryTypeKnowledge,
					Category:       domain.MemoryCategoryProject,
					CanonicalKey:   "project.constraint.network",
					Summary:        "This service runs inside the internal network.",
					Detail:         "The ingestion service cannot access the public internet directly.",
					KeywordMatched: true,
					KeywordScore:   120,
					FinalScore:     1320,
					UpdateTime:     time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				knowledgeListCalls++
			}
			return nil, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 6})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			t.Fatal("expected fact ranking cache hit to avoid vector recall")
			return nil, nil
		},
	})
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true})

	retriever := service.FactRetriever()
	result, err := retriever.SearchFacts(context.Background(), ragretrieve.FactMemorySearchRequest{
		UserID:           "user-1",
		Query:            "Can this service access public websites directly?",
		KnowledgeBaseIDs: []string{"kb-ops"},
		TopK:             3,
	})
	if err != nil {
		t.Fatalf("SearchFacts returned error: %v", err)
	}
	if knowledgeListCalls != 0 {
		t.Fatalf("expected no knowledge list calls on fact cache hit, got %d", knowledgeListCalls)
	}
	if embedding.callCount != 0 {
		t.Fatalf("expected no embedding calls on fact cache hit, got %d", embedding.callCount)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "memory_fact:mem-kb-1" {
		t.Fatalf("expected cached projected chunk, got %+v", result.Chunks)
	}
}

func TestMemoryServiceFactRetrieverSkipsKBScopedMemoriesWithoutKnowledgeBaseIDs(t *testing.T) {
	var listFilters []port.MemoryItemListFilter
	var vectorFilters []port.MemoryItemEmbeddingSearchFilter
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			listFilters = append(listFilters, filter)
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeGlobal {
				return []domain.MemoryItem{{
					ID:           "mem-global-1",
					UserID:       "user-1",
					ScopeType:    domain.MemoryScopeGlobal,
					MemoryType:   domain.MemoryTypeKnowledge,
					CanonicalKey: "project.constraint.network",
					Summary:      "This service runs inside the internal network.",
					Content:      "The ingestion service cannot access the public internet directly.",
					Status:       domain.MemoryStatusActive,
					UpdateTime:   time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
				}}, nil
			}
			return nil, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1000})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(_ context.Context, _ []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			vectorFilters = append(vectorFilters, filter)
			return nil, nil
		},
	})

	retriever := service.FactRetriever()
	result, err := retriever.SearchFacts(context.Background(), ragretrieve.FactMemorySearchRequest{
		UserID: "user-1",
		Query:  "Can this service access the public internet?",
		TopK:   3,
	})
	if err != nil {
		t.Fatalf("SearchFacts returned error: %v", err)
	}
	for _, filter := range listFilters {
		if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
			t.Fatalf("did not expect KB-scoped list filter without knowledge base ids: %+v", filter)
		}
	}
	for _, filter := range vectorFilters {
		if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
			t.Fatalf("did not expect KB-scoped vector filter without knowledge base ids: %+v", filter)
		}
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "memory_fact:mem-global-1" {
		t.Fatalf("expected only global fact memory chunk, got %+v", result.Chunks)
	}
}

func TestMemoryServiceSaveAndExpireMemoryBumpRecallCacheVersion(t *testing.T) {
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	cache := &recallCacheStub{}
	metrics := ragcachemetrics.NewService()
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) {
			return domain.MemoryItem{
				ID:         "mem-1",
				UserID:     "user-1",
				ScopeType:  domain.MemoryScopeGlobal,
				MemoryType: domain.MemoryTypePreference,
				Status:     domain.MemoryStatusActive,
				UpdateTime: now.Add(-time.Hour),
			}, nil
		},
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }
	service.SetRecallCache(cache, RecallCacheOptions{Enabled: true})
	service.SetCacheMetrics(metrics)

	_, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeKB,
		ScopeID:      "kb-ops",
		MemoryType:   domain.MemoryTypeKnowledge,
		Content:      "Check vector store connectivity first.",
		Summary:      "Check vector store connectivity first.",
		DisplayValue: "vector-store",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	_, err = service.ExpireMemory(context.Background(), "user-1", "mem-1")
	if err != nil {
		t.Fatalf("ExpireMemory returned error: %v", err)
	}
	if len(cache.kbBumps) != 1 || cache.kbBumps[0] != "user-1:kb-ops" {
		t.Fatalf("expected kb cache version bump, got %+v", cache.kbBumps)
	}
	if len(cache.globalBumps) != 1 || cache.globalBumps[0] != "user-1" {
		t.Fatalf("expected global cache version bump, got %+v", cache.globalBumps)
	}
	if snapshot := metrics.Snapshot(); snapshot.VersionInvalidations != 2 {
		t.Fatalf("expected 2 version invalidations, got %+v", snapshot)
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

func TestMemoryServiceRecallMemoriesSeparatesRulesFromFacts(t *testing.T) {
	var filters []port.MemoryItemListFilter
	var touchedIDs []string
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			filters = append(filters, filter)
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				if len(filter.MemoryTypes) != 1 {
					t.Fatalf("expected exactly one memory type filter, got %+v", filter)
				}
				if filter.MemoryTypes[0] == domain.MemoryTypePreference {
					return []domain.MemoryItem{
						{
							ID:         "mem-kb-pref-1",
							UserID:     "user-1",
							ScopeType:  domain.MemoryScopeKB,
							ScopeID:    "kb-1",
							MemoryType: domain.MemoryTypePreference,
							Summary:    "Start with the action order first.",
							Status:     domain.MemoryStatusActive,
							UpdateTime: time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
						},
					}, nil
				}
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
			if len(filter.MemoryTypes) != 1 {
				t.Fatalf("expected exactly one memory type filter, got %+v", filter)
			}
			switch filter.MemoryTypes[0] {
			case domain.MemoryTypePreference:
				return []domain.MemoryItem{
					{
						ID:         "mem-global-pref-1",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeGlobal,
						MemoryType: domain.MemoryTypePreference,
						Summary:    "Always answer in Chinese.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
					},
				}, nil
			case domain.MemoryTypeKnowledge:
				if filter.SearchText == "" {
					t.Fatalf("expected fact recall to pass SearchText, got %+v", filter)
				}
				if len(filter.SearchTokens) == 0 {
					t.Fatalf("expected fact recall to pass SearchTokens, got %+v", filter)
				}
				return []domain.MemoryItem{
					{
						ID:         "mem-global-fact-1",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeGlobal,
						MemoryType: domain.MemoryTypeKnowledge,
						Summary:    "Use semantic chunk retrieval for follow-up questions.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 20, 8, 30, 0, 0, time.UTC),
					},
				}, nil
			default:
				return nil, nil
			}
		},
		touchFn: func(_ context.Context, _ string, ids []string, _ time.Time) error {
			touchedIDs = append([]string(nil), ids...)
			return nil
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
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 recalled memories, got %+v", result.Items)
	}
	if result.Items[0].ID != "mem-kb-pref-1" || result.Items[1].ID != "mem-global-pref-1" {
		t.Fatalf("expected rule memories first, got %+v", result.Items)
	}
	if result.Items[2].ID != "mem-kb-1" {
		t.Fatalf("expected fact memory after rules, got %+v", result.Items)
	}
	if result.Context == "" {
		t.Fatalf("expected non-empty memory context, got %+v", result)
	}
	if !strings.Contains(result.Context, "Rule Memories:") || !strings.Contains(result.Context, "Fact Memories:") {
		t.Fatalf("expected split memory sections, got %q", result.Context)
	}
	if result.ScopeCounts[domain.MemoryScopeKB] != 2 || result.ScopeCounts[domain.MemoryScopeGlobal] != 1 {
		t.Fatalf("unexpected scope counts: %+v", result.ScopeCounts)
	}
	if result.RuleCount != 2 || result.FactCandidateCount != 1 || result.FactSelectedCount != 1 {
		t.Fatalf("unexpected rule/fact counts: %+v", result)
	}
	if len(result.RuleMemoryIDs) != 2 || result.RuleMemoryIDs[0] != "mem-kb-pref-1" {
		t.Fatalf("unexpected rule memory ids: %+v", result.RuleMemoryIDs)
	}
	if len(result.FactMemoryIDs) != 1 || result.FactMemoryIDs[0] != "mem-kb-1" {
		t.Fatalf("unexpected fact memory ids: %+v", result.FactMemoryIDs)
	}
	if len(result.SelectedMemoryIDs) != 3 || result.SelectedMemoryIDs[2] != "mem-kb-1" {
		t.Fatalf("unexpected selected memory ids: %+v", result.SelectedMemoryIDs)
	}
	if len(touchedIDs) != 3 || touchedIDs[0] != "mem-kb-pref-1" {
		t.Fatalf("expected selected memories to be touched, got %+v", touchedIDs)
	}
	if len(filters) != 4 {
		t.Fatalf("expected 4 list calls, got %+v", filters)
	}
}

func TestMemoryServiceSaveExplicitMemorySingleValuedSameValueUpdatesExisting(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	var created domain.MemoryItem
	var updated domain.MemoryItem
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			return item, nil
		},
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			updated = item
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.CanonicalKeys) != 1 || filter.CanonicalKeys[0] != "response.language" {
				t.Fatalf("unexpected list filter: %+v", filter)
			}
			return []domain.MemoryItem{{
				ID:              "mem-1",
				UserID:          "user-1",
				ScopeType:       domain.MemoryScopeGlobal,
				Namespace:       "global:global",
				MemoryType:      domain.MemoryTypePreference,
				Category:        domain.MemoryCategoryResponse,
				CanonicalKey:    "response.language",
				ValueType:       domain.MemoryValueTypeEnum,
				ValueJSON:       "zh-CN",
				DisplayValue:    "中文",
				Content:         "以后都用中文回答",
				Summary:         "默认中文回答",
				Status:          domain.MemoryStatusActive,
				LastConfirmedAt: ptrTime(now.Add(-time.Hour)),
				UpdateTime:      now.Add(-time.Hour),
			}}, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		CanonicalKey: "response.language",
		ValueJSON:    "zh-CN",
		DisplayValue: "中文",
		Content:      "以后都用中文回答",
		Summary:      "以后默认使用中文回答",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if created.ID != "" {
		t.Fatalf("expected no create on same single-valued memory, got %+v", created)
	}
	if updated.ID != "mem-1" || updated.Status != domain.MemoryStatusActive {
		t.Fatalf("expected existing memory to be updated, got %+v", updated)
	}
	if updated.LastConfirmedAt == nil || !updated.LastConfirmedAt.Equal(now) {
		t.Fatalf("expected last confirmed at to refresh, got %+v", updated)
	}
	if updated.Summary != "以后默认使用中文回答" {
		t.Fatalf("expected richer summary to be merged, got %+v", updated)
	}
	if item.ID != "mem-1" {
		t.Fatalf("expected updated item to be returned, got %+v", item)
	}
}

func TestMemoryServiceSaveExplicitMemorySingleValuedDifferentValueSupersedesExisting(t *testing.T) {
	now := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	var created domain.MemoryItem
	updates := make([]domain.MemoryItem, 0, 1)
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			return item, nil
		},
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			updates = append(updates, item)
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
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
				UpdateTime:   now.Add(-2 * time.Hour),
			}}, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		CanonicalKey: "response.language",
		ValueJSON:    "zh-CN",
		DisplayValue: "中文",
		Content:      "以后都用中文回答",
		Summary:      "默认中文回答",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if len(updates) != 1 || updates[0].Status != domain.MemoryStatusSuperseded {
		t.Fatalf("expected old memory to be superseded, got %+v", updates)
	}
	if created.SupersedesID != "mem-old" || created.Status != domain.MemoryStatusActive {
		t.Fatalf("expected created memory to supersede old one, got %+v", created)
	}
	if item.SupersedesID != "mem-old" {
		t.Fatalf("expected returned item to link superseded id, got %+v", item)
	}
}

func TestMemoryServiceSaveExplicitMemoryMultiValuedSameValueMerges(t *testing.T) {
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	var created domain.MemoryItem
	var updated domain.MemoryItem
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			return item, nil
		},
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			updated = item
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{{
				ID:           "mem-1",
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
				Content:      "项目集成 Slack",
				Status:       domain.MemoryStatusActive,
				UpdateTime:   now.Add(-time.Hour),
			}}, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeKB,
		ScopeID:      "kb-1",
		CanonicalKey: "project.integrations",
		ValueJSON:    "slack",
		DisplayValue: "Slack",
		Content:      "项目已经集成 Slack",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if created.ID != "" {
		t.Fatalf("expected merge path without create, got %+v", created)
	}
	if updated.ID != "mem-1" {
		t.Fatalf("expected existing memory to be merged, got %+v", updated)
	}
	if item.ID != "mem-1" {
		t.Fatalf("expected merged memory item to be returned, got %+v", item)
	}
}

func TestMemoryServiceSaveExplicitMemoryMultiValuedDifferentValueCreatesNew(t *testing.T) {
	now := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
	var created domain.MemoryItem
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return []domain.MemoryItem{{
				ID:           "mem-1",
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
				Content:      "项目集成 Slack",
				Status:       domain.MemoryStatusActive,
				UpdateTime:   now.Add(-time.Hour),
			}}, nil
		},
	}, MemoryServiceOptions{})
	service.now = func() time.Time { return now }

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:       "user-1",
		ScopeType:    domain.MemoryScopeKB,
		ScopeID:      "kb-1",
		CanonicalKey: "project.integrations",
		ValueJSON:    "github",
		DisplayValue: "GitHub",
		Content:      "项目集成 GitHub",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if created.ID == "" || created.Status != domain.MemoryStatusActive {
		t.Fatalf("expected new active memory, got %+v", created)
	}
	if item.ID == "" || item.DisplayValue != "GitHub" {
		t.Fatalf("expected created item to be returned, got %+v", item)
	}
}

func TestMemoryServiceListMemoriesSupportsCategoryAndCanonicalKeyFilters(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.Categories) != 1 || filter.Categories[0] != domain.MemoryCategoryProject {
				t.Fatalf("unexpected category filters: %+v", filter)
			}
			if len(filter.CanonicalKeys) != 1 || filter.CanonicalKeys[0] != "project.integrations" {
				t.Fatalf("unexpected canonical key filters: %+v", filter)
			}
			if len(filter.Statuses) != 1 || filter.Statuses[0] != domain.MemoryStatusSuperseded {
				t.Fatalf("unexpected status filters: %+v", filter)
			}
			return []domain.MemoryItem{{ID: "mem-1"}}, nil
		},
	}, MemoryServiceOptions{DefaultListStatus: domain.MemoryStatusActive})

	items, err := service.ListMemories(context.Background(), ListMemoriesInput{
		UserID:       "user-1",
		Category:     domain.MemoryCategoryProject,
		CanonicalKey: "project.integrations",
		Status:       domain.MemoryStatusSuperseded,
	})
	if err != nil {
		t.Fatalf("ListMemories returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "mem-1" {
		t.Fatalf("unexpected list result: %+v", items)
	}
}

func TestMemoryServiceRecallMemoriesBuildsProjectionAwareScopedContext(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
					return nil, nil
				}
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
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				return nil, nil
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
	if result.Items[0].ID != "mem-global-1" || result.Items[1].ID != "mem-kb-1" {
		t.Fatalf("expected rule memory first and fact memory second, got %+v", result.Items)
	}
	if !strings.Contains(result.Context, "Rule Memories:") {
		t.Fatalf("expected rule memory section, got %q", result.Context)
	}
	if !strings.Contains(result.Context, "Fact Memories:") {
		t.Fatalf("expected fact memory section, got %q", result.Context)
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
	if result.SelectedEntries[0].MemoryType != domain.MemoryTypePreference || result.SelectedEntries[1].ScopeID != "kb-ops" || !strings.Contains(result.SelectedEntries[1].Detail, "vector store") {
		t.Fatalf("unexpected selected entries: %+v", result.SelectedEntries)
	}
	if len(result.SelectedEntries[0].HitSources) != 0 {
		t.Fatalf("expected rule entry without hit sources, got %+v", result.SelectedEntries[0])
	}
	if len(result.SelectedEntries[1].HitSources) != 1 || result.SelectedEntries[1].HitSources[0] != memoryHitSourceKeyword {
		t.Fatalf("expected keyword-only fact entry, got %+v", result.SelectedEntries[1])
	}
	if result.SourceCounts[memoryHitSourceKeyword] != 1 {
		t.Fatalf("expected two keyword-backed memories, got %+v", result.SourceCounts)
	}
	if result.ContributionCounts[memoryContributionKeywordOnly] != 1 {
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
				if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
					return nil, nil
				}
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
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				return nil, nil
			}
			return []domain.MemoryItem{
				{
					ID:         "mem-global-keyword",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypeKnowledge,
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
			if len(filter.MemoryTypes) != 1 || filter.MemoryTypes[0] != domain.MemoryTypeKnowledge {
				t.Fatalf("expected vector recall to stay on knowledge memories, got %+v", filter)
			}
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
	for _, item := range result.Items {
		if item.MemoryType == domain.MemoryTypeFeedback {
			t.Fatalf("expected feedback memories to stay out of chat recall, got %+v", result.Items)
		}
	}
}

func TestMemoryServiceFactRetrieverProjectsKnowledgeMemoriesToChunks(t *testing.T) {
	var filters []port.MemoryItemListFilter
	embedding := &embeddingServiceStub{vector: []float32{0.9, 0.1}}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			filters = append(filters, filter)
			if len(filter.MemoryTypes) != 1 || filter.MemoryTypes[0] != domain.MemoryTypeKnowledge {
				t.Fatalf("expected fact retriever to only search knowledge memories, got %+v", filter)
			}
			if filter.SearchText == "" {
				t.Fatalf("expected fact retriever to pass SearchText, got %+v", filter)
			}
			if len(filter.SearchTokens) == 0 {
				t.Fatalf("expected fact retriever to pass SearchTokens, got %+v", filter)
			}
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItem{
					{
						ID:           "mem-kb-1",
						UserID:       "user-1",
						ScopeType:    domain.MemoryScopeKB,
						ScopeID:      "kb-ops",
						Namespace:    "kb:kb-ops",
						MemoryType:   domain.MemoryTypeKnowledge,
						Category:     domain.MemoryCategoryProject,
						CanonicalKey: "project.constraint.network",
						Summary:      "This service runs inside the internal network.",
						Content:      "The ingestion service runs in an internal network and cannot access the public internet directly.",
						DisplayValue: "internal-only",
						Status:       domain.MemoryStatusActive,
						UpdateTime:   time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC),
					},
				}, nil
			}
			return nil, nil
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200, MaxCandidatesPerScope: 6})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		searchFn: func(_ context.Context, vector []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
			if len(filter.MemoryTypes) != 1 || filter.MemoryTypes[0] != domain.MemoryTypeKnowledge {
				t.Fatalf("expected vector projection to stay on knowledge memories, got %+v", filter)
			}
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				return []domain.MemoryItemSearchHit{
					{
						MemoryItem: domain.MemoryItem{
							ID:           "mem-kb-1",
							UserID:       "user-1",
							ScopeType:    domain.MemoryScopeKB,
							ScopeID:      "kb-ops",
							Namespace:    "kb:kb-ops",
							MemoryType:   domain.MemoryTypeKnowledge,
							Category:     domain.MemoryCategoryProject,
							CanonicalKey: "project.constraint.network",
							Summary:      "This service runs inside the internal network.",
							Content:      "The ingestion service runs in an internal network and cannot access the public internet directly.",
							DisplayValue: "internal-only",
							Status:       domain.MemoryStatusActive,
							UpdateTime:   time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC),
						},
						Score: 0.91,
					},
				}, nil
			}
			return nil, nil
		},
	})

	retriever := service.FactRetriever()
	if retriever == nil {
		t.Fatal("expected fact retriever to be available")
	}

	result, err := retriever.SearchFacts(context.Background(), ragretrieve.FactMemorySearchRequest{
		UserID:           "user-1",
		Query:            "Is this service internal-only or can it access the public internet?",
		KnowledgeBaseIDs: []string{"kb-ops"},
		TopK:             3,
	})
	if err != nil {
		t.Fatalf("SearchFacts returned error: %v", err)
	}
	if len(filters) != 2 {
		t.Fatalf("expected kb + global fact list calls, got %+v", filters)
	}
	if result.CandidateCount != 1 || result.SelectedCount != 1 {
		t.Fatalf("unexpected fact retrieval counts: %+v", result)
	}
	if len(result.SelectedMemoryIDs) != 1 || result.SelectedMemoryIDs[0] != "mem-kb-1" {
		t.Fatalf("unexpected selected memory ids: %+v", result.SelectedMemoryIDs)
	}
	if len(result.Chunks) != 1 {
		t.Fatalf("expected one projected memory chunk, got %+v", result.Chunks)
	}
	chunk := result.Chunks[0]
	if chunk.ID != "memory_fact:mem-kb-1" {
		t.Fatalf("unexpected chunk id: %+v", chunk)
	}
	if chunk.KnowledgeBaseID != "kb-ops" || chunk.DocumentID != "mem-kb-1" {
		t.Fatalf("expected kb/document linkage on chunk, got %+v", chunk)
	}
	if !strings.Contains(chunk.Text, "internal network") {
		t.Fatalf("expected projected fact text, got %+v", chunk)
	}
	if chunk.Metadata["source"] != ragretrieve.ChannelMemoryFact {
		t.Fatalf("expected memory fact source metadata, got %+v", chunk.Metadata)
	}
	if chunk.Metadata["memory_id"] != "mem-kb-1" || chunk.Metadata["canonical_key"] != "project.constraint.network" {
		t.Fatalf("unexpected memory metadata: %+v", chunk.Metadata)
	}
	if !strings.Contains(chunk.Metadata["section"].(string), "Fact Memory") {
		t.Fatalf("expected fact memory section metadata, got %+v", chunk.Metadata)
	}
	if result.ScopeCounts[domain.MemoryScopeKB] != 1 {
		t.Fatalf("unexpected scope counts: %+v", result.ScopeCounts)
	}
}

func TestMemoryServiceRecallMemoriesTouchLastUsedFailsOpen(t *testing.T) {
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
				return nil, nil
			}
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				return []domain.MemoryItem{{
					ID:         "mem-kb-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeKB,
					ScopeID:    "kb-ops",
					MemoryType: domain.MemoryTypeKnowledge,
					Summary:    "Ops troubleshooting note.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
				}}, nil
			}
			return nil, nil
		},
		touchFn: func(context.Context, string, []string, time.Time) error {
			return errors.New("touch failed")
		},
	}, MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200})

	result, err := service.RecallMemories(context.Background(), RecallMemoriesInput{
		UserID:           "user-1",
		Query:            "How should we troubleshoot ingestion?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("RecallMemories returned error: %v", err)
	}
	if !result.Used || len(result.Items) != 1 {
		t.Fatalf("expected recall result despite touch failure, got %+v", result)
	}
}

func TestBuildRecallSearchTokensSupportsWordOrderChangesAndCJKQueries(t *testing.T) {
	tokens := buildRecallSearchTokens("Main bus remove 了吗？")
	if len(tokens) == 0 {
		t.Fatal("expected mixed-language search tokens")
	}
	for _, expected := range []string{"main", "bus", "remove"} {
		if !containsString(tokens, expected) {
			t.Fatalf("expected token %q in %+v", expected, tokens)
		}
	}
	if containsString(tokens, "了吗") {
		t.Fatalf("expected cjk filler token to be filtered, got %+v", tokens)
	}

	cjkTokens := buildRecallSearchTokens("已经移除RocketMQ了吗")
	if !containsString(cjkTokens, "已经") || !containsString(cjkTokens, "移除") {
		t.Fatalf("expected cjk bigram tokens, got %+v", cjkTokens)
	}
	if containsString(cjkTokens, "了吗") {
		t.Fatalf("expected cjk filler token to be filtered, got %+v", cjkTokens)
	}
	if len(cjkTokens) > 8 {
		t.Fatalf("expected token list to be bounded, got %+v", cjkTokens)
	}
}

func TestBuildRecallSearchTokensDropsEnglishAndCJKNoiseTokens(t *testing.T) {
	tokens := buildRecallSearchTokens("How should we troubleshoot the vector store connection please?")
	for _, expected := range []string{"troubleshoot", "vector", "store", "connection"} {
		if !containsString(tokens, expected) {
			t.Fatalf("expected token %q in %+v", expected, tokens)
		}
	}
	for _, unexpected := range []string{"how", "should", "we", "the", "please"} {
		if containsString(tokens, unexpected) {
			t.Fatalf("expected noise token %q to be removed from %+v", unexpected, tokens)
		}
	}

	cjkTokens := buildRecallSearchTokens("请问这个 main bus 可以怎么移除呢")
	for _, unexpected := range []string{"请问", "这个", "可以", "怎么"} {
		if containsString(cjkTokens, unexpected) {
			t.Fatalf("expected cjk noise token %q to be removed from %+v", unexpected, cjkTokens)
		}
	}
}

func TestScoreMemoryTextMatchesWordOrderChangesAndMixedLanguageQueries(t *testing.T) {
	score, matched := scoreMemoryText(
		"How should we troubleshoot vector store connection refused?",
		"Before retrying the pipeline, troubleshoot the connection refused issue by checking vector store connectivity.",
	)
	if !matched || score <= 0 {
		t.Fatalf("expected reordered english text to match, got score=%d matched=%v", score, matched)
	}

	mixedScore, mixedMatched := scoreMemoryText(
		"main bus remove 了吗",
		"The main bus removal is complete. RocketMQ was removed from the main bus path last week.",
	)
	if !mixedMatched || mixedScore <= 0 {
		t.Fatalf("expected mixed-language query to match reordered text, got score=%d matched=%v", mixedScore, mixedMatched)
	}
}

func TestMemoryServiceSaveExplicitMemoryEmbeddingFailureDoesNotAbort(t *testing.T) {
	embedding := &embeddingServiceStub{err: errors.New("embed failed")}
	service := NewMemoryService(memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, MemoryServiceOptions{})
	service.SetEmbeddingSupport(embedding, memoryItemEmbeddingRepoStub{
		upsertFn: func(context.Context, []domain.MemoryItemEmbedding) error {
			t.Fatal("expected no upsert when embedding generation fails")
			return nil
		},
	})

	item, err := service.SaveExplicitMemory(context.Background(), SaveExplicitMemoryInput{
		UserID:  "user-1",
		Content: "Remember that we use the custom chunker.",
	})
	if err != nil {
		t.Fatalf("SaveExplicitMemory returned error: %v", err)
	}
	if item.ID == "" {
		t.Fatalf("expected memory item to still be saved, got %+v", item)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
