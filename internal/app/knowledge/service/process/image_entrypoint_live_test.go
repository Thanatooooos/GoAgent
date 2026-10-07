package process_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	postgres "local/rag-project/internal/adapter/repository/postgres"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
	process "local/rag-project/internal/app/knowledge/service/process"
	"local/rag-project/internal/framework/config"
)

type entrypointStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (s *entrypointStorage) Upload(_ context.Context, file port.FileUpload) (port.StoredFile, error) {
	data, err := io.ReadAll(file.Body)
	if err != nil {
		return port.StoredFile{}, err
	}
	s.mu.Lock()
	s.objects[file.Key] = data
	s.mu.Unlock()
	return port.StoredFile{Key: file.Key}, nil
}
func (s *entrypointStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	data, ok := s.objects[key]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("object not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *entrypointStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
	return nil
}

type entrypointEmbedding struct{}

type failingEntrypointEmbedding struct{ entrypointEmbedding }

func (failingEntrypointEmbedding) EmbedBatch([]string) ([][]float32, error) {
	return nil, fmt.Errorf("controlled embedding failure")
}
func (failingEntrypointEmbedding) EmbedBatchWithModel([]string, string) ([][]float32, error) {
	return nil, fmt.Errorf("controlled embedding failure")
}

type blockingEntrypointEmbedding struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingEntrypointEmbedding) Embed(string) ([]float32, error) {
	return entrypointVector(), nil
}
func (e *blockingEntrypointEmbedding) EmbedWithModel(string, string) ([]float32, error) {
	return entrypointVector(), nil
}
func (e *blockingEntrypointEmbedding) EmbedBatch(texts []string) ([][]float32, error) {
	return e.EmbedBatchWithModel(texts, "")
}
func (e *blockingEntrypointEmbedding) EmbedBatchWithModel(texts []string, _ string) ([][]float32, error) {
	e.once.Do(func() { close(e.started) })
	<-e.release
	return entrypointVectors(texts), nil
}
func (e *blockingEntrypointEmbedding) Dimension() int { return 4096 }

func (entrypointEmbedding) Embed(string) ([]float32, error) { return entrypointVector(), nil }
func (entrypointEmbedding) EmbedWithModel(string, string) ([]float32, error) {
	return entrypointVector(), nil
}
func (entrypointEmbedding) EmbedBatch(texts []string) ([][]float32, error) {
	return entrypointVectors(texts), nil
}
func (entrypointEmbedding) EmbedBatchWithModel(texts []string, _ string) ([][]float32, error) {
	return entrypointVectors(texts), nil
}
func (entrypointEmbedding) Dimension() int { return 4096 }
func entrypointVector() []float32          { value := make([]float32, 4096); value[0] = 1; return value }
func entrypointVectors(texts []string) [][]float32 {
	values := make([][]float32, len(texts))
	for i := range values {
		values[i] = entrypointVector()
	}
	return values
}

func TestImageEvidenceMainEntrypointLive(t *testing.T) {
	if os.Getenv("IMAGE_EVIDENCE_LIVE") != "1" || os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set IMAGE_EVIDENCE_LIVE=1 and DOCREADER_LIVE=1")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	db, err := postgres.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	if err := postgres.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "text.png"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "report.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	mixed, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "mixed.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	blank, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "blank.png"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kbID := fmt.Sprintf("imgkb%d", time.Now().UnixNano()%1000000000)
		docID := fmt.Sprintf("imgdoc%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_base (id, name, embedding_model, collection_name, created_by)
			VALUES (?, 'image entrypoint', 'test', ?, 'test')`, kbID, kbID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'text.png', 'source.png', 'png', 'test', 'pending')`, docID, kbID).Error; err != nil {
			return err
		}
		storage := &entrypointStorage{objects: map[string][]byte{"source.png": fixture, "report.pdf": report, "mixed.pdf": mixed, "blank.png": blank}}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一张写有收据文字的图片。"}}]}`))
		}))
		defer server.Close()
		imageService := &imageevidence.Service{
			DB: tx, Storage: storage,
			OCR:       coreparser.NewHTTPOCRClient(config.Get().Parser.OCR.URL, 30*time.Second),
			Vision:    vision.NewClient(server.URL, "test", time.Second, 128),
			Embedding: entrypointEmbedding{},
		}
		enrichment := false
		processor := process.NewDocumentProcessService(process.DocumentProcessServiceOptions{
			BaseRepo:     postgresknowledge.NewKnowledgeBaseRepository(tx),
			DocumentRepo: postgresknowledge.NewKnowledgeDocumentRepository(tx, nil),
			ChunkRepo:    postgresknowledge.NewKnowledgeChunkRepository(tx),
			ChunkLogRepo: postgresknowledge.NewKnowledgeDocumentChunkLogRepository(tx),
			Storage:      storage, VectorStore: pgvectorstore.NewVectorStore(tx),
			Transaction: postgresknowledge.NewDocumentProcessTransaction(tx),
			Embedding:   entrypointEmbedding{}, EnrichmentEnabled: &enrichment,
			ImageEvidence: imageService,
		})
		if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: docID, TriggeredBy: "test"}); err != nil {
			return err
		}
		var status string
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "running" {
			return fmt.Errorf("image-only document with pending evidence should be running, got %s", status)
		}
		for n := 0; n < 2; n++ {
			processed, err := imageService.ProcessOne(ctx, "test-worker")
			if err != nil || !processed {
				return fmt.Errorf("image task %d: processed=%t err=%v", n, processed, err)
			}
		}
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "success" {
			return fmt.Errorf("image-only document did not recover: %s", status)
		}
		var vectors int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, docID).Scan(&vectors).Error; err != nil {
			return err
		}
		if vectors != 1 {
			return fmt.Errorf("expected exactly one image evidence vector, got %d", vectors)
		}
		reportID := fmt.Sprintf("rptdoc%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'report.pdf', 'report.pdf', 'pdf', 'test', 'pending')`, reportID, kbID).Error; err != nil {
			return err
		}
		if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: reportID, TriggeredBy: "test"}); err != nil {
			return err
		}
		var textChunkID string
		if err := tx.Raw(`SELECT id FROM t_knowledge_chunk WHERE doc_id = ? LIMIT 1`, reportID).Scan(&textChunkID).Error; err != nil {
			return err
		}
		if len(textChunkID) <= 20 || len(textChunkID) > 64 {
			return fmt.Errorf("versioned text chunk was not persisted: %q", textChunkID)
		}
		mixedID := fmt.Sprintf("mixdoc%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'mixed.pdf', 'mixed.pdf', 'pdf', 'test', 'pending')`, mixedID, kbID).Error; err != nil {
			return err
		}
		if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: mixedID, TriggeredBy: "test"}); err != nil {
			return err
		}
		var textVectors, imageTasks int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'child'`, mixedID).Scan(&textVectors).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_image_task WHERE doc_id = ?`, mixedID).Scan(&imageTasks).Error; err != nil {
			return err
		}
		if textVectors == 0 || imageTasks == 0 {
			return fmt.Errorf("mixed PDF missing text vectors or image tasks: text=%d tasks=%d", textVectors, imageTasks)
		}
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, mixedID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "running" {
			return fmt.Errorf("mixed PDF with pending image tasks should be running, got %s", status)
		}
		for n := int64(0); n < imageTasks; n++ {
			processed, err := imageService.ProcessOne(ctx, "test-worker")
			if err != nil || !processed {
				return fmt.Errorf("mixed image task %d: processed=%t err=%v", n, processed, err)
			}
		}
		var imageVectors int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, mixedID).Scan(&imageVectors).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, mixedID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "success" || imageVectors == 0 {
			return fmt.Errorf("mixed PDF did not publish both evidence types: status=%s text=%d image=%d", status, textVectors, imageVectors)
		}
		blankServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"NO_CONTENT"}}]}`))
		}))
		defer blankServer.Close()
		imageService.Vision = vision.NewClient(blankServer.URL, "test", time.Second, 128)
		blankID := fmt.Sprintf("blank%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'blank.png', 'blank.png', 'png', 'test', 'pending')`, blankID, kbID).Error; err != nil {
			return err
		}
		if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: blankID, TriggeredBy: "test"}); err != nil {
			return err
		}
		for n := 0; n < 2; n++ {
			processed, err := imageService.ProcessOne(ctx, "test-worker")
			if err != nil || !processed {
				return fmt.Errorf("blank image task %d: processed=%t err=%v", n, processed, err)
			}
		}
		var blankVectors int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ?`, blankID).Scan(&blankVectors).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, blankID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "failed" || blankVectors != 0 {
			return fmt.Errorf("blank image should have no evidence and fail: status=%s vectors=%d", status, blankVectors)
		}
		imageService.Vision = vision.NewClient(server.URL, "test", time.Second, 128)
		var oldRevision string
		if err := tx.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, reportID).Scan(&oldRevision).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE t_knowledge_document SET doc_name = 'mixed.pdf', file_url = 'mixed.pdf'
			WHERE id = ?`, reportID).Error; err != nil {
			return err
		}
		blocking := &blockingEntrypointEmbedding{started: make(chan struct{}), release: make(chan struct{})}
		defer func() {
			blocking.once.Do(func() { close(blocking.started) })
			select {
			case <-blocking.release:
			default:
				close(blocking.release)
			}
		}()
		stagedProcessor := process.NewDocumentProcessService(process.DocumentProcessServiceOptions{
			BaseRepo:     postgresknowledge.NewKnowledgeBaseRepository(tx),
			DocumentRepo: postgresknowledge.NewKnowledgeDocumentRepository(tx, nil),
			ChunkRepo:    postgresknowledge.NewKnowledgeChunkRepository(tx),
			ChunkLogRepo: postgresknowledge.NewKnowledgeDocumentChunkLogRepository(tx),
			Storage:      storage, VectorStore: pgvectorstore.NewVectorStore(tx),
			Transaction: postgresknowledge.NewDocumentProcessTransaction(tx),
			Embedding:   blocking, EnrichmentEnabled: &enrichment, ImageEvidence: imageService,
		})
		processedResult := make(chan error, 1)
		go func() {
			processedResult <- stagedProcessor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: reportID, TriggeredBy: "test"})
		}()
		select {
		case <-blocking.started:
		case <-time.After(15 * time.Second):
			return fmt.Errorf("mixed PDF did not reach text embedding")
		}
		var currentRevision, stagedRevision string
		if err := tx.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, reportID).Scan(&currentRevision).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT id FROM t_knowledge_document_revision WHERE doc_id = ? AND status = 'building'`, reportID).Scan(&stagedRevision).Error; err != nil {
			return err
		}
		if currentRevision != oldRevision || stagedRevision == "" {
			return fmt.Errorf("old revision was not retained while embedding blocked: old=%s active=%s staged=%s", oldRevision, currentRevision, stagedRevision)
		}
		stagedTasks := 0
		for n := 0; n < 8; n++ {
			processed, err := imageService.ProcessOne(ctx, "test-worker")
			if err != nil {
				return err
			}
			if !processed {
				break
			}
			stagedTasks++
		}
		if stagedTasks == 0 {
			return fmt.Errorf("staged image tasks were not processed")
		}
		var stagedEvidence, prematureImageVectors int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_image_evidence WHERE revision_id = ?`, stagedRevision).Scan(&stagedEvidence).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, reportID).Scan(&prematureImageVectors).Error; err != nil {
			return err
		}
		if stagedEvidence == 0 || prematureImageVectors != 0 {
			return fmt.Errorf("staged OCR was not isolated: evidence=%d vectors=%d", stagedEvidence, prematureImageVectors)
		}
		close(blocking.release)
		select {
		case err := <-processedResult:
			if err != nil {
				return err
			}
		case <-time.After(15 * time.Second):
			return fmt.Errorf("mixed PDF publication did not finish")
		}
		for n := 0; n < 8; n++ {
			processed, err := imageService.ProcessOne(ctx, "test-worker")
			if err != nil {
				return err
			}
			if !processed {
				break
			}
		}
		if err := tx.Raw(`SELECT active_revision_id, status FROM t_knowledge_document WHERE id = ?`, reportID).Row().Scan(&currentRevision, &status); err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, reportID).Scan(&imageVectors).Error; err != nil {
			return err
		}
		if currentRevision != stagedRevision || status != "success" || imageVectors == 0 {
			return fmt.Errorf("staged revision was not published: revision=%s status=%s vectors=%d", currentRevision, status, imageVectors)
		}
		failingProcessor := process.NewDocumentProcessService(process.DocumentProcessServiceOptions{
			BaseRepo:     postgresknowledge.NewKnowledgeBaseRepository(tx),
			DocumentRepo: postgresknowledge.NewKnowledgeDocumentRepository(tx, nil),
			ChunkRepo:    postgresknowledge.NewKnowledgeChunkRepository(tx),
			ChunkLogRepo: postgresknowledge.NewKnowledgeDocumentChunkLogRepository(tx),
			Storage:      storage, VectorStore: pgvectorstore.NewVectorStore(tx),
			Transaction: postgresknowledge.NewDocumentProcessTransaction(tx),
			Embedding:   failingEntrypointEmbedding{}, EnrichmentEnabled: &enrichment,
			ImageEvidence: imageService,
		})
		if err := failingProcessor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: reportID, TriggeredBy: "test"}); err == nil {
			return fmt.Errorf("controlled text embedding failure was ignored")
		}
		if err := tx.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, reportID).Scan(&currentRevision).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, reportID).Scan(&imageVectors).Error; err != nil {
			return err
		}
		if currentRevision != stagedRevision || imageVectors == 0 {
			return fmt.Errorf("failed embedding damaged the published revision: revision=%s vectors=%d", currentRevision, imageVectors)
		}
		return fmt.Errorf("ROLLBACK_TEST_FIXTURE")
	}); err != nil && err.Error() != "ROLLBACK_TEST_FIXTURE" {
		t.Fatal(err)
	}
}
