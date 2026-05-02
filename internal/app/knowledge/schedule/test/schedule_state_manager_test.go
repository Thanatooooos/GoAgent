package schedule_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
)

type stateScheduleRepository struct {
	updateWhereFn func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error)
}

func (s stateScheduleRepository) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stateScheduleRepository) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stateScheduleRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	if s.updateWhereFn != nil {
		return s.updateWhereFn(ctx, cond, patch)
	}
	return 0, nil
}

func (s stateScheduleRepository) Delete(ctx context.Context, id string) error { return nil }

func (s stateScheduleRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (s stateScheduleRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stateScheduleRepository) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stateScheduleRepository) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	return nil, nil
}

func (s stateScheduleRepository) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return false, nil
}

func (s stateScheduleRepository) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	return false, nil
}

func (s stateScheduleRepository) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	return false, nil
}

type stateExecRepository struct {
	updateWhereFn func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error)
}

func (s stateExecRepository) Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s stateExecRepository) Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s stateExecRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
	if s.updateWhereFn != nil {
		return s.updateWhereFn(ctx, cond, patch)
	}
	return 0, nil
}

func (s stateExecRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s stateExecRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (s stateExecRepository) List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error) {
	return nil, nil
}

func TestScheduleStateManagerMarkSuccessIfOwned(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Minute)
	next := now.Add(time.Hour)
	lease := domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"}
	state := domain.KnowledgeDocumentScheduleStateContext{
		ScheduleID:  "schedule-1",
		ExecID:      "exec-1",
		CronExpr:    "0 */5 * * * *",
		StartTime:   start,
		NextRunTime: &next,
	}

	var scheduleCond port.KnowledgeDocumentScheduleConditions
	var schedulePatch port.KnowledgeDocumentSchedulePatch
	var execCond port.KnowledgeDocumentScheduleExecConditions
	var execPatch port.KnowledgeDocumentScheduleExecPatch

	manager := schedule.NewScheduleStateManagerWithOptions(
		stateScheduleRepository{
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
				scheduleCond = cond
				schedulePatch = patch
				return 1, nil
			},
		},
		stateExecRepository{
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
				execCond = cond
				execPatch = patch
				return 1, nil
			},
		},
		schedule.ScheduleStateManagerOptions{Now: func() time.Time { return now }},
	)

	updated, err := manager.MarkSuccessIfOwned(
		context.Background(),
		lease,
		state,
		schedule.ScheduleFetchResult{Message: "changed", ETag: "etag-1", LastModified: "last-modified", ContentHash: "hash-1"},
		schedule.StoredFileDTO{OriginFileName: "demo.md", Size: 42},
	)
	if err != nil {
		t.Fatalf("MarkSuccessIfOwned() error = %v", err)
	}
	if !updated {
		t.Fatal("MarkSuccessIfOwned() should report schedule updated")
	}
	if scheduleCond.ID != "schedule-1" || scheduleCond.LockOwnerEQ != "owner-1" {
		t.Fatalf("schedule update should be guarded by lease ownership, got %+v", scheduleCond)
	}
	if schedulePatch.LastStatus.Value != domain.KnowledgeDocumentScheduleRunStatusSuccess {
		t.Fatalf("unexpected schedule status patch: %+v", schedulePatch.LastStatus)
	}
	if execCond.ID != "exec-1" || execPatch.Status.Value != domain.KnowledgeDocumentScheduleRunStatusSuccess {
		t.Fatalf("unexpected exec update cond=%+v patch=%+v", execCond, execPatch)
	}
	if execPatch.FileSize.Value == nil || *execPatch.FileSize.Value != 42 {
		t.Fatalf("unexpected exec file size patch: %+v", execPatch.FileSize.Value)
	}
}

func TestScheduleStateManagerAddsLeaseLostNoteWhenScheduleNotOwned(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	var execPatch port.KnowledgeDocumentScheduleExecPatch
	manager := schedule.NewScheduleStateManagerWithOptions(
		stateScheduleRepository{
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
				return 0, nil
			},
		},
		stateExecRepository{
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
				execPatch = patch
				return 1, nil
			},
		},
		schedule.ScheduleStateManagerOptions{Now: func() time.Time { return now }},
	)

	updated, err := manager.MarkSkippedIfOwned(
		context.Background(),
		domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"},
		domain.KnowledgeDocumentScheduleStateContext{ExecID: "exec-1", StartTime: now},
		"remote file unchanged",
	)
	if err != nil {
		t.Fatalf("MarkSkippedIfOwned() error = %v", err)
	}
	if updated {
		t.Fatal("MarkSkippedIfOwned() should report schedule not updated")
	}
	if !strings.Contains(execPatch.Message.Value, "schedule lock lost") {
		t.Fatalf("expected lease lost note in exec message, got %q", execPatch.Message.Value)
	}
}

func TestScheduleStateManagerDisableIfOwned(t *testing.T) {
	t.Parallel()

	var schedulePatch port.KnowledgeDocumentSchedulePatch
	manager := schedule.NewScheduleStateManager(
		stateScheduleRepository{
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
				schedulePatch = patch
				return 1, nil
			},
		},
		stateExecRepository{},
	)

	updated, err := manager.DisableIfOwned(
		context.Background(),
		domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"},
		"invalid cron",
	)
	if err != nil {
		t.Fatalf("DisableIfOwned() error = %v", err)
	}
	if !updated {
		t.Fatal("DisableIfOwned() should report schedule updated")
	}
	if !schedulePatch.Enabled.Set || schedulePatch.Enabled.Value {
		t.Fatalf("schedule should be disabled, got %+v", schedulePatch.Enabled)
	}
	if !schedulePatch.NextRunTime.Set || schedulePatch.NextRunTime.Value != nil {
		t.Fatalf("next run time should be cleared, got %+v", schedulePatch.NextRunTime)
	}
}
