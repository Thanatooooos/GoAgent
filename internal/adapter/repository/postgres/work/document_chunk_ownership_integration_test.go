package work_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	taskqueue "local/rag-project/internal/adapter/taskqueue/goroutine"
	pgvector "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/app/knowledge/service"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type chunkStorage struct {
	port.FileStorage
	text string
}

func (s chunkStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.text)), nil
}

type chunkEmbedding struct {
	aiembedding.EmbeddingService
	value            float32
	started, release chan struct{}
	failure          error
	once             sync.Once
	calls            atomic.Int32
}

func (e *chunkEmbedding) EmbedBatchWithModel(texts []string, _ string) ([][]float32, error) {
	e.calls.Add(1)
	if os.Getenv("CODEX_CHUNK_CRASH_STAGE") == "running" {
		os.Exit(77)
	}
	if e.started != nil {
		e.once.Do(func() { close(e.started) })
		<-e.release
	}
	if e.failure != nil {
		return nil, e.failure
	}
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = []float32{e.value, 1, 0}
	}
	return result, nil
}
func (e *chunkEmbedding) EmbedBatch(s []string) ([][]float32, error) {
	return e.EmbedBatchWithModel(s, "")
}
func (e *chunkEmbedding) Embed(s string) ([]float32, error) {
	v, err := e.EmbedBatch([]string{s})
	if err != nil {
		return nil, err
	}
	return v[0], nil
}
func (e *chunkEmbedding) EmbedWithModel(s, m string) ([]float32, error) { return e.Embed(s) }
func (e *chunkEmbedding) Dimension() int                                { return 3 }

type chunkWakeup struct {
	task port.ChunkDocumentTask
	err  error
}

func (w *chunkWakeup) SubmitChunkDocument(_ context.Context, task port.ChunkDocumentTask) error {
	w.task = task
	return w.err
}

type chunkFixture struct {
	db           *gorm.DB
	id, user, kb string
	jobs         *postgresknowledge.ChunkJobs
}

func newChunkFixture(t *testing.T) *chunkFixture {
	t.Helper()
	db := testDB(t)
	id := publicationID(t)
	kb := publicationID(t)
	user, _ := testIdentity(t)
	if _, err := postgresknowledge.NewKnowledgeBaseRepository(db).Create(context.Background(), domain.KnowledgeBase{ID: kb, Name: "ownership", EmbeddingModel: "fixture", CollectionName: kb, CreatedBy: user, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	doc := domain.NewUploadedKnowledgeDocument(id, kb, "source.txt", id+".txt", "txt", user, 100)
	doc.ChunkStrategy = "fixed_size"
	doc.ChunkConfig = []byte(`{"enableParentChild":false,"chunkSize":200,"overlap":0}`)
	if _, err := postgresknowledge.NewKnowledgeDocumentRepository(db, nil).Create(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	return &chunkFixture{db: db, id: id, user: user, kb: kb, jobs: postgresknowledge.NewChunkJobs(db)}
}
func (f *chunkFixture) command(wakeup port.TaskQueue) *service.KnowledgeDocumentService {
	s := service.NewKnowledgeDocumentService(postgresknowledge.NewKnowledgeBaseRepository(f.db), postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil), nil, postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db), nil, chunkStorage{}, wakeup, nil, nil, nil)
	s.SetChunkJobs(f.jobs)
	return s
}
func (f *chunkFixture) processor(text string, embedding *chunkEmbedding, ttl time.Duration) *service.DocumentProcessService {
	enrichment := false
	return service.NewDocumentProcessService(service.DocumentProcessServiceOptions{Jobs: f.jobs, LeaseDuration: ttl, BaseRepo: postgresknowledge.NewKnowledgeBaseRepository(f.db), DocumentRepo: postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil), ChunkRepo: postgresknowledge.NewKnowledgeChunkRepository(f.db), ChunkLogRepo: postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db), Storage: chunkStorage{text: text}, VectorStore: pgvector.NewVectorStore(f.db), Transaction: postgresknowledge.NewDocumentProcessTransaction(f.db), Embedding: embedding, EnrichmentEnabled: &enrichment})
}
func (f *chunkFixture) admit(t *testing.T) port.ChunkDocumentTask {
	t.Helper()
	wake := &chunkWakeup{err: errors.New("lost local wakeup")}
	if err := f.command(wake).StartChunk(context.Background(), service.StartChunkKnowledgeDocumentInput{DocumentID: f.id, OperatorID: f.user}); err != nil {
		t.Fatal(err)
	}
	if wake.task.TaskID == "" {
		t.Fatal("admission did not persist job identity")
	}
	return wake.task
}
func jobState(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var state string
	if err := db.Raw(`SELECT state FROM t_document_chunk_job WHERE id=?`, id).Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	return state
}
func (f *chunkFixture) assertResult(t *testing.T, task port.ChunkDocumentTask, text string) {
	t.Helper()
	if jobState(t, f.db, task.TaskID) != "completed" {
		t.Fatal("job not completed")
	}
	var chunks, vectors []string
	f.db.Raw(`SELECT content FROM t_knowledge_chunk WHERE doc_id=? AND deleted=0`, f.id).Scan(&chunks)
	f.db.Raw(`SELECT content FROM t_knowledge_chunk_vector WHERE doc_id=?`, f.id).Scan(&vectors)
	if len(chunks) == 0 || len(vectors) == 0 {
		t.Fatal("publication is missing chunks/vectors")
	}
	for _, s := range append(chunks, vectors...) {
		if !strings.Contains(s, text) {
			t.Fatalf("stale content %q", s)
		}
	}
	doc, err := postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil).GetByID(context.Background(), f.id)
	if err != nil || doc.Status != "success" {
		t.Fatalf("status: %+v %v", doc, err)
	}
	log, err := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db).GetByID(context.Background(), task.TaskID)
	if err != nil || log.Status != "success" || log.ChunkCount < 1 || log.StartTime == nil || log.EndTime == nil {
		t.Fatalf("processing log: %+v %v", log, err)
	}
}

func TestDocumentChunkLateExecutionCannotReplaceNewResult(t *testing.T) {
	for _, lateFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "late_success", true: "late_failure"}[lateFailure], func(t *testing.T) {
			f := newChunkFixture(t)
			ctx := context.Background()
			taskA := f.admit(t)
			embA := &chunkEmbedding{value: 1, started: make(chan struct{}), release: make(chan struct{})}
			if lateFailure {
				embA.failure = errors.New("old embedding failed")
			}
			procA := f.processor("generation A", embA, 3*time.Minute)
			done := make(chan error, 1)
			go func() {
				done <- procA.ExecuteChunk(ctx, service.ExecuteChunkInput{TaskID: taskA.TaskID, DocumentID: f.id})
			}()
			select {
			case <-embA.started:
			case <-time.After(2 * time.Second):
				t.Fatal("A did not start")
			}
			// A live heartbeat cannot be mistaken for a stale document update time.
			f.db.Exec(`UPDATE t_knowledge_document SET update_time=CURRENT_TIMESTAMP-INTERVAL '2 hours' WHERE id=?`, f.id)
			helper := schedule.NewDocumentStatusHelper(postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil))
			if _, err := helper.RecoverStuckRunning(ctx, 10); err != nil {
				t.Fatal(err)
			}
			if jobState(t, f.db, taskA.TaskID) != "running" {
				t.Fatal("legacy recovery revoked live job")
			}
			f.db.Exec(`UPDATE t_document_chunk_job SET lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=?`, taskA.TaskID)
			if _, err := f.jobs.RecoverExpired(ctx); err != nil {
				t.Fatal(err)
			}
			if jobState(t, f.db, taskA.TaskID) != "interrupted" {
				t.Fatal("expired A is not explicitly interrupted")
			}
			taskB := f.admit(t)
			embB := &chunkEmbedding{value: 2}
			procB := f.processor("generation B", embB, time.Minute)
			if err := procB.ExecuteChunk(ctx, service.ExecuteChunkInput{TaskID: taskB.TaskID, DocumentID: f.id}); err != nil {
				t.Fatal(err)
			}
			f.assertResult(t, taskB, "generation B")
			close(embA.release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("late A succeeded")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("A did not return")
			}
			f.assertResult(t, taskB, "generation B")
			log, err := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db).GetByID(ctx, taskA.TaskID)
			if err != nil || log.Status != "failed" {
				t.Fatalf("old log was revived: %+v %v", log, err)
			}
			if embB.calls.Load() != 1 {
				t.Fatal("B ran more than once")
			}
		})
	}
}

func TestDocumentChunkConcurrentClaimHeartbeatAndCommitRollback(t *testing.T) {
	f := newChunkFixture(t)
	ctx := context.Background()
	task := f.admit(t)
	var claims atomic.Int32
	var winner port.ChunkJob
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		owner := publicationID(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := f.jobs.Claim(ctx, task.TaskID, owner, time.Minute)
			if err != nil {
				t.Error(err)
			}
			if job != nil {
				claims.Add(1)
				mu.Lock()
				winner = *job
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claims=%d", claims.Load())
	}
	wrong := winner
	wrong.Owner = "another-owner"
	if !errors.Is(f.jobs.Renew(ctx, wrong, time.Minute), port.ErrChunkLeaseLost) {
		t.Fatal("wrong owner renewed")
	}
	if err := f.jobs.Renew(ctx, winner, time.Minute); err != nil {
		t.Fatal(err)
	}
	f.db.Exec(`UPDATE t_document_chunk_job SET lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=?`, task.TaskID)
	if !errors.Is(f.jobs.Renew(ctx, winner, time.Minute), port.ErrChunkLeaseLost) {
		t.Fatal("expired owner renewed")
	}
	if _, err := f.jobs.RecoverExpired(ctx); err != nil {
		t.Fatal(err)
	}
	// A worker paused before log creation cannot resurrect a running log after recovery.
	_, err := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db).Create(port.WithChunkJob(ctx, winner), domain.NewKnowledgeDocumentChunkLog(task.TaskID, f.id))
	if !errors.Is(err, port.ErrChunkLeaseLost) {
		t.Fatalf("stale worker created a new log: %v", err)
	}
	// A terminal-write error after chunk/vector writes rolls back the entire projection.
	task = f.admit(t)
	callback := "chunk/terminal-fault/" + task.TaskID
	f.db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "t_knowledge_document_chunk_log" {
			tx.AddError(errors.New("log completion failed"))
		}
	})
	proc := f.processor("rollback generation", &chunkEmbedding{value: 2}, time.Minute)
	err = proc.ExecuteChunk(ctx, service.ExecuteChunkInput{TaskID: task.TaskID, DocumentID: f.id})
	f.db.Callback().Update().Remove(callback)
	if err == nil || jobState(t, f.db, task.TaskID) != "failed" {
		t.Fatalf("failure not recorded: %v", err)
	}
	var count int64
	f.db.Table("t_knowledge_chunk_vector").Where("doc_id=?", f.id).Count(&count)
	if count != 0 {
		t.Fatal("failed commit exposed vectors")
	}
}

func TestDocumentChunkJobCrashHelper(t *testing.T) {
	stage := os.Getenv("CODEX_CHUNK_CRASH_STAGE")
	if stage == "" {
		t.Skip("subprocess helper")
	}
	db := testDB(t)
	f := &chunkFixture{db: db, id: os.Getenv("CODEX_CHUNK_DOCUMENT"), user: os.Getenv("CODEX_CHUNK_USER"), jobs: postgresknowledge.NewChunkJobs(db)}
	task := f.admit(t)
	if stage == "pending" {
		os.Exit(77)
	}
	f.processor("crashed generation", &chunkEmbedding{value: 1}, 200*time.Millisecond).ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: task.TaskID, DocumentID: f.id})
	t.Fatal("child did not exit")
}
func TestDocumentChunkJobProcessExitAndStartupRecovery(t *testing.T) {
	for _, stage := range []string{"pending", "running"} {
		t.Run(stage, func(t *testing.T) {
			f := newChunkFixture(t)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestDocumentChunkJobCrashHelper$")
			cmd.Env = append(os.Environ(), "CODEX_CHUNK_CRASH_STAGE="+stage, "CODEX_CHUNK_DOCUMENT="+f.id, "CODEX_CHUNK_USER="+f.user)
			out, err := cmd.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 77 {
				t.Fatalf("child: %s %v", out, err)
			}
			var taskID string
			f.db.Raw(`SELECT current_chunk_job_id FROM t_knowledge_document WHERE id=?`, f.id).Scan(&taskID)
			if stage == "running" {
				time.Sleep(250 * time.Millisecond)
				if _, err := f.jobs.RecoverExpired(context.Background()); err != nil {
					t.Fatal(err)
				}
				if jobState(t, f.db, taskID) != "interrupted" {
					t.Fatal("crashed running job not interrupted")
				}
				task := f.admit(t)
				taskID = task.TaskID
			}
			// Fresh worker startup, without the original goroutine or wake-up.
			f.db.Exec(`UPDATE t_document_chunk_job SET created_at='2000-01-01' WHERE id=?`, taskID)
			proc := f.processor("startup generation", &chunkEmbedding{value: 3}, time.Minute)
			queue := taskqueue.NewDurableTaskQueue(proc, postgresknowledge.NewChunkJobs(f.db), 2)
			defer queue.Shutdown()
			deadline := time.Now().Add(3 * time.Second)
			for jobState(t, f.db, taskID) != "completed" {
				if time.Now().After(deadline) {
					t.Fatal("startup did not recover pending intent")
				}
				time.Sleep(10 * time.Millisecond)
			}
			f.assertResult(t, port.ChunkDocumentTask{TaskID: taskID}, "startup generation")
		})
	}
}
