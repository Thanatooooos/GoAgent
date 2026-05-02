package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

const (
	scheduleStateLeaseLostNote = " (schedule lock lost; schedule state was not written back)"
	scheduleStateMaxMessageLen = 512
)

type ScheduleFetchResult struct {
	Message      string
	ETag         string
	LastModified string
	ContentHash  string
}

type ScheduleStateManagerOptions struct {
	Now func() time.Time
}

type ScheduleStateManager struct {
	scheduleRepo port.KnowledgeDocumentScheduleRepository
	execRepo     port.KnowledgeDocumentScheduleExecRepository
	now          func() time.Time
}

func NewScheduleStateManager(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	execRepo port.KnowledgeDocumentScheduleExecRepository,
) *ScheduleStateManager {
	return NewScheduleStateManagerWithOptions(scheduleRepo, execRepo, ScheduleStateManagerOptions{})
}

func NewScheduleStateManagerWithOptions(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	execRepo port.KnowledgeDocumentScheduleExecRepository,
	options ScheduleStateManagerOptions,
) *ScheduleStateManager {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &ScheduleStateManager{
		scheduleRepo: scheduleRepo,
		execRepo:     execRepo,
		now:          now,
	}
}

func (m *ScheduleStateManager) MarkSkippedFetchIfOwned(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	state domain.KnowledgeDocumentScheduleStateContext,
	fetchResult ScheduleFetchResult,
) (bool, error) {
	updatedAt := m.currentTime()
	scheduleUpdated, err := m.updateScheduleIfOwned(ctx, lease, port.KnowledgeDocumentSchedulePatch{
		CronExpr:        port.ValueOf(state.CronExpr),
		LastRunTime:     port.ValueOf(timePointer(state.StartTime)),
		NextRunTime:     port.ValueOf(state.NextRunTime),
		LastStatus:      port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSkipped),
		LastError:       port.ValueOf(truncateScheduleStateMessage(fetchResult.Message)),
		LastETag:        port.ValueOf(fetchResult.ETag),
		LastModified:    port.ValueOf(fetchResult.LastModified),
		LastContentHash: port.ValueOf(fetchResult.ContentHash),
		UpdatedAt:       port.ValueOf(updatedAt),
	})
	if err != nil {
		return false, err
	}

	err = m.updateExecIfPresent(ctx, state.ExecID, port.KnowledgeDocumentScheduleExecPatch{
		Status:       port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSkipped),
		Message:      port.ValueOf(m.withLeaseNote(fetchResult.Message, scheduleUpdated)),
		EndTime:      port.ValueOf(timePointer(updatedAt)),
		ContentHash:  port.ValueOf(fetchResult.ContentHash),
		ETag:         port.ValueOf(fetchResult.ETag),
		LastModified: port.ValueOf(fetchResult.LastModified),
		UpdatedAt:    port.ValueOf(updatedAt),
	})
	return scheduleUpdated, err
}

func (m *ScheduleStateManager) MarkSkippedIfOwned(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	state domain.KnowledgeDocumentScheduleStateContext,
	message string,
) (bool, error) {
	updatedAt := m.currentTime()
	scheduleUpdated, err := m.updateScheduleIfOwned(ctx, lease, port.KnowledgeDocumentSchedulePatch{
		CronExpr:    port.ValueOf(state.CronExpr),
		LastRunTime: port.ValueOf(timePointer(state.StartTime)),
		NextRunTime: port.ValueOf(state.NextRunTime),
		LastStatus:  port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSkipped),
		LastError:   port.ValueOf(truncateScheduleStateMessage(message)),
		UpdatedAt:   port.ValueOf(updatedAt),
	})
	if err != nil {
		return false, err
	}

	err = m.updateExecIfPresent(ctx, state.ExecID, port.KnowledgeDocumentScheduleExecPatch{
		Status:    port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSkipped),
		Message:   port.ValueOf(m.withLeaseNote(message, scheduleUpdated)),
		EndTime:   port.ValueOf(timePointer(updatedAt)),
		UpdatedAt: port.ValueOf(updatedAt),
	})
	return scheduleUpdated, err
}

func (m *ScheduleStateManager) MarkSuccessIfOwned(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	state domain.KnowledgeDocumentScheduleStateContext,
	fetchResult ScheduleFetchResult,
	stored StoredFileDTO,
) (bool, error) {
	endTime := m.currentTime()
	scheduleUpdated, err := m.updateScheduleIfOwned(ctx, lease, port.KnowledgeDocumentSchedulePatch{
		CronExpr:        port.ValueOf(state.CronExpr),
		LastRunTime:     port.ValueOf(timePointer(state.StartTime)),
		NextRunTime:     port.ValueOf(state.NextRunTime),
		LastSuccessTime: port.ValueOf(timePointer(endTime)),
		LastStatus:      port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSuccess),
		LastError:       port.ValueOf(""),
		LastETag:        port.ValueOf(fetchResult.ETag),
		LastModified:    port.ValueOf(fetchResult.LastModified),
		LastContentHash: port.ValueOf(fetchResult.ContentHash),
		UpdatedAt:       port.ValueOf(endTime),
	})
	if err != nil {
		return false, err
	}

	err = m.updateExecIfPresent(ctx, state.ExecID, port.KnowledgeDocumentScheduleExecPatch{
		Status:       port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSuccess),
		Message:      port.ValueOf(m.withLeaseNote("refresh success", scheduleUpdated)),
		EndTime:      port.ValueOf(timePointer(endTime)),
		FileName:     port.ValueOf(stored.OriginFileName),
		FileSize:     port.ValueOf(int64Pointer(stored.Size)),
		ContentHash:  port.ValueOf(fetchResult.ContentHash),
		ETag:         port.ValueOf(fetchResult.ETag),
		LastModified: port.ValueOf(fetchResult.LastModified),
		UpdatedAt:    port.ValueOf(endTime),
	})
	return scheduleUpdated, err
}

func (m *ScheduleStateManager) MarkFailedIfOwned(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	state domain.KnowledgeDocumentScheduleStateContext,
	errorMessage string,
) (bool, error) {
	updatedAt := m.currentTime()
	message := truncateScheduleStateMessage(errorMessage)
	scheduleUpdated, err := m.updateScheduleIfOwned(ctx, lease, port.KnowledgeDocumentSchedulePatch{
		CronExpr:    port.ValueOf(state.CronExpr),
		LastRunTime: port.ValueOf(timePointer(state.StartTime)),
		NextRunTime: port.ValueOf(state.NextRunTime),
		LastStatus:  port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusFailed),
		LastError:   port.ValueOf(message),
		UpdatedAt:   port.ValueOf(updatedAt),
	})
	if err != nil {
		return false, err
	}

	err = m.updateExecIfPresent(ctx, state.ExecID, port.KnowledgeDocumentScheduleExecPatch{
		Status:    port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusFailed),
		Message:   port.ValueOf(m.withLeaseNote(message, scheduleUpdated)),
		EndTime:   port.ValueOf(timePointer(updatedAt)),
		UpdatedAt: port.ValueOf(updatedAt),
	})
	return scheduleUpdated, err
}

func (m *ScheduleStateManager) DisableIfOwned(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, reason string) (bool, error) {
	return m.updateScheduleIfOwned(ctx, lease, port.KnowledgeDocumentSchedulePatch{
		Enabled:     port.ValueOf(false),
		NextRunTime: port.ValueOf((*time.Time)(nil)),
		LastStatus:  port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusFailed),
		LastError:   port.ValueOf(truncateScheduleStateMessage(reason)),
		UpdatedAt:   port.ValueOf(m.currentTime()),
	})
}

func (m *ScheduleStateManager) MarkLeaseLost(ctx context.Context, state domain.KnowledgeDocumentScheduleStateContext, stage string) error {
	if strings.TrimSpace(state.ExecID) == "" {
		return nil
	}

	message := "schedule lock lost; execution stopped"
	if strings.TrimSpace(stage) != "" {
		message += ": " + strings.TrimSpace(stage)
	}
	now := m.currentTime()
	return m.updateExecIfPresent(ctx, state.ExecID, port.KnowledgeDocumentScheduleExecPatch{
		Status:    port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusFailed),
		Message:   port.ValueOf(truncateScheduleStateMessage(message)),
		EndTime:   port.ValueOf(timePointer(now)),
		UpdatedAt: port.ValueOf(now),
	})
}

func (m *ScheduleStateManager) MarkSuccessExecOnly(
	ctx context.Context,
	state domain.KnowledgeDocumentScheduleStateContext,
	stored *StoredFileDTO,
	contentHash string,
	etag string,
	lastModified string,
	message string,
) error {
	if strings.TrimSpace(state.ExecID) == "" {
		return nil
	}

	now := m.currentTime()
	patch := port.KnowledgeDocumentScheduleExecPatch{
		Status:       port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusSuccess),
		Message:      port.ValueOf(truncateScheduleStateMessage(message)),
		EndTime:      port.ValueOf(timePointer(now)),
		ContentHash:  port.ValueOf(contentHash),
		ETag:         port.ValueOf(etag),
		LastModified: port.ValueOf(lastModified),
		UpdatedAt:    port.ValueOf(now),
	}
	if stored != nil {
		patch.FileName = port.ValueOf(stored.OriginFileName)
		patch.FileSize = port.ValueOf(int64Pointer(stored.Size))
	}
	return m.updateExecIfPresent(ctx, state.ExecID, patch)
}

func (m *ScheduleStateManager) updateScheduleIfOwned(
	ctx context.Context,
	lease domain.KnowledgeDocumentScheduleLockLease,
	patch port.KnowledgeDocumentSchedulePatch,
) (bool, error) {
	if m == nil || m.scheduleRepo == nil || !validScheduleStateLease(lease) {
		return false, nil
	}
	rows, err := m.scheduleRepo.UpdateWhere(ctx, port.KnowledgeDocumentScheduleConditions{
		ID:          lease.ScheduleID,
		LockOwnerEQ: lease.LockToken,
	}, patch)
	if err != nil {
		return false, fmt.Errorf("update schedule state if owned: %w", err)
	}
	return rows > 0, nil
}

func (m *ScheduleStateManager) updateExecIfPresent(
	ctx context.Context,
	execID string,
	patch port.KnowledgeDocumentScheduleExecPatch,
) error {
	if strings.TrimSpace(execID) == "" {
		return nil
	}
	if m == nil || m.execRepo == nil {
		return fmt.Errorf("knowledge document schedule exec repository is required")
	}
	if _, err := m.execRepo.UpdateWhere(ctx, port.KnowledgeDocumentScheduleExecConditions{ID: execID}, patch); err != nil {
		return fmt.Errorf("update schedule exec state: %w", err)
	}
	return nil
}

func (m *ScheduleStateManager) withLeaseNote(message string, scheduleUpdated bool) string {
	if scheduleUpdated {
		return truncateScheduleStateMessage(message)
	}
	baseMessage := strings.TrimSpace(message)
	if baseMessage == "" {
		baseMessage = "execution completed"
	}
	return truncateScheduleStateMessage(baseMessage + scheduleStateLeaseLostNote)
}

func (m *ScheduleStateManager) currentTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now()
	}
	return m.now()
}

func validScheduleStateLease(lease domain.KnowledgeDocumentScheduleLockLease) bool {
	return strings.TrimSpace(lease.ScheduleID) != "" && strings.TrimSpace(lease.LockToken) != ""
}

func truncateScheduleStateMessage(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= scheduleStateMaxMessageLen {
		return trimmed
	}
	return trimmed[:scheduleStateMaxMessageLen]
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}
