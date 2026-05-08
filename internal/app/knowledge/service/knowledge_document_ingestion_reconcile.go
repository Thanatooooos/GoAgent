package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

// ReconcileIngestionTaskCompletion 以 ingestion task 的终态为准，对 document/chunk_log 做补偿修复。
// 当前仅对已完成的 success/failed task 生效；对 running/pending 只跳过，不主动改写状态。
func (s *KnowledgeDocumentService) ReconcileIngestionTaskCompletion(ctx context.Context, input KnowledgeDocumentIngestionTaskCompletedInput) error {
	if s == nil || s.documentRepo == nil || s.ingestionTaskReader == nil {
		return nil
	}

	taskID := strings.TrimSpace(input.TaskID)
	documentID := strings.TrimSpace(input.DocumentID)
	if taskID == "" || documentID == "" {
		return nil
	}

	task, err := s.ingestionTaskReader.GetKnowledgePipelineTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load ingestion task for reconcile: %w", err)
	}
	if strings.TrimSpace(task.ID) == "" {
		return nil
	}

	taskDocumentID := strings.TrimSpace(readIngestionTaskMetadataString(task.Metadata, "documentId"))
	if taskDocumentID != "" && taskDocumentID != documentID {
		return fmt.Errorf("ingestion task %q belongs to document %q, not %q", taskID, taskDocumentID, documentID)
	}

	expected, ok := buildIngestionReconcileExpectation(task, input)
	if !ok {
		return nil
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("load knowledge document for reconcile: %w", err)
	}
	if strings.TrimSpace(document.ID) == "" {
		return nil
	}

	if err := s.reconcileDocumentWithTask(ctx, document, expected); err != nil {
		return err
	}
	if err := s.reconcileChunkLogWithTask(ctx, document, expected); err != nil {
		return err
	}
	return nil
}

// ScanAndReconcileIngestionTasks 定时扫描 pipeline 模式文档，并基于最新 chunk log 对齐 task/document/chunk_log 状态。
// 当前用于兜底修复“完成回调部分失败”或“回写状态漂移”的场景。
func (s *KnowledgeDocumentService) ScanAndReconcileIngestionTasks(ctx context.Context, batchSize int) error {
	if s == nil || s.documentRepo == nil || s.chunkLogRepo == nil || s.ingestionTaskReader == nil {
		return nil
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	statuses := []string{
		domain.KnowledgeDocumentStatusRunning,
		domain.KnowledgeDocumentStatusSuccess,
		domain.KnowledgeDocumentStatusFailed,
	}

	for _, status := range statuses {
		page := 0
		for {
			items, err := s.documentRepo.List(ctx, port.KnowledgeDocumentListFilter{
				Status: status,
				ListOptions: port.ListOptions{
					Offset: page * batchSize,
					Limit:  batchSize,
				},
			})
			if err != nil {
				return fmt.Errorf("list knowledge documents for reconcile scan: %w", err)
			}
			if len(items) == 0 {
				break
			}
			for _, item := range items {
				if err := s.reconcileDocumentByLatestChunkLog(ctx, item); err != nil {
					return err
				}
			}
			if len(items) < batchSize {
				break
			}
			page++
		}
	}

	return nil
}

func (s *KnowledgeDocumentService) reconcileDocumentByLatestChunkLog(ctx context.Context, document domain.KnowledgeDocument) error {
	if strings.TrimSpace(document.ID) == "" || strings.TrimSpace(document.ProcessMode) != domain.KnowledgeDocumentProcessModePipeline {
		return nil
	}

	logs, err := s.chunkLogRepo.ListByDocumentID(ctx, document.ID, port.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("list latest knowledge document chunk log for reconcile: %w", err)
	}
	if len(logs) == 0 || strings.TrimSpace(logs[0].ID) == "" {
		return nil
	}

	return s.ReconcileIngestionTaskCompletion(ctx, KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:      logs[0].ID,
		DocumentID:  document.ID,
		ChunkCount:  logs[0].ChunkCount,
		StartedAt:   logs[0].StartTime,
		CompletedAt: logs[0].EndTime,
	})
}

func readIngestionTaskMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

type ingestionReconcileExpectation struct {
	taskID       string
	documentID   string
	status       string
	chunkCount   int
	errorMessage string
	startedAt    *time.Time
	completedAt  *time.Time
	operatorID   string
	pipelineID   string
}

func buildIngestionReconcileExpectation(
	task ingestiondomain.Task,
	input KnowledgeDocumentIngestionTaskCompletedInput,
) (ingestionReconcileExpectation, bool) {
	status := strings.TrimSpace(task.Status)
	switch status {
	case ingestiondomain.TaskStatusSuccess, ingestiondomain.TaskStatusFailed:
	default:
		return ingestionReconcileExpectation{}, false
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		operatorID = strings.TrimSpace(task.UpdatedBy)
	}
	if operatorID == "" {
		operatorID = strings.TrimSpace(task.CreatedBy)
	}
	if operatorID == "" {
		operatorID = "system"
	}

	chunkCount := task.ChunkCount
	if chunkCount == 0 && input.ChunkCount > 0 {
		chunkCount = input.ChunkCount
	}

	errorMessage := strings.TrimSpace(task.ErrorMessage)
	if errorMessage == "" {
		errorMessage = strings.TrimSpace(input.ErrorMessage)
	}

	startedAt := task.StartedAt
	if startedAt == nil {
		startedAt = input.StartedAt
	}
	completedAt := task.CompletedAt
	if completedAt == nil {
		completedAt = input.CompletedAt
	}

	return ingestionReconcileExpectation{
		taskID:       strings.TrimSpace(task.ID),
		documentID:   strings.TrimSpace(input.DocumentID),
		status:       status,
		chunkCount:   chunkCount,
		errorMessage: errorMessage,
		startedAt:    startedAt,
		completedAt:  completedAt,
		operatorID:   operatorID,
		pipelineID:   strings.TrimSpace(task.PipelineID),
	}, true
}

func (s *KnowledgeDocumentService) reconcileDocumentWithTask(
	ctx context.Context,
	document domain.KnowledgeDocument,
	expected ingestionReconcileExpectation,
) error {
	desiredStatus := domain.KnowledgeDocumentStatusSuccess
	if expected.status == ingestiondomain.TaskStatusFailed {
		desiredStatus = domain.KnowledgeDocumentStatusFailed
	}

	needsUpdate := strings.TrimSpace(document.Status) != desiredStatus
	if desiredStatus == domain.KnowledgeDocumentStatusSuccess && document.ChunkCount != expected.chunkCount {
		needsUpdate = true
	}
	if !needsUpdate {
		return nil
	}

	document.Status = desiredStatus
	if desiredStatus == domain.KnowledgeDocumentStatusSuccess {
		document.ChunkCount = expected.chunkCount
	}
	document.UpdatedBy = expected.operatorID
	document.UpdatedAt = time.Now()

	_, err := s.documentRepo.Update(ctx, document)
	if err != nil {
		return fmt.Errorf("reconcile knowledge document %q: %w", document.ID, err)
	}
	return nil
}

func (s *KnowledgeDocumentService) reconcileChunkLogWithTask(
	ctx context.Context,
	document domain.KnowledgeDocument,
	expected ingestionReconcileExpectation,
) error {
	if s.chunkLogRepo == nil {
		return nil
	}

	record, err := s.chunkLogRepo.GetByTaskID(ctx, expected.taskID)
	if err != nil {
		return fmt.Errorf("load knowledge document chunk log for reconcile: %w", err)
	}
	if strings.TrimSpace(record.ID) == "" {
		record = domain.NewKnowledgeDocumentChunkLog(expected.taskID, expected.documentID)
		record.ProcessMode = document.ProcessMode
		record.ChunkStrategy = document.ChunkStrategy
		record.PipelineID = document.PipelineID
		if record.PipelineID == "" {
			record.PipelineID = expected.pipelineID
		}
		record.ChunkCount = expected.chunkCount
		record.ErrorMessage = ""
		record.Status = domain.KnowledgeDocumentChunkLogStatusSuccess
		record.StartTime = expected.startedAt
		record.EndTime = expected.completedAt
		if expected.status == ingestiondomain.TaskStatusFailed {
			record.Status = domain.KnowledgeDocumentChunkLogStatusFailed
			record.ErrorMessage = expected.errorMessage
		}
		_, err := s.chunkLogRepo.Create(ctx, record)
		if err != nil {
			return fmt.Errorf("create reconciled knowledge document chunk log %q: %w", expected.taskID, err)
		}
		return nil
	}

	if strings.TrimSpace(record.DocumentID) != "" && strings.TrimSpace(record.DocumentID) != expected.documentID {
		return fmt.Errorf("knowledge document chunk log task %q belongs to document %q, not %q", expected.taskID, record.DocumentID, expected.documentID)
	}

	desiredStatus := domain.KnowledgeDocumentChunkLogStatusSuccess
	desiredError := ""
	if expected.status == ingestiondomain.TaskStatusFailed {
		desiredStatus = domain.KnowledgeDocumentChunkLogStatusFailed
		desiredError = expected.errorMessage
	}

	needsUpdate := strings.TrimSpace(record.Status) != desiredStatus ||
		record.ChunkCount != expected.chunkCount ||
		strings.TrimSpace(record.ErrorMessage) != desiredError

	if expected.startedAt != nil && (record.StartTime == nil || !record.StartTime.Equal(*expected.startedAt)) {
		record.StartTime = expected.startedAt
		needsUpdate = true
	}
	if expected.completedAt != nil && (record.EndTime == nil || !record.EndTime.Equal(*expected.completedAt)) {
		record.EndTime = expected.completedAt
		needsUpdate = true
	}

	if !needsUpdate {
		return nil
	}

	record.Status = desiredStatus
	record.ChunkCount = expected.chunkCount
	record.ErrorMessage = desiredError
	record.UpdatedAt = time.Now()
	if record.PipelineID == "" {
		record.PipelineID = document.PipelineID
	}
	if record.ProcessMode == "" {
		record.ProcessMode = document.ProcessMode
	}
	if record.ChunkStrategy == "" {
		record.ChunkStrategy = document.ChunkStrategy
	}

	_, err = s.chunkLogRepo.Update(ctx, record)
	if err != nil {
		return fmt.Errorf("reconcile knowledge document chunk log %q: %w", record.ID, err)
	}
	return nil
}
