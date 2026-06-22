package process

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

func (s *DocumentProcessService) ensureDocumentRunning(ctx context.Context, document domain.KnowledgeDocument, operatorID string) error {
	if document.Status == domain.KnowledgeDocumentStatusRunning {
		return nil
	}
	rows, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(document.ID),
		port.KnowledgeDocument.Enabled.Eq(true),
		port.KnowledgeDocument.Deleted.Eq(false),
		port.KnowledgeDocument.Status.In(
			domain.KnowledgeDocumentStatusPending,
			domain.KnowledgeDocumentStatusFailed,
			domain.KnowledgeDocumentStatusSuccess,
		),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusRunning),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document running", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document cannot start processing", nil)
	}
	return nil
}

func (s *DocumentProcessService) markDocumentSuccess(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusSuccess),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document success", err)
	}
	return nil
}

func (s *DocumentProcessService) markDocumentFailed(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	return err
}

func (s *DocumentProcessService) createRunningChunkLog(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocumentChunkLog, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, exception.NewServiceException("failed to generate knowledge document chunk log id", err)
	}

	startTime := s.now()
	chunkLog := domain.NewKnowledgeDocumentChunkLog(fmt.Sprintf("%d", id), document.ID)
	chunkLog.Status = domain.KnowledgeDocumentChunkLogStatusRunning
	chunkLog.ProcessMode = document.ProcessMode
	chunkLog.ChunkStrategy = document.ChunkStrategy
	chunkLog.PipelineID = document.PipelineID
	chunkLog.StartTime = &startTime
	chunkLog.CreatedAt = startTime
	chunkLog.UpdatedAt = startTime

	created, err := s.chunkLogRepo.Create(ctx, chunkLog)
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, exception.NewServiceException("failed to create knowledge document chunk log", err)
	}
	return created, nil
}

func (s *DocumentProcessService) finishChunkLog(ctx context.Context, chunkLog domain.KnowledgeDocumentChunkLog, status string, result documentProcessResult, errorMessage string) error {
	endTime := s.now()
	chunkLog.Status = status
	chunkLog.ExtractDuration = result.ExtractDuration
	chunkLog.ChunkDuration = result.ChunkDuration
	chunkLog.EmbedDuration = result.EmbedDuration
	chunkLog.PersistDuration = result.PersistDuration
	chunkLog.TotalDuration = result.TotalDuration
	chunkLog.ChunkCount = result.ChunkCount
	chunkLog.ErrorMessage = truncateChunkLogError(errorMessage)
	chunkLog.EndTime = &endTime
	chunkLog.UpdatedAt = endTime

	if _, err := s.chunkLogRepo.Update(ctx, chunkLog); err != nil {
		return exception.NewServiceException("failed to update knowledge document chunk log", err)
	}
	return nil
}

func truncateChunkLogError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		return message[:2000]
	}
	return message
}
