package imageevidence

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/config"
)

type liveStorage struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (s *liveStorage) Upload(_ context.Context, file port.FileUpload) (port.StoredFile, error) {
	data, err := io.ReadAll(file.Body)
	if err != nil {
		return port.StoredFile{}, err
	}
	s.mu.Lock()
	s.files[file.Key] = data
	s.mu.Unlock()
	return port.StoredFile{Key: file.Key}, nil
}
func (s *liveStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	data, ok := s.files[key]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("missing object")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *liveStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.files, key)
	s.mu.Unlock()
	return nil
}

type liveOCR struct{}

func (liveOCR) Recognize(context.Context, []byte) (parser.OCRResult, error) {
	return parser.OCRResult{Status: "success", Text: "invoice total 42"}, nil
}

type liveEmbedding struct{}

func liveVector() []float32 {
	vector := make([]float32, 4096)
	vector[0] = 1
	return vector
}

func (liveEmbedding) Embed(string) ([]float32, error) { return liveVector(), nil }
func (liveEmbedding) EmbedWithModel(string, string) ([]float32, error) {
	return liveVector(), nil
}
func (liveEmbedding) EmbedBatch([]string) ([][]float32, error)                  { return nil, nil }
func (liveEmbedding) EmbedBatchWithModel([]string, string) ([][]float32, error) { return nil, nil }
func (liveEmbedding) Dimension() int                                            { return 4096 }

func TestImageEvidencePostgresLive(t *testing.T) {
	if os.Getenv("IMAGE_EVIDENCE_LIVE") != "1" {
		t.Skip("set IMAGE_EVIDENCE_LIVE=1")
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
	ctx := context.Background()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kbID := fmt.Sprintf("imgkb%d", time.Now().UnixNano()%1000000000)
		docID := fmt.Sprintf("imgdoc%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_base
			(id, name, embedding_model, collection_name, created_by)
			VALUES (?, 'Image evidence live', 'test', ?, 'test')`, kbID, kbID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'image.png', 'test', 'png', 'test', 'running')`, docID, kbID).Error; err != nil {
			return err
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一张白色方形图片。"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		}))
		defer server.Close()
		storage := &liveStorage{files: map[string][]byte{}}
		svc := &Service{DB: tx, Storage: storage, OCR: liveOCR{}, Vision: vision.NewClient(server.URL, "test", time.Second, 128), Embedding: liveEmbedding{}}
		pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/lZkAAAAASUVORK5CYII=")
		if err != nil {
			return err
		}
		registered, err := svc.Register(ctx, RegisterInput{DocumentID: docID, KnowledgeBaseID: kbID,
			Source: pngBytes, Parsed: parser.ParseResult{ImageInventoryConfirmed: true,
				Metadata: map[string]any{"parser_type": "docreader"},
				Images:   []parser.ImageOccurrence{{Index: 0, OriginalRef: "image.png", MIMEType: "image/png", Data: pngBytes, AdjacentText: "nearby source paragraph"}}}})
		if err != nil {
			return err
		}
		if registered.ImageCount != 1 || registered.Failures != 0 {
			return fmt.Errorf("unexpected registration: %+v", registered)
		}
		var occurrenceID string
		if err := tx.Raw(`SELECT id FROM t_knowledge_image_occurrence
			WHERE revision_id = ?`, registered.RevisionID).Scan(&occurrenceID).Error; err != nil {
			return err
		}
		if raw, _, err := svc.OpenOccurrenceOriginal(ctx, occurrenceID); err != nil || !bytes.Equal(raw, pngBytes) {
			return fmt.Errorf("original image is unavailable before evidence: %w", err)
		}
		if err := tx.Exec(`UPDATE t_knowledge_image_task
			SET status = 'error', error_code = 'test_failure'
			WHERE revision_id = ? AND operation = 'ocr'`, registered.RevisionID).Error; err != nil {
			return err
		}
		if queued, err := svc.RetryOccurrence(ctx, occurrenceID); err != nil || queued != 1 {
			return fmt.Errorf("retry without evidence failed: queued=%d err=%w", queued, err)
		}
		if err := tx.Exec(`UPDATE t_knowledge_image_task
			SET next_run_at = now() + interval '1 hour'
			WHERE revision_id = ? AND operation = 'caption'`, registered.RevisionID).Error; err != nil {
			return err
		}
		for n := 0; n < 2; n++ {
			task, err := svc.claim(ctx, "test-worker")
			if err != nil || task.ID == "" {
				return fmt.Errorf("claim %d: %w", n, err)
			}
			if err := svc.process(ctx, "test-worker", task); err != nil {
				return err
			}
			if n == 0 {
				var ocrVectors int64
				if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector
					WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, docID).Scan(&ocrVectors).Error; err != nil {
					return err
				}
				if task.Operation != "ocr" || ocrVectors != 1 {
					return fmt.Errorf("OCR was not published before caption: task=%s vectors=%d", task.Operation, ocrVectors)
				}
				var pendingStatus string
				if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&pendingStatus).Error; err != nil {
					return err
				}
				if pendingStatus != "running" {
					return fmt.Errorf("document with a pending caption should be running, got %s", pendingStatus)
				}
				if err := tx.Exec(`UPDATE t_knowledge_image_task
					SET status = 'error', error_code = 'vision_model_unavailable'
					WHERE revision_id = ? AND operation = 'caption'`, registered.RevisionID).Error; err != nil {
					return err
				}
				if err := refreshDocumentStatus(tx, docID, registered.RevisionID); err != nil {
					return err
				}
				var partialStatus string
				if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&partialStatus).Error; err != nil {
					return err
				}
				if partialStatus != "partial" {
					return fmt.Errorf("OCR evidence with failed caption should be partial, got %s", partialStatus)
				}
				if queued, err := svc.RetryOccurrence(ctx, occurrenceID); err != nil || queued != 1 {
					return fmt.Errorf("caption retry failed: queued=%d err=%w", queued, err)
				}
			}
		}
		var evidenceID, status string
		if err := tx.Raw(`SELECT i.active_evidence_id FROM t_knowledge_image_occurrence i WHERE i.revision_id = ?`, registered.RevisionID).Scan(&evidenceID).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&status).Error; err != nil {
			return err
		}
		if status != "success" {
			return fmt.Errorf("unexpected document status %q", status)
		}
		if detail, err := svc.Get(ctx, evidenceID); err != nil || detail.CaptionStatus != "described" || detail.OCRStatus != "success" || detail.AdjacentText != "nearby source paragraph" || detail.CaptionInputTokens != 10 || detail.CaptionOutputTokens != 5 {
			return fmt.Errorf("unexpected detail: %+v %v", detail, err)
		}
		var vectors int64
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE chunk_id = ?`, evidenceID).Scan(&vectors).Error; err != nil {
			return err
		}
		if vectors != 1 {
			return fmt.Errorf("expected one image vector, got %d", vectors)
		}
		if raw, mimeType, err := svc.OpenOriginal(ctx, evidenceID); err != nil || mimeType != "image/png" || !bytes.Equal(raw, pngBytes) {
			return fmt.Errorf("original image lookup failed: mime=%q err=%v", mimeType, err)
		}
		newRevision, err := svc.Register(ctx, RegisterInput{DocumentID: docID, KnowledgeBaseID: kbID,
			Source: []byte("new revision"), Parsed: parser.ParseResult{Metadata: map[string]any{"parser_type": "docreader"}, ImageInventoryConfirmed: true}})
		if err != nil {
			return err
		}
		if _, err := svc.Register(ctx, RegisterInput{DocumentID: docID, KnowledgeBaseID: kbID,
			ExpectedRevisionID: registered.RevisionID, Source: []byte("stale revision"),
			Parsed: parser.ParseResult{Metadata: map[string]any{"parser_type": "docreader"}, ImageInventoryConfirmed: true}}); err == nil {
			return fmt.Errorf("stale publication unexpectedly replaced the active revision")
		}
		var activeRevision string
		if err := tx.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, docID).Scan(&activeRevision).Error; err != nil {
			return err
		}
		if activeRevision != newRevision.RevisionID {
			return fmt.Errorf("stale publication changed active revision: %s", activeRevision)
		}
		if _, err := svc.Get(ctx, evidenceID); err != nil {
			return fmt.Errorf("historical evidence disappeared: %w", err)
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE chunk_id = ?`, evidenceID).Scan(&vectors).Error; err != nil {
			return err
		}
		if vectors != 0 {
			return fmt.Errorf("stale image vector survived revision switch")
		}
		if err := tx.Exec(`UPDATE t_knowledge_document SET deleted = 1 WHERE id = ?`, docID).Error; err != nil {
			return err
		}
		if _, err := svc.Get(ctx, evidenceID); err == nil {
			return fmt.Errorf("deleted document image remained readable")
		}
		if err := svc.cleanupDeleted(ctx); err != nil {
			return err
		}
		if len(storage.files) != 0 {
			return fmt.Errorf("image objects not cleaned after document deletion")
		}
		return fmt.Errorf("ROLLBACK_TEST_FIXTURE")
	}); err != nil && err.Error() != "ROLLBACK_TEST_FIXTURE" {
		t.Fatal(err)
	}
}
