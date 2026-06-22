package longtermmemory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
	fwlog "local/rag-project/internal/framework/log"
)

func TestPreferenceCandidateLifecycleServicePersistPendingPreferenceCandidateStoresPendingCandidate(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	var created domain.MemoryItem
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.createFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
		created = item
		item.ID = "cand-1"
		return item, nil
	}

	metrics := cachemetrics.NewService()
	service := newPreferenceCandidateLifecycleServiceForTest(repo, now)
	service.memory.SetCacheMetrics(metrics)

	candidate, err := service.PersistPendingPreferenceCandidate(context.Background(), PersistPreferenceCandidateInput{
		UserID: "user-1",
		Candidate: PreferenceCandidate{
			ScopeType:        domain.MemoryScopeGlobal,
			MemoryType:       domain.MemoryTypePreference,
			CanonicalKey:     "response.language",
			Summary:          "以后默认用中文回答",
			Content:          "以后默认用中文回答",
			SourceMessageID:  "msg-1",
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
			Confidence:       0.94,
		},
	})
	if err != nil {
		t.Fatalf("PersistPendingPreferenceCandidate returned error: %v", err)
	}
	if candidate.ID != "cand-1" || candidate.Status != domain.MemoryStatusPending {
		t.Fatalf("unexpected returned candidate: %+v", candidate)
	}
	if created.UserID != "user-1" {
		t.Fatalf("expected user id to be persisted, got %+v", created)
	}
	if created.ScopeType != domain.MemoryScopeGlobal || created.Namespace != "global:global" {
		t.Fatalf("expected global namespace defaults, got %+v", created)
	}
	if created.MemoryType != domain.MemoryTypePreference || created.Category != domain.MemoryCategoryResponse {
		t.Fatalf("expected preference response memory fields, got %+v", created)
	}
	if created.CanonicalKey != "response.language" || created.Status != domain.MemoryStatusPending {
		t.Fatalf("expected pending response.language candidate, got %+v", created)
	}
	if created.ValueType != domain.MemoryValueTypeEnum || created.ValueJSON != "以后默认用中文回答" {
		t.Fatalf("expected enum value storage, got %+v", created)
	}
	if created.SourceMessageID != "msg-1" || created.ExtractionMethod != domain.MemoryExtractionMethodLLM {
		t.Fatalf("expected source auditing fields, got %+v", created)
	}
	if created.CreatedBy != "system" || created.UpdatedBy != "system" {
		t.Fatalf("expected system audit actor for auto-generated candidate, got %+v", created)
	}
	if created.LastConfirmedAt != nil {
		t.Fatalf("expected pending candidate to have no confirmation timestamp, got %+v", created)
	}
	if !created.CreateTime.Equal(now) || !created.UpdateTime.Equal(now) {
		t.Fatalf("expected timestamps from service clock, got %+v", created)
	}
	assertLongTermMemoryEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomePendingPersisted, 1)
}

func TestPreferenceCandidateLifecycleServiceTryPersistPendingPreferenceCandidateFailsOpen(t *testing.T) {
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.createFn = func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
		return domain.MemoryItem{}, errors.New("db down")
	}
	service := newPreferenceCandidateLifecycleServiceForTest(repo, time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC))

	candidate, ok := service.TryPersistPendingPreferenceCandidate(context.Background(), PersistPreferenceCandidateInput{
		UserID: "user-1",
		Candidate: PreferenceCandidate{
			CanonicalKey:     "behavior.avoid",
			Summary:          "以后不要一上来就大改代码",
			Content:          "不要一上来就大改代码",
			SourceMessageID:  "msg-2",
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
			Confidence:       0.89,
		},
	})
	if ok {
		t.Fatalf("expected fail-open persistence to report not persisted, got candidate=%+v", candidate)
	}
	if candidate.ID != "" {
		t.Fatalf("expected empty candidate on fail-open path, got %+v", candidate)
	}
}

func TestPreferenceCandidateLifecycleServiceListPendingPreferenceCandidatesReturnsPendingOnly(t *testing.T) {
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.listFn = func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
		if filter.UserID != "user-1" {
			t.Fatalf("unexpected user filter: %+v", filter)
		}
		if len(filter.ScopeTypes) != 1 || filter.ScopeTypes[0] != domain.MemoryScopeGlobal {
			t.Fatalf("expected global scope filter, got %+v", filter)
		}
		if len(filter.MemoryTypes) != 1 || filter.MemoryTypes[0] != domain.MemoryTypePreference {
			t.Fatalf("expected preference filter, got %+v", filter)
		}
		if len(filter.Statuses) != 1 || filter.Statuses[0] != domain.MemoryStatusPending {
			t.Fatalf("expected pending filter, got %+v", filter)
		}
		if len(filter.CanonicalKeys) != len(Phase1PreferenceCanonicalKeys()) {
			t.Fatalf("expected phase-1 canonical key filter, got %+v", filter)
		}
		return []domain.MemoryItem{
			{
				ID:               "cand-1",
				UserID:           "user-1",
				ScopeType:        domain.MemoryScopeGlobal,
				Namespace:        "global:global",
				MemoryType:       domain.MemoryTypePreference,
				Category:         domain.MemoryCategoryResponse,
				CanonicalKey:     "response.language",
				ValueType:        domain.MemoryValueTypeEnum,
				ValueJSON:        "以后默认用中文回答",
				DisplayValue:     "以后默认用中文回答",
				SourceMessageID:  "msg-1",
				Content:          "以后默认用中文回答",
				Summary:          "以后默认用中文回答",
				Confidence:       0.94,
				Status:           domain.MemoryStatusPending,
				ExtractionMethod: domain.MemoryExtractionMethodLLM,
			},
		}, nil
	}
	repo.countFn = func(_ context.Context, filter port.MemoryItemListFilter) (int64, error) {
		if filter.Offset != 0 || filter.Limit != 0 {
			t.Fatalf("expected count filter without paging, got %+v", filter)
		}
		return 37, nil
	}

	service := newPreferenceCandidateLifecycleServiceForTest(repo, time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC))

	result, err := service.ListPendingPreferenceCandidates(context.Background(), ListPreferenceCandidatesInput{
		UserID:   "user-1",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListPendingPreferenceCandidates returned error: %v", err)
	}
	if result.Total != 37 || len(result.Items) != 1 {
		t.Fatalf("unexpected pending list result: %+v", result)
	}
	if result.Items[0].ID != "cand-1" || result.Items[0].Status != domain.MemoryStatusPending {
		t.Fatalf("unexpected pending list item: %+v", result.Items[0])
	}
}

func TestPreferenceCandidateLifecycleServiceConfirmPreferenceCandidateSupersedesSingleValueActiveMemory(t *testing.T) {
	now := time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC)
	var updates []domain.MemoryItem
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
		return domain.MemoryItem{
			ID:               "cand-1",
			UserID:           "user-1",
			ScopeType:        domain.MemoryScopeGlobal,
			Namespace:        "global:global",
			MemoryType:       domain.MemoryTypePreference,
			Category:         domain.MemoryCategoryWorkflow,
			CanonicalKey:     "workflow.troubleshooting.first_step",
			ValueType:        domain.MemoryValueTypeText,
			ValueJSON:        "先判断是不是环境问题",
			DisplayValue:     "先判断是不是环境问题",
			SourceMessageID:  "msg-3",
			Content:          "先判断是不是环境问题",
			Summary:          "以后遇到报错先判断是不是环境问题",
			Confidence:       0.91,
			Status:           domain.MemoryStatusPending,
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
			CreatedBy:        "system",
			UpdatedBy:        "system",
			CreateTime:       now.Add(-time.Minute),
			UpdateTime:       now.Add(-time.Minute),
		}, nil
	}
	repo.listActiveByKeyFn = func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
		return []domain.MemoryItem{{
			ID:               "mem-old",
			UserID:           "user-1",
			ScopeType:        domain.MemoryScopeGlobal,
			Namespace:        "global:global",
			MemoryType:       domain.MemoryTypePreference,
			Category:         domain.MemoryCategoryWorkflow,
			CanonicalKey:     "workflow.troubleshooting.first_step",
			ValueType:        domain.MemoryValueTypeText,
			ValueJSON:        "先看错误日志",
			DisplayValue:     "先看错误日志",
			Content:          "先看错误日志",
			Summary:          "遇到问题先看错误日志",
			Confidence:       1,
			Status:           domain.MemoryStatusActive,
			ExtractionMethod: domain.MemoryExtractionMethodManual,
			CreatedBy:        "user-1",
			UpdatedBy:        "user-1",
			CreateTime:       now.Add(-2 * time.Hour),
			UpdateTime:       now.Add(-time.Hour),
		}}, nil
	}
	repo.updateFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
		updates = append(updates, item)
		return item, nil
	}

	metrics := cachemetrics.NewService()
	service := newPreferenceCandidateLifecycleServiceForTest(repo, now)
	service.memory.SetCacheMetrics(metrics)

	candidate, err := service.ConfirmPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-1",
	})
	if err != nil {
		t.Fatalf("ConfirmPreferenceCandidate returned error: %v", err)
	}
	if candidate.ID != "cand-1" || candidate.Status != domain.MemoryStatusActive {
		t.Fatalf("unexpected confirmed candidate: %+v", candidate)
	}
	if len(updates) != 2 {
		t.Fatalf("expected supersede + activate updates, got %+v", updates)
	}
	if updates[0].ID != "mem-old" || updates[0].Status != domain.MemoryStatusSuperseded {
		t.Fatalf("expected old active memory to be superseded first, got %+v", updates[0])
	}
	if updates[1].ID != "cand-1" || updates[1].Status != domain.MemoryStatusActive || updates[1].SupersedesID != "mem-old" {
		t.Fatalf("expected pending candidate to become active and supersede old value, got %+v", updates[1])
	}
	if updates[1].LastConfirmedAt == nil || !updates[1].LastConfirmedAt.Equal(now) {
		t.Fatalf("expected confirmation timestamp on activated candidate, got %+v", updates[1])
	}
	if updates[1].UpdatedBy != "user-1" {
		t.Fatalf("expected confirmer to become update actor, got %+v", updates[1])
	}
	assertLongTermMemoryEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomeCandidateConfirmed, 1)
}

func TestPreferenceCandidateLifecycleServiceConfirmPreferenceCandidateEnforcesBehaviorAvoidQuota(t *testing.T) {
	now := time.Date(2026, 6, 13, 11, 30, 0, 0, time.UTC)
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
		return domain.MemoryItem{
			ID:               "cand-2",
			UserID:           "user-1",
			ScopeType:        domain.MemoryScopeGlobal,
			Namespace:        "global:global",
			MemoryType:       domain.MemoryTypePreference,
			Category:         domain.MemoryCategoryBehavior,
			CanonicalKey:     "behavior.avoid",
			ValueType:        domain.MemoryValueTypeText,
			ValueJSON:        "不要一上来就大改代码",
			DisplayValue:     "不要一上来就大改代码",
			SourceMessageID:  "msg-4",
			Content:          "不要一上来就大改代码",
			Summary:          "以后不要一上来就大改代码",
			Confidence:       0.89,
			Status:           domain.MemoryStatusPending,
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
		}, nil
	}
	repo.listActiveByKeyFn = func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
		active := make([]domain.MemoryItem, 0, 10)
		for idx := 0; idx < 10; idx++ {
			active = append(active, domain.MemoryItem{
				ID:           "mem-" + string(rune('a'+idx)),
				UserID:       "user-1",
				ScopeType:    domain.MemoryScopeGlobal,
				MemoryType:   domain.MemoryTypePreference,
				CanonicalKey: "behavior.avoid",
				Content:      "avoid rule",
				Summary:      "avoid rule",
				Status:       domain.MemoryStatusActive,
			})
		}
		return active, nil
	}
	repo.updateFn = func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
		t.Fatal("did not expect updates when behavior.avoid quota is exceeded")
		return domain.MemoryItem{}, nil
	}

	service := newPreferenceCandidateLifecycleServiceForTest(repo, now)

	_, err := service.ConfirmPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-2",
	})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected quota exceeded error, got %v", err)
	}
}

func TestPreferenceCandidateLifecycleServiceRejectPreferenceCandidateTransitionsPendingToRejected(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	var updated domain.MemoryItem
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
		return domain.MemoryItem{
			ID:               "cand-3",
			UserID:           "user-1",
			ScopeType:        domain.MemoryScopeGlobal,
			Namespace:        "global:global",
			MemoryType:       domain.MemoryTypePreference,
			Category:         domain.MemoryCategoryBehavior,
			CanonicalKey:     "behavior.avoid",
			ValueType:        domain.MemoryValueTypeText,
			ValueJSON:        "不要一上来就大改代码",
			DisplayValue:     "不要一上来就大改代码",
			SourceMessageID:  "msg-5",
			Content:          "不要一上来就大改代码",
			Summary:          "以后不要一上来就大改代码",
			Confidence:       0.89,
			Status:           domain.MemoryStatusPending,
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
			CreatedBy:        "system",
			UpdatedBy:        "system",
		}, nil
	}
	repo.updateFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
		updated = item
		return item, nil
	}

	metrics := cachemetrics.NewService()
	service := newPreferenceCandidateLifecycleServiceForTest(repo, now)
	service.memory.SetCacheMetrics(metrics)

	candidate, err := service.RejectPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-3",
	})
	if err != nil {
		t.Fatalf("RejectPreferenceCandidate returned error: %v", err)
	}
	if candidate.Status != domain.MemoryStatusRejected || updated.Status != domain.MemoryStatusRejected {
		t.Fatalf("expected pending candidate to become rejected, candidate=%+v updated=%+v", candidate, updated)
	}
	if updated.UpdatedBy != "user-1" || !updated.UpdateTime.Equal(now) {
		t.Fatalf("expected reject audit fields to be updated, got %+v", updated)
	}
	assertLongTermMemoryEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomeCandidateRejected, 1)
}

func TestPreferenceCandidateLifecycleServiceRejectsInvalidPendingTransitions(t *testing.T) {
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
		return domain.MemoryItem{
			ID:               "cand-4",
			UserID:           "user-1",
			ScopeType:        domain.MemoryScopeGlobal,
			Namespace:        "global:global",
			MemoryType:       domain.MemoryTypePreference,
			Category:         domain.MemoryCategoryResponse,
			CanonicalKey:     "response.language",
			ValueType:        domain.MemoryValueTypeEnum,
			ValueJSON:        "以后默认用中文回答",
			DisplayValue:     "以后默认用中文回答",
			SourceMessageID:  "msg-6",
			Content:          "以后默认用中文回答",
			Summary:          "以后默认用中文回答",
			Confidence:       0.94,
			Status:           domain.MemoryStatusActive,
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
		}, nil
	}
	repo.updateFn = func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) {
		t.Fatal("did not expect update for invalid state transition")
		return domain.MemoryItem{}, nil
	}
	service := newPreferenceCandidateLifecycleServiceForTest(repo, time.Date(2026, 6, 13, 12, 30, 0, 0, time.UTC))

	_, confirmErr := service.ConfirmPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-4",
	})
	if confirmErr == nil || !strings.Contains(confirmErr.Error(), "not pending") {
		t.Fatalf("expected not pending confirmation error, got %v", confirmErr)
	}

	_, rejectErr := service.RejectPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-4",
	})
	if rejectErr == nil || !strings.Contains(rejectErr.Error(), "not pending") {
		t.Fatalf("expected not pending rejection error, got %v", rejectErr)
	}
}

func TestPreferenceCandidateLifecycleServiceLogsConfirmationAndRejection(t *testing.T) {
	t.Parallel()

	t.Run("confirm", func(t *testing.T) {
		now := time.Date(2026, 6, 13, 13, 0, 0, 0, time.UTC)
		core, observed := observer.New(zap.InfoLevel)
		ctx := fwlog.BindLogger(context.Background(), zap.New(core).Sugar())
		repo := newPreferenceCandidateLifecycleRepoStub()
		repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
			return domain.MemoryItem{
				ID:               "cand-log-confirm",
				UserID:           "user-1",
				ScopeType:        domain.MemoryScopeGlobal,
				Namespace:        "global:global",
				MemoryType:       domain.MemoryTypePreference,
				Category:         domain.MemoryCategoryBehavior,
				CanonicalKey:     "behavior.avoid",
				ValueType:        domain.MemoryValueTypeText,
				ValueJSON:        "Do not jump into large refactors first.",
				DisplayValue:     "Do not jump into large refactors first.",
				SourceMessageID:  "msg-log-confirm",
				Content:          "Do not jump into large refactors first.",
				Summary:          "Do not jump into large refactors first.",
				Confidence:       0.94,
				Status:           domain.MemoryStatusPending,
				ExtractionMethod: domain.MemoryExtractionMethodLLM,
				CreatedBy:        "system",
				UpdatedBy:        "system",
				CreateTime:       now.Add(-time.Minute),
				UpdateTime:       now.Add(-time.Minute),
			}, nil
		}
		repo.updateFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		}
		service := newPreferenceCandidateLifecycleServiceForTest(repo, now)

		candidate, err := service.ConfirmPreferenceCandidate(ctx, DecidePreferenceCandidateInput{
			UserID:      "user-1",
			CandidateID: "cand-log-confirm",
		})
		if err != nil {
			t.Fatalf("ConfirmPreferenceCandidate returned error: %v", err)
		}
		if candidate.Status != domain.MemoryStatusActive {
			t.Fatalf("expected active candidate, got %+v", candidate)
		}

		entries := observed.All()
		if len(entries) != 2 {
			t.Fatalf("expected 2 log entries, got %d", len(entries))
		}
		if entries[0].Message != "long-term memory candidate confirmation requested" {
			t.Fatalf("unexpected request log: %+v", entries[0])
		}
		if entries[1].Message != "long-term memory candidate confirmed" {
			t.Fatalf("unexpected confirmed log: %+v", entries[1])
		}
		if entries[1].ContextMap()["status_to"] != domain.MemoryStatusActive {
			t.Fatalf("unexpected confirm log context: %+v", entries[1].ContextMap())
		}
	})

	t.Run("reject", func(t *testing.T) {
		now := time.Date(2026, 6, 13, 13, 5, 0, 0, time.UTC)
		core, observed := observer.New(zap.InfoLevel)
		ctx := fwlog.BindLogger(context.Background(), zap.New(core).Sugar())
		repo := newPreferenceCandidateLifecycleRepoStub()
		repo.getByID = func(context.Context, string) (domain.MemoryItem, error) {
			return domain.MemoryItem{
				ID:               "cand-log-reject",
				UserID:           "user-1",
				ScopeType:        domain.MemoryScopeGlobal,
				Namespace:        "global:global",
				MemoryType:       domain.MemoryTypePreference,
				Category:         domain.MemoryCategoryBehavior,
				CanonicalKey:     "behavior.avoid",
				ValueType:        domain.MemoryValueTypeText,
				ValueJSON:        "Do not jump into large refactors first.",
				DisplayValue:     "Do not jump into large refactors first.",
				SourceMessageID:  "msg-log-reject",
				Content:          "Do not jump into large refactors first.",
				Summary:          "Do not jump into large refactors first.",
				Confidence:       0.91,
				Status:           domain.MemoryStatusPending,
				ExtractionMethod: domain.MemoryExtractionMethodLLM,
				CreatedBy:        "system",
				UpdatedBy:        "system",
				CreateTime:       now.Add(-time.Minute),
				UpdateTime:       now.Add(-time.Minute),
			}, nil
		}
		repo.updateFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		}
		service := newPreferenceCandidateLifecycleServiceForTest(repo, now)

		candidate, err := service.RejectPreferenceCandidate(ctx, DecidePreferenceCandidateInput{
			UserID:      "user-1",
			CandidateID: "cand-log-reject",
		})
		if err != nil {
			t.Fatalf("RejectPreferenceCandidate returned error: %v", err)
		}
		if candidate.Status != domain.MemoryStatusRejected {
			t.Fatalf("expected rejected candidate, got %+v", candidate)
		}

		entries := observed.All()
		if len(entries) != 2 {
			t.Fatalf("expected 2 log entries, got %d", len(entries))
		}
		if entries[0].Message != "long-term memory candidate rejection requested" {
			t.Fatalf("unexpected request log: %+v", entries[0])
		}
		if entries[1].Message != "long-term memory candidate rejected by user" {
			t.Fatalf("unexpected rejected log: %+v", entries[1])
		}
		if entries[1].ContextMap()["status_to"] != domain.MemoryStatusRejected {
			t.Fatalf("unexpected rejection log context: %+v", entries[1].ContextMap())
		}
	})
}

func newPreferenceCandidateLifecycleServiceForTest(repo memoryItemRepoStub, now time.Time) *PreferenceCandidateLifecycleService {
	memoryService := NewMemoryService(repo, MemoryServiceOptions{})
	memoryService.now = func() time.Time { return now }
	return NewPreferenceCandidateLifecycleService(memoryService)
}

func newPreferenceCandidateLifecycleRepoStub() memoryItemRepoStub {
	return memoryItemRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		updateFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			return item, nil
		},
		getByID: func(context.Context, string) (domain.MemoryItem, error) {
			return domain.MemoryItem{}, nil
		},
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return nil, nil
		},
		countFn: func(context.Context, port.MemoryItemListFilter) (int64, error) {
			return 0, nil
		},
		listActiveByKeyFn: func(context.Context, string, string, string, string) ([]domain.MemoryItem, error) {
			return nil, nil
		},
	}
}

func assertLongTermMemoryEventCount(t *testing.T, snapshot cachemetrics.MetricsSnapshot, layer string, outcome string, want int64) {
	t.Helper()

	for _, event := range snapshot.Events {
		if event.CacheKind != longtermmemoryobs.CacheKindLongTermMemory {
			continue
		}
		if event.Layer == layer && event.Outcome == outcome {
			if event.Count != want {
				t.Fatalf("event %s/%s count=%d want=%d snapshot=%+v", layer, outcome, event.Count, want, snapshot.Events)
			}
			return
		}
	}
	if want == 0 {
		return
	}
	t.Fatalf("missing event %s/%s in snapshot=%+v", layer, outcome, snapshot.Events)
}
