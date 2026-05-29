package longtermmemory

import (
	"context"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultMemoryMaintenanceDeleteRetention = 90 * 24 * time.Hour
	defaultMemoryMaintenanceExpireBatchSize = 200
	defaultMemoryMaintenanceDeleteBatchSize = 200
	defaultMemoryMaintenanceUpdatedBy       = "memory-maintenance"
)

func (s *MemoryService) RunMaintenance(ctx context.Context, input MaintenanceInput) (MaintenanceResult, error) {
	if s == nil || s.repo == nil {
		return MaintenanceResult{}, exception.NewServiceException("memory item repository is required", nil)
	}

	now := s.now()
	expireBatchSize := input.ExpireBatchSize
	if expireBatchSize <= 0 {
		expireBatchSize = defaultMemoryMaintenanceExpireBatchSize
	}
	deleteBatchSize := input.DeleteBatchSize
	if deleteBatchSize <= 0 {
		deleteBatchSize = defaultMemoryMaintenanceDeleteBatchSize
	}
	deleteRetention := input.DeleteRetention
	if deleteRetention <= 0 {
		deleteRetention = defaultMemoryMaintenanceDeleteRetention
	}
	updatedBy := strings.TrimSpace(input.UpdatedBy)
	if updatedBy == "" {
		updatedBy = defaultMemoryMaintenanceUpdatedBy
	}

	result := MaintenanceResult{}
	expiredItems, err := s.listExpiredMaintenanceCandidates(ctx, now, expireBatchSize)
	if err != nil {
		s.recordMaintenanceFailure()
		return MaintenanceResult{}, err
	}
	if len(expiredItems) > 0 {
		expiredIDs := make([]string, 0, len(expiredItems))
		for _, item := range expiredItems {
			if id := strings.TrimSpace(item.ID); id != "" {
				expiredIDs = append(expiredIDs, id)
			}
		}
		if len(expiredIDs) > 0 {
			err = s.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
				affected, err := repo.ExpireByIDs(ctx, expiredIDs, updatedBy, now)
				if err != nil {
					return exception.NewServiceException("failed to expire due memory items", err)
				}
				result.ExpiredCount = affected
				return nil
			})
			if err != nil {
				s.recordMaintenanceFailure()
				return MaintenanceResult{}, err
			}
			s.bumpRecallCacheVersions(ctx, expiredItems)
		}
	}

	cutoff := now.Add(-deleteRetention)
	deletedCount, err := s.repo.DeleteByStatusesUpdatedBefore(
		ctx,
		[]string{domain.MemoryStatusExpired, domain.MemoryStatusSuperseded},
		cutoff,
		deleteBatchSize,
	)
	if err != nil {
		s.recordMaintenanceFailure()
		return MaintenanceResult{}, exception.NewServiceException("failed to delete stale memory items", err)
	}
	result.DeletedCount = deletedCount
	s.recordMaintenanceRun(result)
	return result, nil
}

func (s *MemoryService) listExpiredMaintenanceCandidates(ctx context.Context, now time.Time, limit int) ([]domain.MemoryItem, error) {
	items, err := s.repo.List(ctx, port.MemoryItemListFilter{
		Statuses:      []string{domain.MemoryStatusActive, domain.MemoryStatusPending},
		ExpiresBefore: &now,
		ListOptions: port.ListOptions{
			Limit: limit,
		},
	})
	if err != nil {
		return nil, exception.NewServiceException("failed to list due memory items", err)
	}
	return items, nil
}

func (s *MemoryService) recordMaintenanceRun(result MaintenanceResult) {
	if s == nil || s.cacheMetrics == nil {
		return
	}
	s.cacheMetrics.RecordMaintenanceRun(result.ExpiredCount, result.DeletedCount)
}

func (s *MemoryService) recordMaintenanceFailure() {
	if s == nil || s.cacheMetrics == nil {
		return
	}
	s.cacheMetrics.RecordMaintenanceFailure()
}

func (s *MemoryService) bumpRecallCacheVersions(ctx context.Context, items []domain.MemoryItem) {
	if s == nil || len(items) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		userID := strings.TrimSpace(item.UserID)
		if userID == "" {
			continue
		}
		scopeType := strings.TrimSpace(item.ScopeType)
		scopeID := strings.TrimSpace(item.ScopeID)
		key := userID + "|" + scopeType + "|" + scopeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		s.bumpRecallCacheVersion(ctx, item)
	}
}
