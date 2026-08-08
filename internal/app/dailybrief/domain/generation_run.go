package domain

import (
	"fmt"
	"time"
)

type GenerationRun struct {
	ID              string
	UserID          string
	BriefDate       string
	TriggerType     string
	Status          string
	StartedAt       time.Time
	FinishedAt      *time.Time
	ErrorMessage    string
	SourceStatsJSON string
	Model           string
	PromptVersion   string
	TokenUsageJSON  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewGenerationRun(id, userID, briefDate, triggerType string) (GenerationRun, error) {
	if !IsValidGenerationRunTriggerType(triggerType) {
		return GenerationRun{}, fmt.Errorf("invalid generation run trigger type %q", triggerType)
	}

	now := time.Now()
	return GenerationRun{
		ID:          id,
		UserID:      userID,
		BriefDate:   briefDate,
		TriggerType: triggerType,
		Status:      GenerationRunStatusRunning,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *GenerationRun) MarkSucceeded(finishedAt time.Time) error {
	return r.markFinished(GenerationRunStatusSucceeded, finishedAt, "")
}

func (r *GenerationRun) MarkDegraded(finishedAt time.Time) error {
	return r.markFinished(GenerationRunStatusDegraded, finishedAt, "")
}

func (r *GenerationRun) MarkFailed(finishedAt time.Time, errorMessage string) error {
	return r.markFinished(GenerationRunStatusFailed, finishedAt, errorMessage)
}

func (r *GenerationRun) markFinished(status string, finishedAt time.Time, errorMessage string) error {
	if r.Status != GenerationRunStatusRunning {
		return fmt.Errorf("cannot transition generation run from %q to %q", r.Status, status)
	}

	r.Status = status
	r.FinishedAt = &finishedAt
	r.ErrorMessage = errorMessage
	r.UpdatedAt = finishedAt
	return nil
}
