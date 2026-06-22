package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
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

func TestMemoryItemRepositoryListIncludesSearchTextFilter(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.List(context.Background(), port.MemoryItemListFilter{
		UserID:      "user-1",
		ScopeTypes:  []string{domain.MemoryScopeKB},
		MemoryTypes: []string{domain.MemoryTypeKnowledge},
		Statuses:    []string{domain.MemoryStatusActive},
		SearchText:  "chunker",
		ListOptions: port.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "summary ilike") || !strings.Contains(sql, "content ilike") || !strings.Contains(sql, "display_value ilike") || !strings.Contains(sql, "canonical_key ilike") {
		t.Fatalf("expected ILIKE prefilter in SQL, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryListIncludesSearchTokensFilter(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.List(context.Background(), port.MemoryItemListFilter{
		UserID:       "user-1",
		ScopeTypes:   []string{domain.MemoryScopeKB},
		MemoryTypes:  []string{domain.MemoryTypeKnowledge},
		Statuses:     []string{domain.MemoryStatusActive},
		SearchTokens: []string{"vector", "store"},
		ListOptions:  port.ListOptions{Limit: 10},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if strings.Count(sql, "summary ilike") < 2 || strings.Count(sql, "canonical_key ilike") < 2 {
		t.Fatalf("expected token-based ILIKE groups in SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, " or ") {
		t.Fatalf("expected token OR prefilter in SQL, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryCountIncludesSameFiltersAsList(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.Count(context.Background(), port.MemoryItemListFilter{
		UserID:        "user-1",
		ScopeTypes:    []string{domain.MemoryScopeGlobal},
		MemoryTypes:   []string{domain.MemoryTypePreference},
		CanonicalKeys: []string{"response.language"},
		Statuses:      []string{domain.MemoryStatusPending},
		SearchText:    "language",
	})
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "count(") {
		t.Fatalf("expected count sql, got %q", recorder.lastSQL)
	}
	for _, expected := range []string{"user_id", "scope_type", "memory_type", "canonical_key", "status", "summary ilike"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected %q filter in count SQL, got %q", expected, recorder.lastSQL)
		}
	}
	if strings.Contains(sql, "limit ") || strings.Contains(sql, "offset ") {
		t.Fatalf("expected count SQL without paging, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryTouchLastUsedScopesByUserAndIDs(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	err = repo.TouchLastUsed(context.Background(), "user-1", []string{"mem-1", "mem-2"}, time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("TouchLastUsed returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "update") || !strings.Contains(sql, "last_used_at") {
		t.Fatalf("expected last_used_at update SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "user_id") || !strings.Contains(sql, "id in") {
		t.Fatalf("expected user/id scoping in SQL, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryExpireByIDsBuildsScopedUpdate(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.ExpireByIDs(context.Background(), []string{"mem-1", "mem-2"}, "system", time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ExpireByIDs returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "update") || !strings.Contains(sql, "status") || !strings.Contains(sql, "expires_at") {
		t.Fatalf("expected expire update SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "id in") || !strings.Contains(sql, "status <>") {
		t.Fatalf("expected id scoping and status guard, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryDeleteByStatusesUpdatedBeforeBuildsLimitedDelete(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.DeleteByStatusesUpdatedBefore(
		context.Background(),
		[]string{domain.MemoryStatusExpired, domain.MemoryStatusSuperseded},
		time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC),
		25,
	)
	if err != nil {
		t.Fatalf("DeleteByStatusesUpdatedBefore returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "update") || !strings.Contains(sql, "deleted") {
		t.Fatalf("expected soft delete SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "status in") || !strings.Contains(sql, "update_time <") {
		t.Fatalf("expected status and cutoff filters, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "limit 25") {
		t.Fatalf("expected limited candidate subquery, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryListActiveByCanonicalKeyScopesGlobalWithoutScopeIDFilter(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.ListActiveByCanonicalKey(context.Background(), "user-1", domain.MemoryScopeGlobal, "", "response.language")
	if err != nil {
		t.Fatalf("ListActiveByCanonicalKey returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "canonical_key") || !strings.Contains(sql, "status") {
		t.Fatalf("expected canonical key and active status in SQL, got %q", recorder.lastSQL)
	}
	if strings.Contains(sql, "scope_id in") {
		t.Fatalf("expected no scope_id filter for global scope, got %q", recorder.lastSQL)
	}
}

func TestMemoryItemRepositoryListActiveSingleValueConflictsBuildsGroupedQuery(t *testing.T) {
	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMemoryItemRepository(db)
	_, err = repo.ListActiveSingleValueConflicts(context.Background(), []string{"response.language", "project.constraint.network"})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "dry run mode unsupported") {
		t.Fatalf("ListActiveSingleValueConflicts returned error: %v", err)
	}
	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "group by") || !strings.Contains(sql, "having count(*) > 1") {
		t.Fatalf("expected grouped duplicate detection SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "coalesce(scope_id, '')") || !strings.Contains(sql, "canonical_key in") {
		t.Fatalf("expected normalized scope and canonical key filter in SQL, got %q", recorder.lastSQL)
	}
}

type gormTraceRecorder struct {
	logger.Interface
	lastSQL string
}

func (r *gormTraceRecorder) LogMode(level logger.LogLevel) logger.Interface {
	if r.Interface == nil {
		r.Interface = logger.Default
	}
	r.Interface = r.Interface.LogMode(level)
	return r
}

func (r *gormTraceRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.lastSQL = sql
	if r.Interface != nil {
		r.Interface.Trace(ctx, begin, fc, err)
	}
}
