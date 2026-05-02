package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service"
)

type scheduleRepositoryStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s scheduleRepositoryStub) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s scheduleRepositoryStub) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s scheduleRepositoryStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	return 0, nil
}

func (s scheduleRepositoryStub) Delete(ctx context.Context, id string) error { return nil }

func (s scheduleRepositoryStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

func (s scheduleRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s scheduleRepositoryStub) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s scheduleRepositoryStub) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	return nil, nil
}

func (s scheduleRepositoryStub) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return false, nil
}

func (s scheduleRepositoryStub) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	return false, nil
}

func (s scheduleRepositoryStub) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	return false, nil
}

type scheduleExecRepositoryStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s scheduleExecRepositoryStub) Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s scheduleExecRepositoryStub) Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s scheduleExecRepositoryStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
	return 0, nil
}

func (s scheduleExecRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s scheduleExecRepositoryStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

func (s scheduleExecRepositoryStub) List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error) {
	return nil, nil
}

func TestKnowledgeDocumentScheduleServiceDeleteByDocIDUsesTransaction(t *testing.T) {
	t.Parallel()

	calls := make([]string, 0, 2)
	txCalled := false
	svc := service.NewKnowledgeDocumentScheduleService(nil, nil, 0, func(
		ctx context.Context,
		fn func(ctx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error,
	) error {
		txCalled = true
		return fn(ctx, scheduleRepositoryStub{
			deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
				calls = append(calls, "schedule:"+documentID)
				return nil
			},
		}, scheduleExecRepositoryStub{
			deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
				calls = append(calls, "exec:"+documentID)
				return nil
			},
		})
	})

	if err := svc.DeleteByDocID(context.Background(), " doc-1 "); err != nil {
		t.Fatalf("DeleteByDocID() error = %v", err)
	}
	if !txCalled {
		t.Fatal("DeleteByDocID() should run inside transaction")
	}
	want := []string{"exec:doc-1", "schedule:doc-1"}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Fatalf("unexpected delete order: got %v want %v", calls, want)
	}
}

func TestKnowledgeDocumentScheduleServiceDeleteByDocIDPropagatesErrorForRollback(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("boom")
	svc := service.NewKnowledgeDocumentScheduleService(nil, nil, 0, func(
		ctx context.Context,
		fn func(ctx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error,
	) error {
		return fn(ctx, scheduleRepositoryStub{
			deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
				return repoErr
			},
		}, scheduleExecRepositoryStub{})
	})

	err := svc.DeleteByDocID(context.Background(), "doc-1")
	if err == nil {
		t.Fatal("DeleteByDocID() should return error")
	}
	if !errors.Is(err, repoErr) {
		t.Fatalf("DeleteByDocID() should wrap repository error, got %v", err)
	}
}

func TestKnowledgeDocumentScheduleServiceDeleteByDocIDSkipsBlankDocumentID(t *testing.T) {
	t.Parallel()

	txCalled := false
	svc := service.NewKnowledgeDocumentScheduleService(nil, nil, 0, func(
		ctx context.Context,
		fn func(ctx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error,
	) error {
		txCalled = true
		return nil
	})

	if err := svc.DeleteByDocID(context.Background(), " "); err != nil {
		t.Fatalf("DeleteByDocID(blank) error = %v", err)
	}
	if txCalled {
		t.Fatal("DeleteByDocID(blank) should not open transaction")
	}
}
