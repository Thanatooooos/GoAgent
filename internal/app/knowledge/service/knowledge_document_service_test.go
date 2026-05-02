package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type knowledgeDocumentServiceDocumentRepoStub struct {
	getByIDFn      func(ctx context.Context, id string) (domain.KnowledgeDocument, error)
	updateFieldsFn func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error)
	deleteFn       func(ctx context.Context, id string) error
}

func (s knowledgeDocumentServiceDocumentRepoStub) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return document, nil
}

func (s knowledgeDocumentServiceDocumentRepoStub) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
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
