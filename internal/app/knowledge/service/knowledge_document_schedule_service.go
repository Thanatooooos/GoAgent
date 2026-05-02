package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

type Sourcetype string

const (
	FILE Sourcetype = "file"
	URL  Sourcetype = "url"
)

func (Sourcetype) FromValue(value string) Sourcetype {
	if value == "" {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "file" || normalized == "localfile" || normalized == "local_file" {
		return FILE
	}
	if normalized == "url" {
		return URL
	}
	return ""
}

func (Sourcetype) Normalized(value string) (Sourcetype, error) {
	if value == "" {
		return "", fmt.Errorf("sourceType can not be empty")
	}
	normalized := FILE.FromValue(value)
	if normalized == "" {
		return "", fmt.Errorf("invalid sourceType")
	}
	return normalized, nil
}

type KnowledgeDocumentScheduleService struct {
	scheduleRepo     port.KnowledgeDocumentScheduleRepository
	scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository
	scheduleSeconds  int64
	transaction      KnowledgeDocumentScheduleTransaction
}

type KnowledgeDocumentScheduleTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error,
) error

func NewKnowledgeDocumentScheduleService(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
	scheduleSeconds int64,
	transaction KnowledgeDocumentScheduleTransaction,
) *KnowledgeDocumentScheduleService {
	return &KnowledgeDocumentScheduleService{
		scheduleRepo:     scheduleRepo,
		scheduleExecRepo: scheduleExecRepo,
		scheduleSeconds:  scheduleSeconds,
		transaction:      transaction,
	}
}

func (s *KnowledgeDocumentScheduleService) SyncSchedule(ctx context.Context, document *domain.KnowledgeDocument, allowCreate bool) error {
	if document == nil {
		return fmt.Errorf("invalid document")
	}
	if document.SourceType != string(URL) {
		return fmt.Errorf("invalid source type")
	}
	docEnabled := document.Enabled
	cron := document.ScheduleCron
	enabled := document.ScheduleEnabled

	if cron == "" || docEnabled == false {
		enabled = false
	}
	var nextRunTime *time.Time
	if enabled {
		ok, err := schedule.IsIntervalLessThan(cron, time.Now(), s.scheduleSeconds)
		if err != nil {
			return err
		}
		if ok {
			return fmt.Errorf("invalid cron")
		}
		nextRunTime, _ = schedule.NextRunTime(cron, time.Now())
	}
	existing, err := s.scheduleRepo.GetByDocumentID(ctx, document.ID)
	if err != nil {
		return err
	}
	if existing.ID == "" {
		if allowCreate == false {
			return fmt.Errorf("cannot create exec schedule")
		}
		id, err := distributedid.NextID()
		if err != nil {
			return exception.NewServiceException("failed to generate knowledge document schedule id", err)
		}
		schedule := domain.NewKnowledgeDocumentSchedule(fmt.Sprintf("%d", id), document.ID, document.KnowledgeBaseID, cron)
		schedule.Enabled = enabled
		schedule.NextRunTime = nextRunTime
		_, err = s.scheduleRepo.Create(ctx, schedule)
		if err != nil {
			return err
		}
	} else {
		_, err := s.scheduleRepo.UpdateWhere(ctx, port.KnowledgeDocumentScheduleConditions{
			DocumentID: existing.DocumentID,
		}, port.KnowledgeDocumentSchedulePatch{
			CronExpr:    port.ValueOf(cron),
			Enabled:     port.ValueOf(enabled),
			NextRunTime: port.ValueOf(nextRunTime),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *KnowledgeDocumentScheduleService) DeleteByDocID(ctx context.Context, docID string) error {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return nil
	}
	if s == nil {
		return exception.NewServiceException("knowledge document schedule service is required", nil)
	}
	if s.transaction == nil {
		return exception.NewServiceException("knowledge document schedule transaction is required", nil)
	}

	return s.transaction(ctx, func(txCtx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error {
		if scheduleRepo == nil {
			return exception.NewServiceException("knowledge document schedule repository is required", nil)
		}
		if scheduleExecRepo == nil {
			return exception.NewServiceException("knowledge document schedule exec repository is required", nil)
		}

		if err := scheduleExecRepo.DeleteByDocumentID(txCtx, docID); err != nil {
			return exception.NewServiceException("failed to delete knowledge document schedule execs", err)
		}
		if err := scheduleRepo.DeleteByDocumentID(txCtx, docID); err != nil {
			return exception.NewServiceException("failed to delete knowledge document schedule", err)
		}
		return nil
	})
}
