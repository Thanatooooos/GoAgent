package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

func (s *KnowledgeDocumentService) Upload(ctx context.Context, input UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document service is required", nil)
	}
	if s.baseRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge base repository is required", nil)
	}
	if s.documentRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.storage == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("file storage is required", nil)
	}

	knowledgeBaseID := strings.TrimSpace(input.KnowledgeBaseID)
	if knowledgeBaseID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge base id is required", nil)
	}

	fileName := sanitizeDocumentFileName(input.FileName)
	if fileName == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("file name is required", nil)
	}
	if input.Body == nil {
		return domain.KnowledgeDocument{}, exception.NewClientException("file body is required", nil)
	}
	if input.Size < 0 {
		return domain.KnowledgeDocument{}, exception.NewClientException("file size is invalid", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("operator id is required", nil)
	}

	sourceType := normalizeKnowledgeDocumentSourceType(input.SourceType)
	if sourceType == "" {
		sourceType = domain.KnowledgeDocumentSourceFile
	}

	processMode, err := normalizeKnowledgeDocumentProcessMode(input.ProcessMode)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	chunkStrategy, err := normalizeKnowledgeDocumentChunkStrategy(input.ChunkStrategy)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	pipelineID := strings.TrimSpace(input.PipelineID)
	if err := validateKnowledgeDocumentProcessingConfig(processMode, chunkStrategy, pipelineID, true); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	input.ProcessMode = processMode
	input.ChunkStrategy = chunkStrategy
	if processMode == domain.KnowledgeDocumentProcessModeChunk {
		input.PipelineID = ""
	} else {
		input.PipelineID = pipelineID
	}

	knowledgeBase, err := s.baseRepo.GetByID(ctx, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to get knowledge base", err)
	}
	if knowledgeBase.ID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge base not found", nil)
	}

	id, err := distributedid.NextID()
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to generate knowledge document id", err)
	}
	documentID := fmt.Sprintf("%d", id)
	document, cleanup, err := s.buildKnowledgeDocumentForUpload(ctx, knowledgeBase, documentID, sourceType, input, operatorID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}

	created, err := s.documentRepo.Create(ctx, document)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to create knowledge document", err)
	}

	if created.IsRemote() && s.scheduleService != nil {
		if err := s.scheduleService.SyncSchedule(ctx, &created, true); err != nil {
			_ = s.documentRepo.Delete(ctx, created.ID)
			if cleanup != nil {
				cleanup()
			}
			log.Warnf("failed to create knowledge document schedule", err)
		}
	}

	return created, nil
}

func (s *KnowledgeDocumentService) StartChunk(ctx context.Context, input StartChunkKnowledgeDocumentInput) error {
	if s == nil {
		return exception.NewServiceException("knowledge document service is required", nil)
	}
	if s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.taskQueue == nil {
		return exception.NewServiceException("task queue is required", nil)
	}

	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return exception.NewClientException("knowledge document id is required", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return exception.NewClientException("knowledge document not found", nil)
	}
	if !document.Enabled {
		return exception.NewClientException("knowledge document is disabled", nil)
	}
	if !document.CanStartProcessing() {
		return exception.NewClientException("knowledge document cannot start processing", nil)
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
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document running", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document processing is already running", nil)
	}

	document.Status = domain.KnowledgeDocumentStatusRunning
	document.UpdatedBy = operatorID
	document.UpdatedAt = time.Now()
	if document.IsRemote() {
		if s.scheduleService == nil {
			_ = s.markDocumentFailed(ctx, document.ID, operatorID)
			return exception.NewServiceException("knowledge document schedule service is required", nil)
		}
		if err := s.scheduleService.SyncSchedule(ctx, &document, true); err != nil {
			_ = s.markDocumentFailed(ctx, document.ID, operatorID)
			return exception.NewServiceException("failed to sync knowledge document schedule", err)
		}
	}

	taskID, err := distributedid.NextID()
	if err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return exception.NewServiceException("failed to generate chunk task id", err)
	}
	if err := s.taskQueue.SubmitChunkDocument(ctx, port.ChunkDocumentTask{
		TaskID:      fmt.Sprintf("%d", taskID),
		DocumentID:  document.ID,
		TriggeredBy: operatorID,
	}); err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return exception.NewServiceException("failed to submit chunk document task", err)
	}

	return nil
}

func (s *KnowledgeDocumentService) Update(ctx context.Context, input UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("operator id is required", nil)
	}

	nextProcessMode, err := effectiveKnowledgeDocumentProcessMode(document.ProcessMode, input.ProcessMode)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	nextChunkStrategy, err := effectiveKnowledgeDocumentChunkStrategy(document.ChunkStrategy, input.ChunkStrategy)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	nextPipelineID := strings.TrimSpace(document.PipelineID)
	if strings.TrimSpace(input.PipelineID) != "" {
		nextPipelineID = strings.TrimSpace(input.PipelineID)
	}
	if err := validateKnowledgeDocumentProcessingConfig(
		nextProcessMode,
		nextChunkStrategy,
		nextPipelineID,
		strings.TrimSpace(input.ProcessMode) != "" || strings.TrimSpace(input.ChunkStrategy) != "" || strings.TrimSpace(input.PipelineID) != "",
	); err != nil {
		return domain.KnowledgeDocument{}, err
	}

	if name := strings.TrimSpace(input.Name); name != "" {
		document.Name = name
	}
	document.ProcessMode = nextProcessMode
	if document.ProcessMode == domain.KnowledgeDocumentProcessModeChunk {
		document.ChunkStrategy = nextChunkStrategy
		document.PipelineID = ""
	} else {
		document.ChunkStrategy = ""
		document.PipelineID = nextPipelineID
	}
	if input.ChunkConfig != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			return domain.KnowledgeDocument{}, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	if location := strings.TrimSpace(input.SourceLocation); location != "" {
		document.SourceLocation = location
	}
	if input.ScheduleEnabled != nil {
		document.ScheduleEnabled = *input.ScheduleEnabled
	}
	if cron := strings.TrimSpace(input.ScheduleCron); cron != "" || (input.ScheduleEnabled != nil && !*input.ScheduleEnabled) {
		document.ScheduleCron = cron
	}
	document.UpdatedBy = operatorID
	document.UpdatedAt = time.Now()

	updated, err := s.documentRepo.Update(ctx, document)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to update knowledge document", err)
	}
	if updated.IsRemote() && s.scheduleService != nil {
		if err := s.scheduleService.SyncSchedule(ctx, &updated, true); err != nil {
			return domain.KnowledgeDocument{}, exception.NewServiceException("failed to update knowledge document schedule", err)
		}
	}
	return updated, nil
}

func (s *KnowledgeDocumentService) Enable(ctx context.Context, input EnableKnowledgeDocumentInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return err
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	rows, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(document.ID),
		port.KnowledgeDocument.Deleted.Eq(false),
	), port.Set(
		port.KnowledgeDocument.Enabled.To(input.Enabled),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document enabled status", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document not found", nil)
	}

	if document.IsRemote() && s.scheduleService != nil {
		document.Enabled = input.Enabled
		document.UpdatedBy = operatorID
		document.UpdatedAt = time.Now()
		if err := s.scheduleService.SyncSchedule(ctx, &document, true); err != nil {
			return exception.NewServiceException("failed to sync knowledge document schedule", err)
		}
	}
	return nil
}

func (s *KnowledgeDocumentService) Delete(ctx context.Context, input DeleteKnowledgeDocumentInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.deleteTx == nil {
		return exception.NewServiceException("knowledge document delete transaction is required", nil)
	}
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return err
	}
	if !document.CanDelete() {
		return exception.NewClientException("knowledge document cannot be deleted in current status", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	if err := s.deleteTx(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
		scheduleRepo port.KnowledgeDocumentScheduleRepository,
		scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
	) error {
		rows, err := documentRepo.UpdateFields(txCtx, port.Where(
			port.KnowledgeDocument.ID.Eq(document.ID),
			port.KnowledgeDocument.Deleted.Eq(false),
			port.KnowledgeDocument.Status.In(
				domain.KnowledgeDocumentStatusPending,
				domain.KnowledgeDocumentStatusFailed,
				domain.KnowledgeDocumentStatusSuccess,
			),
		), port.Set(
			port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusDeleting),
			port.KnowledgeDocument.UpdatedBy.To(operatorID),
			port.KnowledgeDocument.UpdatedAt.To(time.Now()),
		))
		if err != nil {
			return exception.NewServiceException("failed to mark knowledge document deleting", err)
		}
		if rows == 0 {
			return exception.NewClientException("knowledge document cannot be deleted in current status", nil)
		}

		// Hard delete cross-table / externalized runtime data first so we do not leave
		// executable schedule state or searchable vectors behind after the document is deleted.
		if document.IsRemote() {
			if scheduleExecRepo != nil {
				if err := scheduleExecRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
					return exception.NewServiceException("failed to delete knowledge document schedule execs", err)
				}
			}
			if scheduleRepo != nil {
				if err := scheduleRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
					return exception.NewServiceException("failed to delete knowledge document schedule", err)
				}
			}
		}
		if vectorStore != nil {
			if err := vectorStore.DeleteByDocumentID(txCtx, document.ID); err != nil {
				return exception.NewServiceException("failed to delete knowledge document vectors", err)
			}
		}

		// Soft delete chunk rows and the document row itself. Both repositories are backed by
		// GORM soft_delete models, so the business data remains recoverable/auditable in DB.
		if chunkRepo != nil {
			if err := chunkRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
				return exception.NewServiceException("failed to delete knowledge document chunks", err)
			}
		}
		if err := documentRepo.Delete(txCtx, document.ID); err != nil {
			return exception.NewServiceException("failed to delete knowledge document", err)
		}
		return nil
	}); err != nil {
		return err
	}
	if s.storage != nil && strings.TrimSpace(document.FileURL) != "" {
		if err := s.storage.Delete(ctx, document.FileURL); err != nil {
			log.Errorf("knowledge document file cleanup failed after delete commit: documentId=%s operatorId=%s fileUrl=%s err=%v",
				document.ID, operatorID, document.FileURL, err)
		}
	}
	return nil
}
