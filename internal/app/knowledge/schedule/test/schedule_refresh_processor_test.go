package schedule_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
)

type processorScheduleRepository struct {
	schedule      domain.KnowledgeDocumentSchedule
	updateWhereFn func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error)
	renewLockFn   func(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error)
}

func (r processorScheduleRepository) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (r processorScheduleRepository) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (r processorScheduleRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	if r.updateWhereFn != nil {
		return r.updateWhereFn(ctx, cond, patch)
	}
	return 1, nil
}

func (r processorScheduleRepository) Delete(ctx context.Context, id string) error { return nil }

func (r processorScheduleRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (r processorScheduleRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	if r.schedule.ID == id {
		return r.schedule, nil
	}
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (r processorScheduleRepository) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return domain.KnowledgeDocumentSchedule{}, nil
}

func (r processorScheduleRepository) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	return nil, nil
}

func (r processorScheduleRepository) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (r processorScheduleRepository) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	if r.renewLockFn != nil {
		return r.renewLockFn(ctx, lease, lockUntil)
	}
	return true, nil
}

func (r processorScheduleRepository) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	return true, nil
}

type processorDocumentRepository struct {
	document       domain.KnowledgeDocument
	updateFieldsFn func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error)
}

func (r processorDocumentRepository) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (r processorDocumentRepository) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (r processorDocumentRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	return 0, nil
}

func (r processorDocumentRepository) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	if r.updateFieldsFn != nil {
		return r.updateFieldsFn(ctx, where, set)
	}
	return 1, nil
}

func (r processorDocumentRepository) Delete(ctx context.Context, id string) error { return nil }

func (r processorDocumentRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	if r.document.ID == id {
		return r.document, nil
	}
	return domain.KnowledgeDocument{}, nil
}

func (r processorDocumentRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (r processorDocumentRepository) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (r processorDocumentRepository) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	return nil, nil
}

type processorExecRepository struct {
	createFn      func(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error)
	updateWhereFn func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error)
}

func (r processorExecRepository) Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	if r.createFn != nil {
		return r.createFn(ctx, exec)
	}
	return exec, nil
}

func (r processorExecRepository) Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (r processorExecRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
	if r.updateWhereFn != nil {
		return r.updateWhereFn(ctx, cond, patch)
	}
	return 1, nil
}

func (r processorExecRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error) {
	return domain.KnowledgeDocumentScheduleExec{}, nil
}

func (r processorExecRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (r processorExecRepository) List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error) {
	return nil, nil
}

type processorRemoteFetcher struct {
	result schedule.RemoteFetchResult
	err    error
}

func (f processorRemoteFetcher) FetchIfChanged(ctx context.Context, rawURL string, lastETag string, lastModified string, lastContentHash string, fallbackFileName string) (schedule.RemoteFetchResult, error) {
	return f.result, f.err
}

func TestScheduleRefreshProcessorMarksSkippedWhenRemoteUnchanged(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	var scheduleStatus string
	var execStatus string
	scheduleRepo := processorScheduleRepository{
		schedule: domain.KnowledgeDocumentSchedule{
			ID:              "schedule-1",
			DocumentID:      "doc-1",
			KnowledgeBaseID: "kb-1",
			LastETag:        `"v1"`,
		},
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
			if patch.LastStatus.Set {
				scheduleStatus = patch.LastStatus.Value
			}
			return 1, nil
		},
	}
	execRepo := processorExecRepository{
		createFn: func(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
			exec.ID = "exec-1"
			return exec, nil
		},
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
			if patch.Status.Set {
				execStatus = patch.Status.Value
			}
			return 1, nil
		},
	}
	documentRepo := processorDocumentRepository{document: scheduledURLDocument()}

	processor := schedule.NewScheduleRefreshProcessor(schedule.ScheduleRefreshProcessorOptions{
		ScheduleRepo:      scheduleRepo,
		DocumentRepo:      documentRepo,
		ExecRepo:          execRepo,
		Storage:           fakeFileStorage{},
		LockManager:       schedule.NewScheduleLockManager(scheduleRepo, schedule.ScheduleLockOptions{Now: func() time.Time { return now }}),
		RemoteFileFetcher: processorRemoteFetcher{result: schedule.RemoteFetchResult{Changed: false, Message: "remote file unchanged", ETag: `"v1"`}},
		Now:               func() time.Time { return now },
		NextID:            func() (int64, error) { return 1, nil },
	})

	err := processor.Process(context.Background(), domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if scheduleStatus != domain.KnowledgeDocumentScheduleRunStatusSkipped || execStatus != domain.KnowledgeDocumentScheduleRunStatusSkipped {
		t.Fatalf("expected skipped statuses, schedule=%q exec=%q", scheduleStatus, execStatus)
	}
}

func TestScheduleRefreshProcessorStoresChangedFileAndMarksSuccess(t *testing.T) {
	t.Parallel()

	tempFile := writeTempFile(t, "fresh content")
	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	var uploaded string
	var sawDocumentSuccess bool
	var execStatus string
	scheduleRepo := processorScheduleRepository{
		schedule: domain.KnowledgeDocumentSchedule{
			ID:              "schedule-1",
			DocumentID:      "doc-1",
			KnowledgeBaseID: "kb-1",
		},
	}
	documentRepo := processorDocumentRepository{
		document: scheduledURLDocument(),
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
			for _, assignment := range set {
				if assignment.Field == port.KnowledgeDocument.Status.Key && assignment.Value == domain.KnowledgeDocumentStatusSuccess {
					sawDocumentSuccess = true
				}
			}
			return 1, nil
		},
	}
	execRepo := processorExecRepository{
		createFn: func(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
			exec.ID = "exec-1"
			return exec, nil
		},
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
			if patch.Status.Set {
				execStatus = patch.Status.Value
			}
			return 1, nil
		},
	}
	storage := fakeFileStorage{
		uploadFn: func(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
			data, err := io.ReadAll(file.Body)
			if err != nil {
				t.Fatalf("ReadAll(upload body) error = %v", err)
			}
			uploaded = string(data)
			return port.StoredFile{Key: file.Key, FileName: file.FileName, ContentType: file.ContentType, Size: file.Size}, nil
		},
	}
	processor := schedule.NewScheduleRefreshProcessor(schedule.ScheduleRefreshProcessorOptions{
		ScheduleRepo: scheduleRepo,
		DocumentRepo: documentRepo,
		ExecRepo:     execRepo,
		Storage:      storage,
		LockManager:  schedule.NewScheduleLockManager(scheduleRepo, schedule.ScheduleLockOptions{Now: func() time.Time { return now }}),
		RemoteFileFetcher: processorRemoteFetcher{result: schedule.RemoteFetchResult{
			Changed:      true,
			TempFile:     tempFile,
			Size:         int64(len("fresh content")),
			ContentType:  "text/markdown",
			FileName:     "fresh.md",
			ContentHash:  "hash-1",
			ETag:         `"v2"`,
			LastModified: "Tue, 28 Apr 2026 00:00:00 GMT",
		}},
		Now:    func() time.Time { return now },
		NextID: func() (int64, error) { return 1, nil },
	})

	err := processor.Process(context.Background(), domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if uploaded != "fresh content" {
		t.Fatalf("unexpected uploaded content: %q", uploaded)
	}
	if !sawDocumentSuccess {
		t.Fatal("expected refreshed document to be marked success")
	}
	if execStatus != domain.KnowledgeDocumentScheduleRunStatusSuccess {
		t.Fatalf("expected exec success status, got %q", execStatus)
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatalf("expected remote temp file to be removed, stat err=%v", err)
	}
}

func TestScheduleRefreshProcessorDisablesInvalidCron(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	var disabled bool
	scheduleRepo := processorScheduleRepository{
		schedule: domain.KnowledgeDocumentSchedule{ID: "schedule-1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1"},
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
			if patch.Enabled.Set && !patch.Enabled.Value {
				disabled = true
			}
			return 1, nil
		},
	}
	document := scheduledURLDocument()
	document.ScheduleCron = "0 0 25 * * *"
	processor := schedule.NewScheduleRefreshProcessor(schedule.ScheduleRefreshProcessorOptions{
		ScheduleRepo:      scheduleRepo,
		DocumentRepo:      processorDocumentRepository{document: document},
		ExecRepo:          processorExecRepository{},
		Storage:           fakeFileStorage{},
		LockManager:       schedule.NewScheduleLockManager(scheduleRepo, schedule.ScheduleLockOptions{Now: func() time.Time { return now }}),
		RemoteFileFetcher: processorRemoteFetcher{},
		Now:               func() time.Time { return now },
		NextID:            func() (int64, error) { return 1, nil },
	})

	err := processor.Process(context.Background(), domain.KnowledgeDocumentScheduleLockLease{ScheduleID: "schedule-1", LockToken: "owner-1"})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if !disabled {
		t.Fatal("invalid cron should disable schedule")
	}
}

func scheduledURLDocument() domain.KnowledgeDocument {
	return domain.KnowledgeDocument{
		ID:              "doc-1",
		KnowledgeBaseID: "kb-1",
		Name:            "demo.md",
		Enabled:         true,
		FileURL:         "old-key",
		FileType:        "text/markdown",
		ProcessMode:     domain.KnowledgeDocumentProcessModeChunk,
		Status:          domain.KnowledgeDocumentStatusSuccess,
		SourceType:      domain.KnowledgeDocumentSourceURL,
		SourceLocation:  "https://example.com/demo.md",
		ScheduleEnabled: true,
		ScheduleCron:    "0 */5 * * * *",
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "remote-*.tmp")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return file.Name()
}
