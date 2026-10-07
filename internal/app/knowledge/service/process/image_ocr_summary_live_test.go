package process_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgres "local/rag-project/internal/adapter/repository/postgres"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
	process "local/rag-project/internal/app/knowledge/service/process"
	"local/rag-project/internal/framework/config"
)

type summaryChatSpy struct{ prompts []string }

func (s *summaryChatSpy) Chat(prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	return "SUMMARY_FROM_BODY_AND_OCR", nil
}

func TestOCRSummaryLive(t *testing.T) {
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
	mixed, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "mixed.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	kbID := fmt.Sprintf("sumkb%d", time.Now().UnixNano()%1000000000)
	docID := fmt.Sprintf("sumdoc%d", time.Now().UnixNano()%1000000000)
	if err := db.Exec(`INSERT INTO t_knowledge_base (id, name, embedding_model, collection_name, created_by)
		VALUES (?, 'ocr summary', 'test', ?, 'test')`, kbID, kbID).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM t_knowledge_base WHERE id = ?`, kbID)
	if err := db.Exec(`INSERT INTO t_knowledge_document
		(id, kb_id, doc_name, file_url, file_type, created_by, status)
		VALUES (?, ?, 'mixed.pdf', 'mixed.pdf', 'pdf', 'test', 'pending')`, docID, kbID).Error; err != nil {
		t.Fatal(err)
	}
	storage := &entrypointStorage{objects: map[string][]byte{"mixed.pdf": mixed}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"CAPTION_SENTINEL_DO_NOT_SUMMARIZE"}}]}`))
	}))
	defer server.Close()
	spy := &summaryChatSpy{}
	imageService := &imageevidence.Service{
		DB: db, Storage: storage,
		OCR:       coreparser.NewHTTPOCRClient(config.Get().Parser.OCR.URL, 30*time.Second),
		Vision:    vision.NewClient(server.URL, "test", time.Second, 128),
		Embedding: entrypointEmbedding{}, SummaryChat: spy, SummaryEnabled: true,
	}
	enrichment := false
	processor := process.NewDocumentProcessService(process.DocumentProcessServiceOptions{
		BaseRepo:     postgresknowledge.NewKnowledgeBaseRepository(db),
		DocumentRepo: postgresknowledge.NewKnowledgeDocumentRepository(db, nil),
		ChunkRepo:    postgresknowledge.NewKnowledgeChunkRepository(db),
		ChunkLogRepo: postgresknowledge.NewKnowledgeDocumentChunkLogRepository(db),
		Storage:      storage, VectorStore: pgvectorstore.NewVectorStore(db),
		Transaction: postgresknowledge.NewDocumentProcessTransaction(db),
		Embedding:   entrypointEmbedding{}, EnrichmentEnabled: &enrichment,
		ImageEvidence: imageService,
	})
	if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: docID, TriggeredBy: "test"}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 20; n++ {
		processed, err := imageService.ProcessOne(ctx, "summary-test")
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	var doc struct{ Summary, SummaryStatus, Status string }
	if err := db.Raw(`SELECT summary, summary_status, status FROM t_knowledge_document WHERE id = ?`, docID).Scan(&doc).Error; err != nil {
		t.Fatal(err)
	}
	if doc.Summary != "SUMMARY_FROM_BODY_AND_OCR" || doc.SummaryStatus != "success" || doc.Status != "success" {
		t.Fatalf("summary or document status mismatch: %+v", doc)
	}
	var ocrText, bodyText, vectorSummary string
	if err := db.Raw(`SELECT COALESCE(e.ocr_text, '') FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.doc_id = ? AND e.ocr_status = 'success' LIMIT 1`, docID).Scan(&ocrText).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT content FROM t_knowledge_chunk WHERE doc_id = ? LIMIT 1`, docID).Scan(&bodyText).Error; err != nil {
		t.Fatal(err)
	}
	if len(spy.prompts) != 1 || ocrText == "" || bodyText == "" ||
		!strings.Contains(spy.prompts[0], ocrText) || !strings.Contains(spy.prompts[0], strings.TrimSpace(bodyText)) ||
		strings.Contains(spy.prompts[0], "CAPTION_SENTINEL_DO_NOT_SUMMARIZE") {
		t.Fatalf("summary inputs incorrect: calls=%d ocr=%q body=%q prompt=%q", len(spy.prompts), ocrText, bodyText, strings.Join(spy.prompts, "\n"))
	}
	if err := db.Raw(`SELECT metadata->>'document_summary' FROM t_knowledge_chunk_vector
		WHERE doc_id = ? AND metadata->>'record_type' = 'child' LIMIT 1`, docID).Scan(&vectorSummary).Error; err != nil {
		t.Fatal(err)
	}
	if vectorSummary != doc.Summary {
		t.Fatalf("text vector summary mismatch: %q", vectorSummary)
	}
	var questionCount int64
	if err := db.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ?
		AND metadata->>'record_type' = 'question'`, docID).Scan(&questionCount).Error; err != nil {
		t.Fatal(err)
	}
	if questionCount != 0 {
		t.Fatalf("OCR summary created %d question vectors", questionCount)
	}
	var revisionID string
	if err := db.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, docID).Scan(&revisionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := imageService.RefreshStatus(ctx, docID, revisionID); err != nil {
		t.Fatal(err)
	}
	var summaryTasks int64
	if err := db.Raw(`SELECT count(*) FROM t_knowledge_image_task
		WHERE revision_id = ? AND operation = 'summary'`, revisionID).Scan(&summaryTasks).Error; err != nil {
		t.Fatal(err)
	}
	if summaryTasks != 1 {
		t.Fatalf("unchanged OCR queued %d summary tasks", summaryTasks)
	}
	image, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "text.png"))
	if err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	storage.objects["text.png"] = image
	storage.mu.Unlock()
	imageDocID := fmt.Sprintf("sumimg%d", time.Now().UnixNano()%1000000000)
	if err := db.Exec(`INSERT INTO t_knowledge_document
		(id, kb_id, doc_name, file_url, file_type, created_by, status)
		VALUES (?, ?, 'text.png', 'text.png', 'png', 'test', 'pending')`, imageDocID, kbID).Error; err != nil {
		t.Fatal(err)
	}
	if err := processor.ExecuteChunk(ctx, process.ExecuteChunkInput{DocumentID: imageDocID, TriggeredBy: "test"}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 10; n++ {
		processed, err := imageService.ProcessOne(ctx, "summary-test")
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	var imageSummary string
	if err := db.Raw(`SELECT summary FROM t_knowledge_document WHERE id = ?`, imageDocID).Scan(&imageSummary).Error; err != nil {
		t.Fatal(err)
	}
	if imageSummary != "SUMMARY_FROM_BODY_AND_OCR" || len(spy.prompts) != 2 ||
		!strings.Contains(spy.prompts[1], "IMAGE RECEIPT 42") ||
		strings.Contains(spy.prompts[1], "CAPTION_SENTINEL_DO_NOT_SUMMARIZE") {
		t.Fatalf("image-only OCR summary mismatch: summary=%q prompts=%v", imageSummary, spy.prompts)
	}
}
