package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type stubScheduleRepository struct {
	listDueFn        func(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error)
	tryAcquireLockFn func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error)
	renewLockFn      func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error)
	releaseLockFn    func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error)
}

func (s stubScheduleRepository) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return schedule, nil
}

func (s stubScheduleRepository) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return schedule, nil
}

func (s stubScheduleRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	return 0, nil
}

func (s stubScheduleRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (s stubScheduleRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (s stubScheduleRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stubScheduleRepository) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s stubScheduleRepository) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	if s.listDueFn != nil {
		return s.listDueFn(ctx, before, limit)
	}
	return nil, nil
}

func (s stubScheduleRepository) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	if s.tryAcquireLockFn != nil {
		return s.tryAcquireLockFn(ctx, lease, lockUntil, now)
	}
	return false, nil
}

func (s stubScheduleRepository) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	if s.renewLockFn != nil {
		return s.renewLockFn(ctx, lease, lockUntil)
	}
	return false, nil
}

func (s stubScheduleRepository) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	if s.releaseLockFn != nil {
		return s.releaseLockFn(ctx, lease)
	}
	return false, nil
}

func TestScheduleLockManagerTryAcquireUsesLeaseTokenAndMinimumTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	manager := NewScheduleLockManager(stubScheduleRepository{
		tryAcquireLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, acquiredAt time.Time) (bool, error) {
			if lease.ScheduleID != "schedule-1" {
				t.Fatalf("unexpected schedule id: %q", lease.ScheduleID)
			}
			if lease.LockToken != "node-1:token-1" {
				t.Fatalf("unexpected lock token: %q", lease.LockToken)
			}
			if !acquiredAt.Equal(now) {
				t.Fatalf("unexpected acquire time: %s", acquiredAt)
			}
			if want := now.Add(60 * time.Second); !lockUntil.Equal(want) {
				t.Fatalf("unexpected lock until: got %s want %s", lockUntil, want)
			}
			return true, nil
		},
	}, ScheduleLockOptions{
		LockSeconds:    30,
		InstancePrefix: "node-1",
		TokenSuffix: func() string {
			return "token-1"
		},
	})

	lease, ok, err := manager.TryAcquire(context.Background(), " schedule-1 ", now)
	if err != nil {
		t.Fatalf("TryAcquire() error = %v", err)
	}
	if !ok {
		t.Fatal("TryAcquire() should return ok")
	}
	if lease.LockToken != "node-1:token-1" {
		t.Fatalf("unexpected returned lease: %+v", lease)
	}
}

func TestScheduleLockManagerRenewAndReleaseRequireLeaseOwnership(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	lease := domain.NewKnowledgeDocumentScheduleLockLease("schedule-1", "node-1:token-1")
	manager := NewScheduleLockManager(stubScheduleRepository{
		renewLockFn: func(ctx context.Context, got domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
			if got != lease {
				t.Fatalf("unexpected renew lease: %+v", got)
			}
			if want := now.Add(120 * time.Second); !lockUntil.Equal(want) {
				t.Fatalf("unexpected renewed lock until: got %s want %s", lockUntil, want)
			}
			return true, nil
		},
		releaseLockFn: func(ctx context.Context, got domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
			if got != lease {
				t.Fatalf("unexpected release lease: %+v", got)
			}
			return true, nil
		},
	}, ScheduleLockOptions{
		LockSeconds: 120,
		Now: func() time.Time {
			return now
		},
	})

	renewed, err := manager.Renew(context.Background(), lease)
	if err != nil || !renewed {
		t.Fatalf("Renew() = %v, %v", renewed, err)
	}

	released, err := manager.Release(context.Background(), lease)
	if err != nil || !released {
		t.Fatalf("Release() = %v, %v", released, err)
	}
}

func TestScheduleLockManagerHeartbeatMarksLostWhenRenewReturnsFalse(t *testing.T) {
	t.Parallel()

	lease := domain.NewKnowledgeDocumentScheduleLockLease("schedule-1", "node-1:token-1")
	manager := NewScheduleLockManager(stubScheduleRepository{
		renewLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
			return false, nil
		},
	}, ScheduleLockOptions{LockSeconds: 60})
	heartbeat := newScheduleLockHeartbeat(lease, time.Now(), 60*time.Second)

	manager.doHeartbeat(heartbeat)

	if !heartbeat.IsLost() {
		t.Fatal("heartbeat should be marked lost when renew returns false")
	}
}

func TestScheduleLockManagerHeartbeatKeepsRetryingRenewErrorWithinTTL(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	now := start.Add(30 * time.Second)
	lease := domain.NewKnowledgeDocumentScheduleLockLease("schedule-1", "node-1:token-1")
	manager := NewScheduleLockManager(stubScheduleRepository{
		renewLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
			return false, errors.New("temporary")
		},
	}, ScheduleLockOptions{
		LockSeconds: 60,
		Now: func() time.Time {
			return now
		},
	})
	heartbeat := newScheduleLockHeartbeat(lease, start, 60*time.Second)

	manager.doHeartbeat(heartbeat)

	if heartbeat.IsLost() {
		t.Fatal("heartbeat should keep retrying renew errors before ttl is exceeded")
	}
}

func TestScheduleLockManagerHeartbeatMarksLostWhenRenewErrorExceedsTTL(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	now := start.Add(60 * time.Second)
	lease := domain.NewKnowledgeDocumentScheduleLockLease("schedule-1", "node-1:token-1")
	manager := NewScheduleLockManager(stubScheduleRepository{
		renewLockFn: func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
			return false, errors.New("network")
		},
	}, ScheduleLockOptions{
		LockSeconds: 60,
		Now: func() time.Time {
			return now
		},
	})
	heartbeat := newScheduleLockHeartbeat(lease, start, 60*time.Second)

	manager.doHeartbeat(heartbeat)

	if !heartbeat.IsLost() {
		t.Fatal("heartbeat should be marked lost when renew errors exceed ttl")
	}
}
