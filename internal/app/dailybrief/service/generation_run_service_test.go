package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func TestGenerationRunServiceMarkFailedTransitionsRun(t *testing.T) {
	t.Parallel()

	run, err := domain.NewGenerationRun("run-1", "user-1", "2026-06-29", domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("NewGenerationRun returned error: %v", err)
	}
	repo := &stubGenerationRunRepo{runByID: run}
	service := NewGenerationRunService(repo)
	finishedAt := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)

	updated, err := service.MarkFailed(context.Background(), "run-1", finishedAt, "timeout")
	if err != nil {
		t.Fatalf("MarkFailed returned error: %v", err)
	}
	if updated.Status != domain.GenerationRunStatusFailed {
		t.Fatalf("expected run status failed, got %q", updated.Status)
	}
	if updated.ErrorMessage != "timeout" {
		t.Fatalf("expected error message to persist, got %q", updated.ErrorMessage)
	}
	if updated.FinishedAt == nil || !updated.FinishedAt.Equal(finishedAt) {
		t.Fatalf("expected finished_at to be set, got %+v", updated.FinishedAt)
	}
}

func TestGenerationRunServiceCreateValidatesTriggerType(t *testing.T) {
	t.Parallel()

	service := NewGenerationRunService(&stubGenerationRunRepo{})
	_, err := service.Create(context.Background(), domain.GenerationRun{
		ID:          "run-2",
		UserID:      "user-1",
		BriefDate:   "2026-06-29",
		TriggerType: "manual",
		Status:      domain.GenerationRunStatusRunning,
	})
	if err == nil || !strings.Contains(err.Error(), "trigger type") {
		t.Fatalf("expected trigger validation error, got %v", err)
	}
}

type stubGenerationRunRepo struct {
	runByID domain.GenerationRun
}

func (s *stubGenerationRunRepo) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	return run, nil
}

func (s *stubGenerationRunRepo) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	s.runByID = run
	return run, nil
}

func (s *stubGenerationRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	if s.runByID.ID == id {
		return s.runByID, nil
	}
	return domain.GenerationRun{}, nil
}

func (s *stubGenerationRunRepo) GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error) {
	if s.runByID.UserID == userID && s.runByID.BriefDate == briefDate && s.runByID.Status == domain.GenerationRunStatusFailed {
		return s.runByID, nil
	}
	return domain.GenerationRun{}, nil
}

func (s *stubGenerationRunRepo) ListRetryEligible(ctx context.Context, filter port.GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error) {
	return nil, nil
}

func (s *stubGenerationRunRepo) CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error) {
	return 0, nil
}
