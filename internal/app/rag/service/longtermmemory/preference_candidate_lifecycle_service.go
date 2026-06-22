package longtermmemory

import (
	"context"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory/governance"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
	"local/rag-project/internal/framework/exception"
)

type PreferenceCandidateLifecycleService struct {
	memory *MemoryService
}

func NewPreferenceCandidateLifecycleService(memory *MemoryService) *PreferenceCandidateLifecycleService {
	return &PreferenceCandidateLifecycleService{memory: memory}
}

func (s *PreferenceCandidateLifecycleService) PersistPendingPreferenceCandidate(ctx context.Context, input PersistPreferenceCandidateInput) (PreferenceCandidate, error) {
	if s == nil || s.memory == nil || s.memory.repo == nil {
		return PreferenceCandidate{}, exception.NewServiceException("memory item repository is required", nil)
	}
	userID, candidate, err := normalizePersistPreferenceCandidateInput(input)
	if err != nil {
		return PreferenceCandidate{}, err
	}

	item, err := buildPendingPreferenceMemoryItem(userID, candidate, s.memory.now())
	if err != nil {
		return PreferenceCandidate{}, err
	}

	var created domain.MemoryItem
	err = s.memory.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
		saved, err := repo.Create(ctx, item)
		if err != nil {
			return exception.NewServiceException("failed to create preference candidate", err)
		}
		created = saved
		return nil
	})
	if err != nil {
		return PreferenceCandidate{}, err
	}
	longtermmemoryobs.Record(s.memory.cacheMetrics, longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomePendingPersisted)
	longtermmemoryobs.LogCandidatePersisted(ctx, userID, created.ID, created.CanonicalKey, created.Confidence)
	return mapMemoryItemToPreferenceCandidate(created)
}

func (s *PreferenceCandidateLifecycleService) TryPersistPendingPreferenceCandidate(ctx context.Context, input PersistPreferenceCandidateInput) (PreferenceCandidate, bool) {
	candidate, err := s.PersistPendingPreferenceCandidate(ctx, input)
	if err != nil {
		return PreferenceCandidate{}, false
	}
	return candidate, true
}

func (s *PreferenceCandidateLifecycleService) ListPendingPreferenceCandidates(ctx context.Context, input ListPreferenceCandidatesInput) (PreferenceCandidatePageResult, error) {
	if s == nil || s.memory == nil || s.memory.repo == nil {
		return PreferenceCandidatePageResult{}, exception.NewServiceException("memory item repository is required", nil)
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return PreferenceCandidatePageResult{}, exception.NewClientException("user id is required", nil)
	}

	page := input.Page
	if page <= 0 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPreferenceCandidatePageSize
	}
	if pageSize > MaxPreferenceCandidatePageSize {
		pageSize = MaxPreferenceCandidatePageSize
	}

	filter := port.MemoryItemListFilter{
		UserID:        userID,
		ScopeTypes:    []string{domain.MemoryScopeGlobal},
		MemoryTypes:   []string{domain.MemoryTypePreference},
		CanonicalKeys: Phase1PreferenceCanonicalKeys(),
		Statuses:      []string{domain.MemoryStatusPending},
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}
	items, err := s.memory.repo.List(ctx, filter)
	if err != nil {
		return PreferenceCandidatePageResult{}, exception.NewServiceException("failed to list pending preference candidates", err)
	}
	total, err := s.memory.repo.Count(ctx, filterWithoutPaging(filter))
	if err != nil {
		return PreferenceCandidatePageResult{}, exception.NewServiceException("failed to count pending preference candidates", err)
	}

	result := PreferenceCandidatePageResult{
		Items:    make([]PreferenceCandidate, 0, len(items)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for _, item := range items {
		candidate, err := mapMemoryItemToPreferenceCandidate(item)
		if err != nil {
			return PreferenceCandidatePageResult{}, err
		}
		result.Items = append(result.Items, candidate)
	}
	return result, nil
}

func filterWithoutPaging(filter port.MemoryItemListFilter) port.MemoryItemListFilter {
	filter.Offset = 0
	filter.Limit = 0
	return filter
}

func (s *PreferenceCandidateLifecycleService) ConfirmPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	if s == nil || s.memory == nil || s.memory.repo == nil {
		return PreferenceCandidate{}, exception.NewServiceException("memory item repository is required", nil)
	}
	userID := strings.TrimSpace(input.UserID)
	candidateID := strings.TrimSpace(input.CandidateID)
	if userID == "" {
		return PreferenceCandidate{}, exception.NewClientException("user id is required", nil)
	}
	if candidateID == "" {
		return PreferenceCandidate{}, exception.NewClientException("preference candidate id is required", nil)
	}
	longtermmemoryobs.LogCandidateConfirmationRequested(ctx, userID, candidateID)

	now := s.memory.now()
	var resolved domain.MemoryItem
	var shouldBump bool
	err := s.memory.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
		pending, err := s.loadOwnedPreferenceCandidateForMutation(ctx, repo, userID, candidateID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(pending.Status) != domain.MemoryStatusPending {
			longtermmemoryobs.LogCandidateConfirmationRejected(ctx, userID, candidateID, "not_pending")
			return exception.NewClientException("preference candidate is not pending", nil)
		}

		decision, err := buildCandidateGateDecision(pending)
		if err != nil {
			longtermmemoryobs.LogCandidateConfirmationRejected(ctx, userID, candidateID, err.Error())
			return err
		}
		if decision.Spec == nil {
			longtermmemoryobs.LogCandidateConfirmationRejected(ctx, userID, candidateID, "unsupported_canonical_key")
			return exception.NewClientException("unsupported phase 1 preference canonical key", nil)
		}

		active, err := repo.ListActiveByCanonicalKey(ctx, userID, pending.ScopeType, pending.ScopeID, pending.CanonicalKey)
		if err != nil {
			return exception.NewServiceException("failed to load active preference memories", err)
		}

		switch decision.Spec.Cardinality {
		case governance.MemoryCardinalitySingle:
			if len(active) > 1 {
				longtermmemoryobs.LogCandidateConfirmationRejected(ctx, userID, candidateID, "multiple_active_conflicts")
				return exception.NewServiceException("multiple active memory items detected for single-valued canonical key", nil)
			}
			if len(active) == 1 {
				existing := active[0]
				if governance.MemoryItemsEquivalent(existing, pending) {
					merged := mergeConfirmedPreferenceIntoExisting(existing, pending, userID, now)
					updatedExisting, err := repo.Update(ctx, merged)
					if err != nil {
						return exception.NewServiceException("failed to refresh active preference memory", err)
					}
					rejectedPending := rejectPendingPreferenceItem(pending, userID, now)
					if _, err := repo.Update(ctx, rejectedPending); err != nil {
						return exception.NewServiceException("failed to reject duplicate preference candidate", err)
					}
					resolved = updatedExisting
					shouldBump = true
					return nil
				}
				superseded := supersedeActivePreferenceItem(existing, userID, now)
				if _, err := repo.Update(ctx, superseded); err != nil {
					return exception.NewServiceException("failed to supersede active preference memory", err)
				}
				pending = activatePendingPreferenceItem(pending, userID, now, existing.ID)
			} else {
				pending = activatePendingPreferenceItem(pending, userID, now, "")
			}
		default:
			for _, existing := range active {
				if !governance.MemoryItemsEquivalent(existing, pending) {
					continue
				}
				merged := mergeConfirmedPreferenceIntoExisting(existing, pending, userID, now)
				updatedExisting, err := repo.Update(ctx, merged)
				if err != nil {
					return exception.NewServiceException("failed to refresh active preference memory", err)
				}
				rejectedPending := rejectPendingPreferenceItem(pending, userID, now)
				if _, err := repo.Update(ctx, rejectedPending); err != nil {
					return exception.NewServiceException("failed to reject duplicate preference candidate", err)
				}
				resolved = updatedExisting
				shouldBump = true
				return nil
			}
			if strings.TrimSpace(pending.CanonicalKey) == "behavior.avoid" && len(active) >= behaviorAvoidActiveLimitPhase1 {
				longtermmemoryobs.LogCandidateConfirmationRejected(ctx, userID, candidateID, preferenceCandidateQuotaExceeded)
				return exception.NewClientException(preferenceCandidateQuotaExceeded, nil)
			}
			pending = activatePendingPreferenceItem(pending, userID, now, "")
		}

		updated, err := repo.Update(ctx, pending)
		if err != nil {
			return exception.NewServiceException("failed to confirm preference candidate", err)
		}
		resolved = updated
		shouldBump = true
		return nil
	})
	if err != nil {
		return PreferenceCandidate{}, err
	}
	if shouldBump {
		s.memory.bumpRecallCacheVersion(ctx, resolved)
	}
	longtermmemoryobs.Record(s.memory.cacheMetrics, longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomeCandidateConfirmed)
	longtermmemoryobs.LogCandidateConfirmed(ctx, userID, resolved.ID, resolved.CanonicalKey, domain.MemoryStatusPending, resolved.Status)
	return mapMemoryItemToPreferenceCandidate(resolved)
}

func (s *PreferenceCandidateLifecycleService) RejectPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	if s == nil || s.memory == nil || s.memory.repo == nil {
		return PreferenceCandidate{}, exception.NewServiceException("memory item repository is required", nil)
	}
	userID := strings.TrimSpace(input.UserID)
	candidateID := strings.TrimSpace(input.CandidateID)
	if userID == "" {
		return PreferenceCandidate{}, exception.NewClientException("user id is required", nil)
	}
	if candidateID == "" {
		return PreferenceCandidate{}, exception.NewClientException("preference candidate id is required", nil)
	}
	longtermmemoryobs.LogCandidateRejectionRequested(ctx, userID, candidateID)

	now := s.memory.now()
	var resolved domain.MemoryItem
	err := s.memory.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
		pending, err := s.loadOwnedPreferenceCandidateForMutation(ctx, repo, userID, candidateID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(pending.Status) != domain.MemoryStatusPending {
			longtermmemoryobs.LogCandidateRejectionFailed(ctx, userID, candidateID, "not_pending")
			return exception.NewClientException("preference candidate is not pending", nil)
		}
		rejected := rejectPendingPreferenceItem(pending, userID, now)
		updated, err := repo.Update(ctx, rejected)
		if err != nil {
			return exception.NewServiceException("failed to reject preference candidate", err)
		}
		resolved = updated
		return nil
	})
	if err != nil {
		return PreferenceCandidate{}, err
	}
	longtermmemoryobs.Record(s.memory.cacheMetrics, longtermmemoryobs.LayerLifecycle, longtermmemoryobs.OutcomeCandidateRejected)
	longtermmemoryobs.LogCandidateRejected(ctx, userID, resolved.ID, resolved.CanonicalKey, domain.MemoryStatusPending, resolved.Status)
	return mapMemoryItemToPreferenceCandidate(resolved)
}

func (s *PreferenceCandidateLifecycleService) loadOwnedPreferenceCandidateForMutation(
	ctx context.Context,
	repo port.MemoryItemRepository,
	userID string,
	candidateID string,
) (domain.MemoryItem, error) {
	item, err := repo.GetByID(ctx, candidateID)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to load preference candidate", err)
	}
	if item.ID == "" || strings.TrimSpace(item.UserID) != strings.TrimSpace(userID) {
		return domain.MemoryItem{}, exception.NewClientException("preference candidate not found", nil)
	}
	if _, err := mapMemoryItemToPreferenceCandidate(item); err != nil {
		return domain.MemoryItem{}, exception.NewClientException("preference candidate not found", nil)
	}
	return item, nil
}

var _ PreferenceCandidateService = (*PreferenceCandidateLifecycleService)(nil)
