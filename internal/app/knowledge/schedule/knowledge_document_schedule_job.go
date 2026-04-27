package schedule

import (
	"context"
	"time"

	"local/rag-project/internal/app/knowledge/port"
)

// KnowledgeDocumentScheduleJob polls due schedules and hands execution off to a worker.
type KnowledgeDocumentScheduleJob struct {
	scheduleRepo port.KnowledgeDocumentScheduleRepository
}

func NewKnowledgeDocumentScheduleJob(scheduleRepo port.KnowledgeDocumentScheduleRepository) *KnowledgeDocumentScheduleJob {
	return &KnowledgeDocumentScheduleJob{scheduleRepo: scheduleRepo}
}

func (j *KnowledgeDocumentScheduleJob) ListDue(ctx context.Context, now time.Time, limit int) error {
	if j == nil || j.scheduleRepo == nil {
		return nil
	}
	_, err := j.scheduleRepo.ListDue(ctx, now, limit)
	return err
}
