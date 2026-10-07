package work_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	"local/rag-project/internal/app/knowledge/port"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

type bootstrapChunkStorage struct{ chunkStorage }

func (s bootstrapChunkStorage) Delete(context.Context, string) error { return nil }

func TestDocumentChunkFormalHTTPAndBootstrapRecovery(t *testing.T) {
	f := newChunkFixture(t) // Validate isolated DB and apply the same migrations as cmd/server.
	connection, err := pgx.ParseConfig(os.Getenv("WORK_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	address := url.URL{Scheme: "postgresql", Host: fmt.Sprintf("%s:%d", connection.Host, connection.Port), Path: "/" + connection.Database}
	cfg.Spring.Datasource = config.DataSourceConfig{Url: address.String(), Username: connection.User, Password: connection.Password}
	cfg.Rag.Knowledge.Schedule.ScanDelayMs = 60000
	storage := bootstrapChunkStorage{chunkStorage: chunkStorage{text: "formal entrypoint generation"}}
	embedding := &chunkEmbedding{value: 1}
	openRuntime := func() *knowledgebootstrap.Runtime {
		t.Helper()
		r, err := knowledgebootstrap.NewRuntime(context.Background(), knowledgebootstrap.RuntimeOptions{Config: cfg, Storage: storage, AIRuntime: &infraai.Runtime{Embedding: embedding}})
		if err != nil {
			t.Fatal(err)
		}
		if !r.DocumentProcessService.FencedChunkProcessing() {
			t.Fatal("formal runtime did not enable chunk ownership")
		}
		return r
	}
	r := openRuntime()
	defer func() {
		if r != nil {
			r.Close()
		}
	}()
	router := gin.New()
	knowledgehttp.RegisterKnowledgeDocumentRoutes(router, r.DocumentService)
	request := httptest.NewRequest(http.MethodPost, "/knowledge-base/docs/"+f.id+"/chunk", nil)
	request.Header.Set("X-User-ID", f.user)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("chunk HTTP: %d %s", response.Code, response.Body.String())
	}
	var taskID string
	if err := f.db.Raw(`SELECT current_chunk_job_id FROM t_knowledge_document WHERE id=?`, f.id).Scan(&taskID).Error; err != nil || taskID == "" {
		t.Fatalf("HTTP returned without durable intent: %v", err)
	}
	await := func(taskID string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for jobState(t, f.db, taskID) != "completed" {
			if time.Now().After(deadline) {
				t.Fatalf("formal runtime did not complete job %s", taskID)
			}
			time.Sleep(10 * time.Millisecond)
		}
		f.assertResult(t, port.ChunkDocumentTask{TaskID: taskID}, "formal entrypoint generation")
	}
	await(taskID)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r = nil
	// The next accepted task has no surviving queue or local wake-up.
	task := f.admit(t)
	r = openRuntime()
	await(task.TaskID)
	if embedding.calls.Load() != 2 {
		t.Fatalf("formal admission or startup ran duplicate models: %d", embedding.calls.Load())
	}
}
