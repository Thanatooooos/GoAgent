package work_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
)

func waitChunkEmbedding(t *testing.T, embedding *chunkEmbedding) {
	t.Helper()
	select {
	case <-embedding.started:
	case <-time.After(3 * time.Second):
		t.Fatal("embedding did not start")
	}
}

func TestDocumentChunkHeartbeatAndDuplicateDelivery(t *testing.T) {
	f := newChunkFixture(t)
	task := f.admit(t)
	embedding := &chunkEmbedding{value: 1, started: make(chan struct{}), release: make(chan struct{})}
	proc := f.processor("heartbeat generation", embedding, 300*time.Millisecond)
	done := make(chan error, 1)
	go func() {
		done <- proc.ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: task.TaskID})
	}()
	waitChunkEmbedding(t, embedding)
	if err := proc.ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: task.TaskID}); err != nil {
		t.Fatal(err)
	}
	// Longer than two original lease periods, while the model ignores cancellation.
	time.Sleep(850 * time.Millisecond)
	if _, err := f.jobs.RecoverExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobState(t, f.db, task.TaskID) != "running" || embedding.calls.Load() != 1 {
		t.Fatal("heartbeat did not preserve exclusive ownership")
	}
	// An old image worker must not move the status out of running underneath it.
	if err := f.db.Exec(`UPDATE t_knowledge_document SET active_revision_id='old-image-revision' WHERE id=?`, f.id).Error; err != nil {
		t.Fatal(err)
	}
	images := &imageevidence.Service{DB: f.db}
	if err := images.RefreshStatus(context.Background(), f.id, "old-image-revision"); err != nil {
		t.Fatal(err)
	}
	doc, err := postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil).GetByID(context.Background(), f.id)
	if err != nil || doc.Status != "running" {
		t.Fatalf("image status revoked live text ownership: %s %v", doc.Status, err)
	}
	close(embedding.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f.assertResult(t, task, "heartbeat generation")
}

func TestDocumentChunkLeaseExpiryDuringPublicationRollsBack(t *testing.T) {
	f := newChunkFixture(t)
	baseline := f.admit(t)
	if err := f.processor("previous generation", &chunkEmbedding{value: 1}, time.Minute).ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: baseline.TaskID}); err != nil {
		t.Fatal(err)
	}
	task := f.admit(t)
	callback := "chunk/commit-expiry/" + task.TaskID
	delayed := false
	f.db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "t_knowledge_document_chunk_log" {
			delayed = true
			time.Sleep(1100 * time.Millisecond)
		}
	})
	err := f.processor("expired generation", &chunkEmbedding{value: 2}, 800*time.Millisecond).ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: task.TaskID})
	f.db.Callback().Update().Remove(callback)
	if err == nil || !delayed {
		t.Fatalf("lease expired during terminal write but commit succeeded: %v delayed=%v", err, delayed)
	}
	if _, err := f.jobs.RecoverExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobState(t, f.db, task.TaskID) != "interrupted" {
		t.Fatal("expired commit not interrupted")
	}
	var contents []string
	if err := f.db.Raw(`SELECT content FROM t_knowledge_chunk_vector WHERE doc_id=?`, f.id).Scan(&contents).Error; err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		t.Fatal("rollback deleted prior projection")
	}
	for _, content := range contents {
		if !strings.Contains(content, "previous generation") {
			t.Fatalf("expired projection leaked: %s", content)
		}
	}
	log, err := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(f.db).GetByID(context.Background(), baseline.TaskID)
	if err != nil || log.Status != "success" {
		t.Fatal("expiry modified the previous execution log")
	}
}

func TestDocumentChunkConfigurationAndResourceRevocation(t *testing.T) {
	for _, change := range []struct{ name, sql string }{
		{"configuration", `UPDATE t_knowledge_document SET chunk_config='{"chunkSize":10}' WHERE id=?`},
		{"file", `UPDATE t_knowledge_document SET file_url='replaced.txt' WHERE id=?`},
		{"disabled", `UPDATE t_knowledge_document SET enabled=0 WHERE id=?`},
		{"deleted", `UPDATE t_knowledge_document SET deleted=1 WHERE id=?`},
	} {
		t.Run(change.name, func(t *testing.T) {
			f := newChunkFixture(t)
			task := f.admit(t)
			embedding := &chunkEmbedding{value: 1, started: make(chan struct{}), release: make(chan struct{})}
			done := make(chan error, 1)
			go func() {
				done <- f.processor("revoked generation", embedding, time.Minute).ExecuteChunk(context.Background(), service.ExecuteChunkInput{TaskID: task.TaskID})
			}()
			waitChunkEmbedding(t, embedding)
			if err := f.db.Exec(change.sql, f.id).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.jobs.RecoverExpired(context.Background()); err != nil {
				t.Fatal(err)
			}
			close(embedding.release)
			if err := <-done; err == nil {
				t.Fatal("revoked execution published")
			}
			state := jobState(t, f.db, task.TaskID)
			if state != "failed" && state != "interrupted" {
				t.Fatalf("unsettled execution: %s", state)
			}
			var count int64
			if err := f.db.Table("t_knowledge_chunk_vector").Where("doc_id=?", f.id).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("revoked projection visible: %d %v", count, err)
			}
		})
	}
}

func TestDocumentChunkRemoteRefreshPublicationAndOccupation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "atomic_metadata", true: "failure_keeps_source"}[fail], func(t *testing.T) {
			f := newChunkFixture(t)
			repo := postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil)
			doc, occupied, err := repo.ClaimRemoteRefresh(context.Background(), f.id)
			if err != nil || !occupied {
				t.Fatalf("occupation: %v %v", occupied, err)
			}
			oldURL := doc.FileURL
			doc.FileURL, doc.Name, doc.FileSize = "remote-new.txt", "remote-new.txt", 456
			embedding := &chunkEmbedding{value: 1, started: make(chan struct{}), release: make(chan struct{})}
			if fail {
				embedding.failure = errors.New("remote embedding failed")
			}
			done := make(chan error, 1)
			go func() {
				done <- f.processor("remote generation", embedding, time.Minute).ProcessRefreshedDocument(context.Background(), doc)
			}()
			waitChunkEmbedding(t, embedding)
			current, err := repo.GetByID(context.Background(), f.id)
			if err != nil || current.FileURL != oldURL {
				t.Fatal("metadata switched before publication")
			}
			if discard, err := repo.CanDiscardRefreshedFile(context.Background(), f.id, doc.FileURL); err != nil || discard {
				t.Fatal("cleanup would delete an in-flight refresh source")
			}
			close(embedding.release)
			err = <-done
			if (err != nil) != fail {
				t.Fatalf("refresh outcome: %v", err)
			}
			current, err = repo.GetByID(context.Background(), f.id)
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if current.FileURL != oldURL || current.Status != "failed" {
					t.Fatal("failure changed source metadata")
				}
			} else if current.FileURL != doc.FileURL || current.Name != doc.Name || current.FileSize != doc.FileSize || current.Status != "success" {
				t.Fatalf("metadata not published atomically: %+v", current)
			}
			if discard, err := repo.CanDiscardRefreshedFile(context.Background(), f.id, doc.FileURL); err != nil || discard != fail {
				t.Fatalf("cleanup reference guard: %v %v", discard, err)
			}
		})
	}
	f := newChunkFixture(t)
	repo := postgresknowledge.NewKnowledgeDocumentRepository(f.db, nil)
	old, _, err := repo.ClaimRemoteRefresh(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	helper := schedule.NewDocumentStatusHelper(repo)
	if err := helper.MarkFailedRefreshIfRunning(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	current, occupied, err := repo.ClaimRemoteRefresh(context.Background(), f.id)
	if err != nil || !occupied {
		t.Fatalf("new occupation: %v", err)
	}
	old.FileURL = "stale-remote.txt"
	_, err = f.jobs.Admit(context.Background(), port.ChunkDocumentTask{TaskID: publicationID(t), DocumentID: f.id, TriggeredBy: f.user}, &old)
	if !errors.Is(err, port.ErrChunkLeaseLost) {
		t.Fatalf("stale refresh adopted new occupation: %v", err)
	}
	if err := helper.MarkFailedRefreshIfRunning(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	actual, err := repo.GetByID(context.Background(), f.id)
	if err != nil || actual.Status != domain.KnowledgeDocumentStatusRunning || !actual.UpdatedAt.Equal(current.UpdatedAt) {
		t.Fatal("late cleanup revoked a newer remote occupation")
	}
}
