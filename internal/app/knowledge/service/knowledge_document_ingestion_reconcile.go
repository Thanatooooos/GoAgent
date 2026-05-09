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

const (
	reconcileSourceTaskCompletion = "task_completion"
	reconcileSourceScan           = "scan"
)

type ingestionReconcileResult struct {
	skipped         bool
	documentUpdated bool
	chunkLogUpdated bool
	chunkLogCreated bool
}

// ReconcileIngestionTaskCompletion uses the ingestion task terminal state as the
// source of truth and repairs document/chunk_log drift when possible.
func (s *KnowledgeDocumentService) ReconcileIngestionTaskCompletion(
	ctx context.Context,
	input KnowledgeDocumentIngestionTaskCompletedInput,
) error {
	_, err := s.reconcileIngestionTaskCompletion(ctx, input, reconcileSourceTaskCompletion)
	return err
}

func (s *KnowledgeDocumentService) reconcileIngestionTaskCompletion(
	ctx context.Context,
	input KnowledgeDocumentIngestionTaskCompletedInput,
	source string,
) (result ingestionReconcileResult, err error) {
	defer func() {
		s.recordIngestionReconcileEvent(source, input, result, err)
	}()

	if s == nil || s.documentRepo == nil || s.ingestionTaskReader == nil {
		result.skipped = true
		return result, nil
	}

	taskID := strings.TrimSpace(input.TaskID)
	documentID := strings.TrimSpace(input.DocumentID)
	if taskID == "" || documentID == "" {
		result.skipped = true
		return result, nil
	}

	task, err := s.ingestionTaskReader.GetKnowledgePipelineTask(ctx, taskID)
	if err != nil {
		return result, fmt.Errorf("load ingestion task for reconcile: %w", err)
	}
	if strings.TrimSpace(task.ID) == "" {
		result.skipped = true
		return result, nil
	}

	taskDocumentID := strings.TrimSpace(readIngestionTaskMetadataString(task.Metadata, "documentId"))
	if taskDocumentID != "" && taskDocumentID != documentID {
		return result, fmt.Errorf("ingestion task %q belongs to document %q, not %q", taskID, taskDocumentID, documentID)
	}

	expected, ok := buildIngestionReconcileExpectation(task, input)
	if !ok {
		result.skipped = true
		return result, nil
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return result, fmt.Errorf("load knowledge document for reconcile: %w", err)
	}
	if strings.TrimSpace(document.ID) == "" {
		result.skipped = true
		return result, nil
	}

	result.documentUpdated, err = s.reconcileDocumentWithTask(ctx, document, expected)
	if err != nil {
		return result, err
	}
	result.chunkLogUpdated, result.chunkLogCreated, err = s.reconcileChunkLogWithTask(ctx, document, expected)
	if err != nil {
		return result, err
	}
	return result, nil
}

// ScanAndReconcileIngestionTasks scans pipeline documents and reconciles their
// latest task/chunk_log/document state.
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

	_, err = s.reconcileIngestionTaskCompletion(ctx, KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:      logs[0].ID,
		DocumentID:  document.ID,
		ChunkCount:  logs[0].ChunkCount,
		StartedAt:   logs[0].StartTime,
		CompletedAt: logs[0].EndTime,
	}, reconcileSourceScan)
	return err
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
) (bool, error) {
	desiredStatus := domain.KnowledgeDocumentStatusSuccess
	if expected.status == ingestiondomain.TaskStatusFailed {
		desiredStatus = domain.KnowledgeDocumentStatusFailed
	}

	needsUpdate := strings.TrimSpace(document.Status) != desiredStatus
	if desiredStatus == domain.KnowledgeDocumentStatusSuccess && document.ChunkCount != expected.chunkCount {
		needsUpdate = true
	}
	if !needsUpdate {
		return false, nil
	}

	document.Status = desiredStatus
	if desiredStatus == domain.KnowledgeDocumentStatusSuccess {
		document.ChunkCount = expected.chunkCount
	}
	document.UpdatedBy = expected.operatorID
	document.UpdatedAt = time.Now()

	_, err := s.documentRepo.Update(ctx, document)
	if err != nil {
		return false, fmt.Errorf("reconcile knowledge document %q: %w", document.ID, err)
	}
	return true, nil
}

func (s *KnowledgeDocumentService) reconcileChunkLogWithTask(
	ctx context.Context,
	document domain.KnowledgeDocument,
	expected ingestionReconcileExpectation,
) (bool, bool, error) {
	if s.chunkLogRepo == nil {
		return false, false, nil
	}

	record, err := s.chunkLogRepo.GetByTaskID(ctx, expected.taskID)
	if err != nil {
		return false, false, fmt.Errorf("load knowledge document chunk log for reconcile: %w", err)
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
			return false, false, fmt.Errorf("create reconciled knowledge document chunk log %q: %w", expected.taskID, err)
		}
		return true, true, nil
	}

	if strings.TrimSpace(record.DocumentID) != "" && strings.TrimSpace(record.DocumentID) != expected.documentID {
		return false, false, fmt.Errorf(
			"knowledge document chunk log task %q belongs to document %q, not %q",
			expected.taskID,
			record.DocumentID,
			expected.documentID,
		)
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
		return false, false, nil
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
		return false, false, fmt.Errorf("reconcile knowledge document chunk log %q: %w", record.ID, err)
	}
	return true, false, nil
}

func (s *KnowledgeDocumentService) recordIngestionReconcileEvent(
	source string,
	input KnowledgeDocumentIngestionTaskCompletedInput,
	result ingestionReconcileResult,
	err error,
) {
	if s == nil || s.reconcileRecorder == nil {
		return
	}
	s.reconcileRecorder.RecordKnowledgeDocumentIngestionReconcile(KnowledgeDocumentIngestionReconcileEvent{
		Source:          strings.TrimSpace(source),
		TaskID:          strings.TrimSpace(input.TaskID),
		DocumentID:      strings.TrimSpace(input.DocumentID),
		Skipped:         result.skipped,
		DocumentUpdated: result.documentUpdated,
		ChunkLogUpdated: result.chunkLogUpdated,
		ChunkLogCreated: result.chunkLogCreated,
		ErrorMessage:    errorMessageOf(err),
	})
}

func errorMessageOf(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
