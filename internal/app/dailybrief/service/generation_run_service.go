package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type GenerationRunService struct {
	repo port.GenerationRunRepository
}

func NewGenerationRunService(repo port.GenerationRunRepository) *GenerationRunService {
	return &GenerationRunService{repo: repo}
}

func (s *GenerationRunService) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	if strings.TrimSpace(run.ID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run id is required")
	}
	if strings.TrimSpace(run.UserID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run user id is required")
	}
	if strings.TrimSpace(run.BriefDate) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run brief date is required")
	}
	if !domain.IsValidGenerationRunTriggerType(run.TriggerType) {
		return domain.GenerationRun{}, fmt.Errorf("generation run trigger type %q is invalid", run.TriggerType)
	}
	if run.Status == "" {
		run.Status = domain.GenerationRunStatusRunning
	}
	if !domain.IsValidGenerationRunStatus(run.Status) {
		return domain.GenerationRun{}, fmt.Errorf("generation run status %q is invalid", run.Status)
	}
	return s.repo.Create(ctx, run)
}

func (s *GenerationRunService) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	if strings.TrimSpace(run.ID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run id is required")
	}
	if strings.TrimSpace(run.UserID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run user id is required")
	}
	if strings.TrimSpace(run.BriefDate) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run brief date is required")
	}
	if !domain.IsValidGenerationRunTriggerType(run.TriggerType) {
		return domain.GenerationRun{}, fmt.Errorf("generation run trigger type %q is invalid", run.TriggerType)
	}
	if !domain.IsValidGenerationRunStatus(run.Status) {
		return domain.GenerationRun{}, fmt.Errorf("generation run status %q is invalid", run.Status)
	}
	return s.repo.Update(ctx, run)
}

func (s *GenerationRunService) MarkSucceeded(ctx context.Context, id string, finishedAt time.Time) (domain.GenerationRun, error) {
	run, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.GenerationRun{}, err
	}
	if strings.TrimSpace(run.ID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run %q not found", id)
	}
	if err := run.MarkSucceeded(finishedAt); err != nil {
		return domain.GenerationRun{}, err
	}
	return s.repo.Update(ctx, run)
}

func (s *GenerationRunService) MarkDegraded(ctx context.Context, id string, finishedAt time.Time) (domain.GenerationRun, error) {
	run, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.GenerationRun{}, err
	}
	if strings.TrimSpace(run.ID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run %q not found", id)
	}
	if err := run.MarkDegraded(finishedAt); err != nil {
		return domain.GenerationRun{}, err
	}
	return s.repo.Update(ctx, run)
}

func (s *GenerationRunService) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) (domain.GenerationRun, error) {
	run, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.GenerationRun{}, err
	}
	if strings.TrimSpace(run.ID) == "" {
		return domain.GenerationRun{}, fmt.Errorf("generation run %q not found", id)
	}
	if err := run.MarkFailed(finishedAt, message); err != nil {
		return domain.GenerationRun{}, err
	}
	return s.repo.Update(ctx, run)
}
