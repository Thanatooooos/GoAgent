package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
)

type stubScheduleLeaseProcessor struct {
	processFn func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error
}

func (s stubScheduleLeaseProcessor) Process(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
	if s.processFn != nil {
		return s.processFn(ctx, lease)
	}
	return nil
}

type stubScheduleTaskDispatcher struct {
	submitFn func(task func()) error
}

func (s stubScheduleTaskDispatcher) Submit(task func()) error {
	if s.submitFn != nil {
		return s.submitFn(task)
	}
	task()
	return nil
}

func TestKnowledgeDocumentScheduleJobScanAcquiresAndDispatchesDueSchedules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	leases := make([]domain.KnowledgeDocumentScheduleLockLease, 0)
	released := make([]domain.KnowledgeDocumentScheduleLockLease, 0)

	repo := stubScheduleRepository{
		listDueFn: func(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
			if !before.Equal(now) {
				t.Fatalf("unexpected scan time: %s", before)
			}
			if limit != 2 {
				t.Fatalf("unexpected scan limit: %d", limit)
			}
			return []domain.KnowledgeDocumentSchedule{
				{ID: "schedule-1"},
				{ID: ""},
				{ID: "schedule-2"},
			}, nil
		},
		tryAcquireLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, acquiredAt time.Time) (bool, error) {
			if lease.ScheduleID == "schedule-2" {
				return false, nil
			}
			return true, nil
		},
		releaseLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
			released = append(released, lease)
			return true, nil
		},
	}

	job := NewKnowledgeDocumentScheduleJobWithOptions(repo, DocumentStatusHelper{}, KnowledgeDocumentScheduleJobOptions{
		LockManager: NewScheduleLockManager(repo, ScheduleLockOptions{
			InstancePrefix: "node-1",
			TokenSuffix: func() string {
				return "token-1"
			},
		}),
		Processor: stubScheduleLeaseProcessor{
			processFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) error {
				leases = append(leases, lease)
				return nil
			},
		},
		Dispatcher: stubScheduleTaskDispatcher{},
		BatchSize:  2,
		Now: func() time.Time {
			return now
		},
	})

	if err := job.Scan(context.Background()); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(leases) != 1 || leases[0].ScheduleID != "schedule-1" {
		t.Fatalf("unexpected processed leases: %+v", leases)
	}
	if len(released) != 1 || released[0].ScheduleID != "schedule-1" {
		t.Fatalf("expected processed lease to be released, got %+v", released)
	}
}

func TestKnowledgeDocumentScheduleJobScanReleasesLeaseWhenDispatcherFails(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("queue full")
	var released domain.KnowledgeDocumentScheduleLockLease
	repo := stubScheduleRepository{
		listDueFn: func(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
			return []domain.KnowledgeDocumentSchedule{{ID: "schedule-1"}}, nil
		},
		tryAcquireLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, acquiredAt time.Time) (bool, error) {
			return true, nil
		},
		releaseLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
			released = lease
			return true, nil
		},
	}

	job := NewKnowledgeDocumentScheduleJobWithOptions(repo, DocumentStatusHelper{}, KnowledgeDocumentScheduleJobOptions{
		LockManager: NewScheduleLockManager(repo, ScheduleLockOptions{}),
		Processor:   stubScheduleLeaseProcessor{},
		Dispatcher: stubScheduleTaskDispatcher{
			submitFn: func(task func()) error {
				return dispatchErr
			},
		},
	})

	err := job.Scan(context.Background())
	if !errors.Is(err, dispatchErr) {
		t.Fatalf("Scan() error = %v, want %v", err, dispatchErr)
	}
	if released.ScheduleID != "schedule-1" {
		t.Fatalf("expected acquired lease to be released, got %+v", released)
	}
}

func TestKnowledgeDocumentScheduleJobScanReleasesLeaseWhenProcessorMissing(t *testing.T) {
	t.Parallel()

	var released domain.KnowledgeDocumentScheduleLockLease
	repo := stubScheduleRepository{
		listDueFn: func(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
			return []domain.KnowledgeDocumentSchedule{{ID: "schedule-1"}}, nil
		},
		tryAcquireLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, acquiredAt time.Time) (bool, error) {
			return true, nil
		},
		releaseLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
			released = lease
			return true, nil
		},
	}

	job := NewKnowledgeDocumentScheduleJobWithOptions(repo, DocumentStatusHelper{}, KnowledgeDocumentScheduleJobOptions{
		LockManager: NewScheduleLockManager(repo, ScheduleLockOptions{}),
	})

	if err := job.Scan(context.Background()); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if released.ScheduleID != "schedule-1" {
		t.Fatalf("expected acquired lease to be released, got %+v", released)
	}
}
