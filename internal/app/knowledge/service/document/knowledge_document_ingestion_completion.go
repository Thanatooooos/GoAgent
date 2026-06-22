package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

func (s *KnowledgeDocumentService) OnIngestionTaskCompleted(ctx context.Context, input KnowledgeDocumentIngestionTaskCompletedInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}

	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return nil
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("get knowledge document: %w", err)
	}
	if document.ID == "" {
		return nil
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		operatorID = "system"
	}

	chunkCount := input.ChunkCount
	if input.ErrorMessage != "" {
		return errors.Join(
			s.markDocumentFailed(ctx, documentID, operatorID),
			s.finishPipelineChunkLogWithRecord(ctx, documentID, input.TaskID, chunkCount, input.ErrorMessage, input.StartedAt, input.CompletedAt),
			s.ReconcileIngestionTaskCompletion(ctx, input),
		)
	}

	return errors.Join(
		s.markDocumentSuccess(ctx, documentID, chunkCount, operatorID),
		s.finishPipelineChunkLogWithRecord(ctx, documentID, input.TaskID, chunkCount, "", input.StartedAt, input.CompletedAt),
		s.ReconcileIngestionTaskCompletion(ctx, input),
	)
}

func (s *KnowledgeDocumentService) markDocumentFailed(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(strings.TrimSpace(documentID)),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedBy.To(strings.TrimSpace(operatorID)),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (s *KnowledgeDocumentService) markDocumentSuccess(ctx context.Context, documentID string, chunkCount int, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(strings.TrimSpace(documentID)),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusSuccess),
		port.KnowledgeDocument.ChunkCount.To(chunkCount),
		port.KnowledgeDocument.UpdatedBy.To(strings.TrimSpace(operatorID)),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (s *KnowledgeDocumentService) finishPipelineChunkLogWithRecord(ctx context.Context, documentID, taskID string, chunkCount int, errorMessage string, startedAt, completedAt *time.Time) error {
	if s.chunkLogRepo == nil {
		return nil
	}
	record, err := s.loadCompletablePipelineChunkLog(ctx, documentID, taskID)
	if err != nil || record.ID == "" {
		return err
	}
	record.Status = domain.KnowledgeDocumentChunkLogStatusSuccess
	record.ErrorMessage = ""
	if errorMessage != "" {
		record.Status = domain.KnowledgeDocumentChunkLogStatusFailed
		record.ErrorMessage = errorMessage
	}
	record.ChunkCount = chunkCount
	now := time.Now()
	if startedAt != nil {
		record.StartTime = startedAt
	}
	if completedAt != nil {
		record.EndTime = completedAt
	}
	record.UpdatedAt = now

	_, err = s.chunkLogRepo.Update(ctx, record)
	return err
}

func (s *KnowledgeDocumentService) loadCompletablePipelineChunkLog(ctx context.Context, documentID string, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
	if s.chunkLogRepo == nil {
		return domain.KnowledgeDocumentChunkLog{}, nil
	}
	if strings.TrimSpace(taskID) != "" {
		record, err := s.chunkLogRepo.GetByTaskID(ctx, strings.TrimSpace(taskID))
		if err != nil {
			return domain.KnowledgeDocumentChunkLog{}, err
		}
		if record.ID != "" {
			if strings.TrimSpace(record.DocumentID) != "" && strings.TrimSpace(record.DocumentID) != strings.TrimSpace(documentID) {
				return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("knowledge document chunk log task %q belongs to document %q, not %q", taskID, record.DocumentID, documentID)
			}
			return record, nil
		}
	}
	items, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{Limit: 1})
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, err
	}
	if len(items) == 0 {
		return domain.KnowledgeDocumentChunkLog{}, nil
	}
	return items[0], nil
}
