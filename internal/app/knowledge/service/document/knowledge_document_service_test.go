package document

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type knowledgeDocumentServiceDocumentRepoStub struct {
	getByIDFn      func(ctx context.Context, id string) (domain.KnowledgeDocument, error)
	listFn         func(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error)
	updateFn       func(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error)
	updateFieldsFn func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error)
	deleteFn       func(ctx context.Context, id string) error
}

func (s knowledgeDocumentServiceDocumentRepoStub) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return document, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, document)
	}
	return document, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	if s.updateFieldsFn != nil {
		return s.updateFieldsFn(ctx, where, set)
	}
	return 0, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) Delete(ctx context.Context, id string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return domain.KnowledgeDocument{}, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type knowledgeDocumentServiceChunkRepoStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s knowledgeDocumentServiceChunkRepoStub) Create(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	return chunk, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error {
	return nil
}

func (s knowledgeDocumentServiceChunkRepoStub) Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	return chunk, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s knowledgeDocumentServiceChunkRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

func (s knowledgeDocumentServiceChunkRepoStub) UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error) {
	return domain.KnowledgeChunk{}, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceChunkRepoStub) List(ctx context.Context, filter port.KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error) {
	return nil, nil
}

type knowledgeDocumentServiceVectorStoreStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s knowledgeDocumentServiceVectorStoreStub) UpsertDocumentChunks(ctx context.Context, chunks []port.ChunkVector) error {
	return nil
}

func (s knowledgeDocumentServiceVectorStoreStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

type knowledgeDocumentServiceIngestionTaskReaderStub struct {
	getTaskFn      func(ctx context.Context, taskID string) (ingestiondomain.Task, error)
	listTaskNodeFn func(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

func (s knowledgeDocumentServiceIngestionTaskReaderStub) GetKnowledgePipelineTask(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
	if s.getTaskFn != nil {
		return s.getTaskFn(ctx, taskID)
	}
	return ingestiondomain.Task{}, nil
}

func (s knowledgeDocumentServiceIngestionTaskReaderStub) ListKnowledgePipelineTaskNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error) {
	if s.listTaskNodeFn != nil {
		return s.listTaskNodeFn(ctx, taskID)
	}
	return nil, nil
}

type knowledgeDocumentServiceIngestionReconcileRecorderStub struct {
	recordFn func(event KnowledgeDocumentIngestionReconcileEvent)
}

func (s knowledgeDocumentServiceIngestionReconcileRecorderStub) RecordKnowledgeDocumentIngestionReconcile(event KnowledgeDocumentIngestionReconcileEvent) {
	if s.recordFn != nil {
		s.recordFn(event)
	}
}

func (s knowledgeDocumentServiceVectorStoreStub) DeleteChunk(ctx context.Context, chunkID string) error {
	return nil
}

func (s knowledgeDocumentServiceVectorStoreStub) DeleteChunks(ctx context.Context, chunkIDs []string) error {
	return nil
}

func (s knowledgeDocumentServiceVectorStoreStub) UpdateChunk(ctx context.Context, chunk port.ChunkVector) error {
	return nil
}

type knowledgeDocumentServiceScheduleRepoStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s knowledgeDocumentServiceScheduleRepoStub) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return schedule, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return schedule, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	return nil, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return false, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	return false, nil
}

func (s knowledgeDocumentServiceScheduleRepoStub) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	return false, nil
}

type knowledgeDocumentServiceScheduleExecRepoStub struct {
	deleteByDocumentIDFn func(ctx context.Context, documentID string) error
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return exec, nil
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return exec, nil
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
	return 0, nil
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.deleteByDocumentIDFn != nil {
		return s.deleteByDocumentIDFn(ctx, documentID)
	}
	return nil
}

func (s knowledgeDocumentServiceScheduleExecRepoStub) List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error) {
	return nil, nil
}

type knowledgeDocumentServiceStorageStub struct {
	deleteFn func(ctx context.Context, key string) error
}

func (s knowledgeDocumentServiceStorageStub) Upload(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
	return port.StoredFile{}, nil
}

func (s knowledgeDocumentServiceStorageStub) Delete(ctx context.Context, key string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, key)
	}
	return nil
}

func (s knowledgeDocumentServiceStorageStub) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, nil
}

type knowledgeDocumentServiceChunkLogRepoStub struct {
	createFn           func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error)
	getByTaskIDFn      func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error)
	listByDocumentIDFn func(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error)
	updateFn           func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error)
}

func (s knowledgeDocumentServiceChunkLogRepoStub) Create(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	if s.createFn != nil {
		return s.createFn(ctx, log)
	}
	return log, nil
}

func (s knowledgeDocumentServiceChunkLogRepoStub) Update(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, log)
	}
	return log, nil
}

func (s knowledgeDocumentServiceChunkLogRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentChunkLog, error) {
	return domain.KnowledgeDocumentChunkLog{}, nil
}

func (s knowledgeDocumentServiceChunkLogRepoStub) GetByTaskID(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
	if s.getByTaskIDFn != nil {
		return s.getByTaskIDFn(ctx, taskID)
	}
	return domain.KnowledgeDocumentChunkLog{}, nil
}

func (s knowledgeDocumentServiceChunkLogRepoStub) ListByDocumentID(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error) {
	if s.listByDocumentIDFn != nil {
		return s.listByDocumentIDFn(ctx, documentID, options)
	}
	return nil, nil
}

func TestKnowledgeDocumentServiceDeleteUsesTransactionalCleanup(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 6)
	document := domain.KnowledgeDocument{
		ID:             "doc-1",
		Status:         domain.KnowledgeDocumentStatusSuccess,
		SourceType:     domain.KnowledgeDocumentSourceURL,
		FileURL:        "knowledge/demo.txt",
		ScheduleCron:   "0 */5 * * * *",
		Enabled:        true,
		SourceLocation: "https://example.com/demo.txt",
	}

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return document, nil
			},
		},
		nil,
		nil,
		nil,
		knowledgeDocumentServiceStorageStub{
			deleteFn: func(ctx context.Context, key string) error {
				order = append(order, "file")
				if key != document.FileURL {
					t.Fatalf("unexpected file key: %q", key)
				}
				return nil
			},
		},
		nil,
		nil,
		nil,
		func(
			ctx context.Context,
			fn func(
				ctx context.Context,
				documentRepo port.KnowledgeDocumentRepository,
				chunkRepo port.KnowledgeChunkRepository,
				vectorStore port.VectorStore,
				scheduleRepo port.KnowledgeDocumentScheduleRepository,
				scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
			) error,
		) error {
			return fn(
				ctx,
				knowledgeDocumentServiceDocumentRepoStub{
					updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
						order = append(order, "mark-deleting")
						assertDocumentStatusIn(t, where, domain.KnowledgeDocumentStatusPending, domain.KnowledgeDocumentStatusFailed, domain.KnowledgeDocumentStatusSuccess)
						assertDocumentAssignment(t, set, port.KnowledgeDocument.Status.Key, domain.KnowledgeDocumentStatusDeleting)
						return 1, nil
					},
					deleteFn: func(ctx context.Context, id string) error {
						order = append(order, "document")
						return nil
					},
				},
				knowledgeDocumentServiceChunkRepoStub{
					deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
						order = append(order, "chunk")
						return nil
					},
				},
				knowledgeDocumentServiceVectorStoreStub{
					deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
						order = append(order, "vector")
						return nil
					},
				},
				knowledgeDocumentServiceScheduleRepoStub{
					deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
						order = append(order, "schedule")
						return nil
					},
				},
				knowledgeDocumentServiceScheduleExecRepoStub{
					deleteByDocumentIDFn: func(ctx context.Context, documentID string) error {
						order = append(order, "schedule-exec")
						return nil
					},
				},
			)
		},
	)

	if err := svc.Delete(context.Background(), DeleteKnowledgeDocumentInput{
		DocumentID: document.ID,
		OperatorID: "alice",
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got := strings.Join(order, ",")
	want := "mark-deleting,schedule-exec,schedule,vector,chunk,document,file"
	if got != want {
		t.Fatalf("unexpected delete order: got=%s want=%s", got, want)
	}
}

func TestKnowledgeDocumentServiceOnIngestionTaskCompletedUsesTaskScopedChunkLog(t *testing.T) {
	t.Parallel()

	updated := domain.KnowledgeDocumentChunkLog{}
	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{ID: id, Status: domain.KnowledgeDocumentStatusRunning}, nil
			},
			updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
				return 1, nil
			},
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{
					ID:         taskID,
					DocumentID: "doc-1",
					Status:     domain.KnowledgeDocumentChunkLogStatusRunning,
				}, nil
			},
			listByDocumentIDFn: func(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error) {
				t.Fatal("expected task-scoped chunk log lookup before latest-document fallback")
				return nil, nil
			},
			updateFn: func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
				updated = log
				return log, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := svc.OnIngestionTaskCompleted(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:      "task-1",
		DocumentID:  "doc-1",
		ChunkCount:  3,
		OperatorID:  "tester",
		CompletedAt: timePointer(time.Now()),
	})
	if err != nil {
		t.Fatalf("OnIngestionTaskCompleted() error = %v", err)
	}
	if updated.ID != "task-1" {
		t.Fatalf("expected task-scoped chunk log update, got id=%q", updated.ID)
	}
	if updated.ChunkCount != 3 {
		t.Fatalf("expected chunk count 3, got %d", updated.ChunkCount)
	}
	if updated.Status != domain.KnowledgeDocumentChunkLogStatusSuccess {
		t.Fatalf("expected success chunk log status, got %q", updated.Status)
	}
}

func TestKnowledgeDocumentServiceOnIngestionTaskCompletedReturnsChunkLogMismatch(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{ID: id, Status: domain.KnowledgeDocumentStatusRunning}, nil
			},
			updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
				return 1, nil
			},
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{
					ID:         taskID,
					DocumentID: "doc-other",
					Status:     domain.KnowledgeDocumentChunkLogStatusRunning,
				}, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	err := svc.OnIngestionTaskCompleted(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:     "task-1",
		DocumentID: "doc-1",
		ChunkCount: 1,
	})
	if err == nil {
		t.Fatal("expected mismatch error when task-scoped chunk log belongs to another document")
	}
	if !strings.Contains(err.Error(), "belongs to document") {
		t.Fatalf("expected document mismatch error, got %v", err)
	}
}

func TestKnowledgeDocumentServiceOnIngestionTaskCompletedReconcilesDocumentStatusFromTask(t *testing.T) {
	t.Parallel()

	var updatedDocument domain.KnowledgeDocument
	var updatedLog domain.KnowledgeDocumentChunkLog

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:            id,
					Status:        domain.KnowledgeDocumentStatusRunning,
					ProcessMode:   domain.KnowledgeDocumentProcessModePipeline,
					ChunkStrategy: "markdown",
					PipelineID:    "pipeline-1",
				}, nil
			},
			updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
				return 1, nil
			},
			updateFn: nil,
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{
					ID:         taskID,
					DocumentID: "doc-1",
					Status:     domain.KnowledgeDocumentChunkLogStatusSuccess,
					ChunkCount: 9,
				}, nil
			},
			updateFn: func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
				updatedLog = log
				return log, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.documentRepo = knowledgeDocumentServiceDocumentRepoStub{
		getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
			return domain.KnowledgeDocument{
				ID:            id,
				Status:        domain.KnowledgeDocumentStatusRunning,
				ProcessMode:   domain.KnowledgeDocumentProcessModePipeline,
				ChunkStrategy: "markdown",
				PipelineID:    "pipeline-1",
			}, nil
		},
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
			return 1, nil
		},
		updateFn: func(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
			updatedDocument = document
			return document, nil
		},
	}
	svc.SetIngestionTaskReader(knowledgeDocumentServiceIngestionTaskReaderStub{
		getTaskFn: func(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
			completedAt := time.Now()
			return ingestiondomain.Task{
				ID:           taskID,
				Status:       ingestiondomain.TaskStatusFailed,
				ChunkCount:   0,
				ErrorMessage: "indexer failed",
				CompletedAt:  &completedAt,
				Metadata: map[string]any{
					"documentId": "doc-1",
				},
				UpdatedBy: "tester",
			}, nil
		},
	})

	err := svc.OnIngestionTaskCompleted(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:       "task-1",
		DocumentID:   "doc-1",
		ChunkCount:   3,
		OperatorID:   "tester",
		ErrorMessage: "indexer failed",
	})
	if err != nil {
		t.Fatalf("OnIngestionTaskCompleted() error = %v", err)
	}
	if updatedDocument.Status != domain.KnowledgeDocumentStatusFailed {
		t.Fatalf("expected reconciled document status failed, got %q", updatedDocument.Status)
	}
	if updatedLog.Status != domain.KnowledgeDocumentChunkLogStatusFailed {
		t.Fatalf("expected reconciled chunk log status failed, got %q", updatedLog.Status)
	}
	if updatedLog.ErrorMessage != "indexer failed" {
		t.Fatalf("expected reconciled chunk log error, got %q", updatedLog.ErrorMessage)
	}
}

func TestKnowledgeDocumentServiceReconcileIngestionTaskCompletionCreatesMissingChunkLog(t *testing.T) {
	t.Parallel()

	var createdLog domain.KnowledgeDocumentChunkLog

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:            id,
					Status:        domain.KnowledgeDocumentStatusSuccess,
					ProcessMode:   domain.KnowledgeDocumentProcessModePipeline,
					ChunkStrategy: "markdown",
					PipelineID:    "pipeline-1",
				}, nil
			},
			updateFn: func(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
				return document, nil
			},
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{}, nil
			},
			createFn: func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
				createdLog = log
				return log, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.SetIngestionTaskReader(knowledgeDocumentServiceIngestionTaskReaderStub{
		getTaskFn: func(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
			startedAt := time.Now().Add(-time.Minute)
			completedAt := time.Now()
			return ingestiondomain.Task{
				ID:          taskID,
				Status:      ingestiondomain.TaskStatusSuccess,
				ChunkCount:  6,
				StartedAt:   &startedAt,
				CompletedAt: &completedAt,
				PipelineID:  "pipeline-1",
				Metadata: map[string]any{
					"documentId": "doc-1",
				},
			}, nil
		},
	})

	err := svc.ReconcileIngestionTaskCompletion(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:     "task-1",
		DocumentID: "doc-1",
		ChunkCount: 6,
	})
	if err != nil {
		t.Fatalf("ReconcileIngestionTaskCompletion() error = %v", err)
	}
	if createdLog.ID != "task-1" {
		t.Fatalf("expected created chunk log id task-1, got %q", createdLog.ID)
	}
	if createdLog.DocumentID != "doc-1" {
		t.Fatalf("expected created chunk log document doc-1, got %q", createdLog.DocumentID)
	}
	if createdLog.Status != domain.KnowledgeDocumentChunkLogStatusSuccess {
		t.Fatalf("expected created chunk log success, got %q", createdLog.Status)
	}
	if createdLog.ChunkCount != 6 {
		t.Fatalf("expected created chunk log chunk count 6, got %d", createdLog.ChunkCount)
	}
}

func TestKnowledgeDocumentServiceReconcileIngestionTaskCompletionRejectsTaskDocumentMismatch(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{ID: id, Status: domain.KnowledgeDocumentStatusRunning}, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.SetIngestionTaskReader(knowledgeDocumentServiceIngestionTaskReaderStub{
		getTaskFn: func(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
			return ingestiondomain.Task{
				ID:     taskID,
				Status: ingestiondomain.TaskStatusFailed,
				Metadata: map[string]any{
					"documentId": "doc-other",
				},
			}, nil
		},
	})

	err := svc.ReconcileIngestionTaskCompletion(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:     "task-1",
		DocumentID: "doc-1",
	})
	if err == nil {
		t.Fatal("expected task/document mismatch error")
	}
	if !strings.Contains(err.Error(), "belongs to document") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestKnowledgeDocumentServiceReconcileIngestionTaskCompletionRecordsMetricsEvent(t *testing.T) {
	t.Parallel()

	events := make([]KnowledgeDocumentIngestionReconcileEvent, 0, 2)

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:          id,
					Status:      domain.KnowledgeDocumentStatusRunning,
					ProcessMode: domain.KnowledgeDocumentProcessModePipeline,
					PipelineID:  "pipeline-1",
				}, nil
			},
			updateFn: func(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
				return document, nil
			},
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{}, nil
			},
			createFn: func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
				return log, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.SetIngestionTaskReader(knowledgeDocumentServiceIngestionTaskReaderStub{
		getTaskFn: func(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
			return ingestiondomain.Task{
				ID:         taskID,
				Status:     ingestiondomain.TaskStatusSuccess,
				ChunkCount: 4,
				PipelineID: "pipeline-1",
				Metadata: map[string]any{
					"documentId": "doc-1",
				},
			}, nil
		},
	})
	svc.SetIngestionReconcileRecorder(knowledgeDocumentServiceIngestionReconcileRecorderStub{
		recordFn: func(event KnowledgeDocumentIngestionReconcileEvent) {
			events = append(events, event)
		},
	})

	err := svc.ReconcileIngestionTaskCompletion(context.Background(), KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:     "task-1",
		DocumentID: "doc-1",
		ChunkCount: 4,
	})
	if err != nil {
		t.Fatalf("ReconcileIngestionTaskCompletion() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 reconcile event, got %d", len(events))
	}
	if events[0].Source != reconcileSourceTaskCompletion {
		t.Fatalf("unexpected event source %q", events[0].Source)
	}
	if !events[0].DocumentUpdated || !events[0].ChunkLogCreated || !events[0].ChunkLogUpdated {
		t.Fatalf("unexpected reconcile event: %+v", events[0])
	}
	if events[0].ErrorMessage != "" || events[0].Skipped {
		t.Fatalf("expected successful reconcile event, got %+v", events[0])
	}
}

func TestKnowledgeDocumentServiceScanAndReconcileIngestionTasksUsesLatestChunkLogTaskID(t *testing.T) {
	t.Parallel()

	calledTaskIDs := make([]string, 0, 3)
	updatedDocuments := make([]domain.KnowledgeDocument, 0, 1)

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			listFn: func(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
				if filter.ListOptions.Offset > 0 {
					return nil, nil
				}
				switch filter.Status {
				case domain.KnowledgeDocumentStatusRunning:
					return []domain.KnowledgeDocument{
						{
							ID:          "doc-pipeline",
							ProcessMode: domain.KnowledgeDocumentProcessModePipeline,
							Status:      domain.KnowledgeDocumentStatusRunning,
							PipelineID:  "pipeline-1",
						},
						{
							ID:          "doc-chunk",
							ProcessMode: domain.KnowledgeDocumentProcessModeChunk,
							Status:      domain.KnowledgeDocumentStatusRunning,
						},
					}, nil
				default:
					return nil, nil
				}
			},
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:          id,
					ProcessMode: domain.KnowledgeDocumentProcessModePipeline,
					Status:      domain.KnowledgeDocumentStatusRunning,
					PipelineID:  "pipeline-1",
				}, nil
			},
			updateFn: func(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
				updatedDocuments = append(updatedDocuments, document)
				return document, nil
			},
		},
		nil,
		knowledgeDocumentServiceChunkLogRepoStub{
			listByDocumentIDFn: func(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error) {
				if documentID != "doc-pipeline" {
					t.Fatalf("unexpected chunk log lookup for document %q", documentID)
				}
				return []domain.KnowledgeDocumentChunkLog{
					{
						ID:         "task-123",
						DocumentID: documentID,
						Status:     domain.KnowledgeDocumentChunkLogStatusRunning,
					},
				}, nil
			},
			getByTaskIDFn: func(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
				return domain.KnowledgeDocumentChunkLog{
					ID:         taskID,
					DocumentID: "doc-pipeline",
					Status:     domain.KnowledgeDocumentChunkLogStatusRunning,
				}, nil
			},
			updateFn: func(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
				return log, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	svc.SetIngestionTaskReader(knowledgeDocumentServiceIngestionTaskReaderStub{
		getTaskFn: func(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
			calledTaskIDs = append(calledTaskIDs, taskID)
			return ingestiondomain.Task{
				ID:         taskID,
				Status:     ingestiondomain.TaskStatusSuccess,
				ChunkCount: 4,
				Metadata: map[string]any{
					"documentId": "doc-pipeline",
				},
			}, nil
		},
	})

	err := svc.ScanAndReconcileIngestionTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("ScanAndReconcileIngestionTasks() error = %v", err)
	}
	if len(calledTaskIDs) != 1 || calledTaskIDs[0] != "task-123" {
		t.Fatalf("expected reconcile to load task-123 once, got %+v", calledTaskIDs)
	}
	if len(updatedDocuments) != 1 || updatedDocuments[0].Status != domain.KnowledgeDocumentStatusSuccess {
		t.Fatalf("expected pipeline document status to be reconciled to success, got %+v", updatedDocuments)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func TestKnowledgeDocumentServiceDeleteSkipsStorageCleanupWhenTransactionFails(t *testing.T) {
	t.Parallel()

	storageDeleted := false
	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:         "doc-1",
					Status:     domain.KnowledgeDocumentStatusSuccess,
					FileURL:    "knowledge/demo.txt",
					SourceType: domain.KnowledgeDocumentSourceFile,
				}, nil
			},
		},
		nil,
		nil,
		nil,
		knowledgeDocumentServiceStorageStub{
			deleteFn: func(ctx context.Context, key string) error {
				storageDeleted = true
				return nil
			},
		},
		nil,
		nil,
		nil,
		func(
			ctx context.Context,
			fn func(
				ctx context.Context,
				documentRepo port.KnowledgeDocumentRepository,
				chunkRepo port.KnowledgeChunkRepository,
				vectorStore port.VectorStore,
				scheduleRepo port.KnowledgeDocumentScheduleRepository,
				scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
			) error,
		) error {
			return errors.New("tx boom")
		},
	)

	err := svc.Delete(context.Background(), DeleteKnowledgeDocumentInput{
		DocumentID: "doc-1",
		OperatorID: "alice",
	})
	if err == nil || err.Error() != "tx boom" {
		t.Fatalf("Delete() error = %v", err)
	}
	if storageDeleted {
		t.Fatal("storage cleanup should not run when transactional delete fails")
	}
}

func TestNormalizeKnowledgeDocumentProcessModeRejectsUnsupportedValue(t *testing.T) {
	t.Parallel()

	_, err := normalizeKnowledgeDocumentProcessMode("custom_mode")
	if err == nil {
		t.Fatal("expected unsupported process mode to fail")
	}
}

func TestKnowledgeDocumentServiceUpdateRejectsInvalidChunkStrategy(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:          id,
					ProcessMode: domain.KnowledgeDocumentProcessModeChunk,
				}, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Update(context.Background(), UpdateKnowledgeDocumentInput{
		DocumentID:    "doc-1",
		ChunkStrategy: "bad_strategy",
		OperatorID:    "alice",
	})
	if err == nil {
		t.Fatal("expected invalid chunk strategy to fail")
	}
}

func TestKnowledgeDocumentServiceUpdateRequiresPipelineIDForPipelineMode(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:          id,
					ProcessMode: domain.KnowledgeDocumentProcessModeChunk,
				}, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Update(context.Background(), UpdateKnowledgeDocumentInput{
		DocumentID:  "doc-1",
		ProcessMode: domain.KnowledgeDocumentProcessModePipeline,
		OperatorID:  "alice",
	})
	if err == nil {
		t.Fatal("expected pipeline mode without pipeline id to fail")
	}
}

func TestKnowledgeDocumentServiceUpdateRejectsPipelineIDInChunkMode(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeDocumentService(
		nil,
		knowledgeDocumentServiceDocumentRepoStub{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
				return domain.KnowledgeDocument{
					ID:          id,
					ProcessMode: domain.KnowledgeDocumentProcessModeChunk,
				}, nil
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.Update(context.Background(), UpdateKnowledgeDocumentInput{
		DocumentID: "doc-1",
		PipelineID: "pipe-1",
		OperatorID: "alice",
	})
	if err == nil {
		t.Fatal("expected chunk mode with pipeline id to fail")
	}
}

func assertDocumentStatusIn(t *testing.T, predicates port.UpdatePredicates, values ...string) {
	t.Helper()
	for _, predicate := range predicates {
		if predicate.Field != port.KnowledgeDocument.Status.Key || predicate.Operator != port.OperatorIn {
			continue
		}
		if len(predicate.Values) != len(values) {
			break
		}
		matched := true
		for i, value := range values {
			if predicate.Values[i] != value {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("missing status in predicate with values=%v in %+v", values, predicates)
}

func assertDocumentAssignment(t *testing.T, assignments port.UpdateAssignments, field port.FieldKey, value any) {
	t.Helper()
	for _, assignment := range assignments {
		if assignment.Field == field && assignment.Value == value {
			return
		}
	}
	t.Fatalf("missing assignment field=%s value=%v in %+v", field, value, assignments)
}
