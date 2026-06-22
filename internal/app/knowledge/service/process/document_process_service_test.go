package process

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

type processBaseRepositoryStub struct {
	base domain.KnowledgeBase
}

func (s processBaseRepositoryStub) Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	return domain.KnowledgeBase{}, nil
}

func (s processBaseRepositoryStub) Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	return domain.KnowledgeBase{}, nil
}

func (s processBaseRepositoryStub) UpdateWhere(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error) {
	return 0, nil
}

func (s processBaseRepositoryStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s processBaseRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error) {
	return s.base, nil
}

func (s processBaseRepositoryStub) GetByName(ctx context.Context, name string) (int, error) {
	return 0, nil
}

func (s processBaseRepositoryStub) Count(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error) {
	return 0, nil
}

func (s processBaseRepositoryStub) List(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error) {
	return nil, nil
}

type processDocumentRepositoryStub struct {
	document      domain.KnowledgeDocument
	updateFields  []port.UpdateAssignments
	updateFieldsE error
}

func (s *processDocumentRepositoryStub) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s *processDocumentRepositoryStub) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s *processDocumentRepositoryStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	return 0, nil
}

func (s *processDocumentRepositoryStub) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	s.updateFields = append(s.updateFields, set)
	return 1, s.updateFieldsE
}

func (s *processDocumentRepositoryStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s *processDocumentRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	return s.document, nil
}

func (s *processDocumentRepositoryStub) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s *processDocumentRepositoryStub) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s *processDocumentRepositoryStub) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	return nil, nil
}

type processChunkRepositoryStub struct {
	created []domain.KnowledgeChunk
}

func (s *processChunkRepositoryStub) Create(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	return chunk, nil
}

func (s *processChunkRepositoryStub) CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error {
	s.created = append([]domain.KnowledgeChunk{}, chunks...)
	return nil
}

func (s *processChunkRepositoryStub) Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	return domain.KnowledgeChunk{}, nil
}

func (s *processChunkRepositoryStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (s *processChunkRepositoryStub) Delete(ctx context.Context, id string) error {
	return nil
}

func (s *processChunkRepositoryStub) UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s *processChunkRepositoryStub) UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s *processChunkRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error) {
	return domain.KnowledgeChunk{}, nil
}

func (s *processChunkRepositoryStub) CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error) {
	return 0, nil
}

func (s *processChunkRepositoryStub) List(ctx context.Context, filter port.KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error) {
	return nil, nil
}

type processChunkLogRepositoryStub struct {
	created []domain.KnowledgeDocumentChunkLog
	updated []domain.KnowledgeDocumentChunkLog
}

func (s *processChunkLogRepositoryStub) Create(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	s.created = append(s.created, log)
	return log, nil
}

func (s *processChunkLogRepositoryStub) Update(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error) {
	s.updated = append(s.updated, log)
	return log, nil
}

func (s *processChunkLogRepositoryStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentChunkLog, error) {
	return domain.KnowledgeDocumentChunkLog{}, nil
}

func (s *processChunkLogRepositoryStub) GetByTaskID(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
	return domain.KnowledgeDocumentChunkLog{}, nil
}

func (s *processChunkLogRepositoryStub) ListByDocumentID(ctx context.Context, documentID string, options port.ListOptions) ([]domain.KnowledgeDocumentChunkLog, error) {
	return nil, nil
}

type processStorageStub struct {
	body string
}

func (s processStorageStub) Upload(ctx context.Context, file port.FileUpload) (port.StoredFile, error) {
	return port.StoredFile{}, nil
}

func (s processStorageStub) Delete(ctx context.Context, key string) error {
	return nil
}

func (s processStorageStub) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.body)), nil
}

type processVectorStoreStub struct {
	upserted    []port.ChunkVector
	upsertError error
}

func (s *processVectorStoreStub) UpsertDocumentChunks(ctx context.Context, chunks []port.ChunkVector) error {
	s.upserted = append([]port.ChunkVector{}, chunks...)
	return s.upsertError
}

func (s *processVectorStoreStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (s *processVectorStoreStub) DeleteChunk(ctx context.Context, chunkID string) error {
	return nil
}

func (s *processVectorStoreStub) DeleteChunks(ctx context.Context, chunkIDs []string) error {
	return nil
}

func (s *processVectorStoreStub) UpdateChunk(ctx context.Context, chunk port.ChunkVector) error {
	return nil
}

type processEmbeddingStub struct{}

func (s processEmbeddingStub) Embed(text string) ([]float32, error) {
	return []float32{1}, nil
}

func (s processEmbeddingStub) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return []float32{1}, nil
}

func (s processEmbeddingStub) EmbedBatch(texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for range texts {
		result = append(result, []float32{1})
	}
	return result, nil
}

func (s processEmbeddingStub) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return s.EmbedBatch(texts)
}

func (s processEmbeddingStub) Dimension() int {
	return 1
}

func TestDocumentProcessServiceExecuteChunkProcessesDocument(t *testing.T) {
	documentRepo := &processDocumentRepositoryStub{document: processDocument()}
	chunkRepo := &processChunkRepositoryStub{}
	chunkLogRepo := &processChunkLogRepositoryStub{}
	vectorStore := &processVectorStoreStub{}
	svc := NewDocumentProcessService(DocumentProcessServiceOptions{
		BaseRepo:     processBaseRepositoryStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "embed-model"}},
		DocumentRepo: documentRepo,
		ChunkRepo:    chunkRepo,
		ChunkLogRepo: chunkLogRepo,
		Storage:      processStorageStub{body: "# title\n\nhello world"},
		VectorStore:  vectorStore,
		Embedding:    processEmbeddingStub{},
	})

	err := svc.ExecuteChunk(context.Background(), ExecuteChunkInput{DocumentID: "doc-1", TriggeredBy: "u-1"})
	if err != nil {
		t.Fatalf("ExecuteChunk() error = %v", err)
	}
	if len(chunkRepo.created) == 0 {
		t.Fatal("expected chunks to be created")
	}
	if len(vectorStore.upserted) != len(chunkRepo.created) {
		t.Fatalf("expected vector count to match chunk count")
	}
	if got := vectorStore.upserted[0].Metadata["document_name"]; got != "doc.md" {
		t.Fatalf("expected document_name metadata, got %v", got)
	}
	if got := vectorStore.upserted[0].Metadata["source_type"]; got != domain.KnowledgeDocumentSourceFile {
		t.Fatalf("expected source_type metadata, got %v", got)
	}
	if got := vectorStore.upserted[0].Metadata["source_file_name"]; got != "doc.md" {
		t.Fatalf("expected source_file_name metadata, got %v", got)
	}
	if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusSuccess) {
		t.Fatal("expected document to be marked success")
	}
	if !chunkLogUpdated(chunkLogRepo.updated, domain.KnowledgeDocumentChunkLogStatusSuccess) {
		t.Fatal("expected chunk log to be marked success")
	}
}

func TestDocumentProcessServiceExecuteChunkMarksFailedOnProcessingError(t *testing.T) {
	documentRepo := &processDocumentRepositoryStub{document: processDocument()}
	chunkLogRepo := &processChunkLogRepositoryStub{}
	svc := NewDocumentProcessService(DocumentProcessServiceOptions{
		BaseRepo:     processBaseRepositoryStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "embed-model"}},
		DocumentRepo: documentRepo,
		ChunkRepo:    &processChunkRepositoryStub{},
		ChunkLogRepo: chunkLogRepo,
		Storage:      processStorageStub{body: "# title\n\nhello world"},
		VectorStore:  &processVectorStoreStub{upsertError: errors.New("vector down")},
		Embedding:    processEmbeddingStub{},
	})

	err := svc.ExecuteChunk(context.Background(), ExecuteChunkInput{DocumentID: "doc-1", TriggeredBy: "u-1"})
	if err == nil {
		t.Fatal("ExecuteChunk() should return error")
	}
	if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusFailed) {
		t.Fatal("expected document to be marked failed")
	}
	if !chunkLogUpdated(chunkLogRepo.updated, domain.KnowledgeDocumentChunkLogStatusFailed) {
		t.Fatal("expected chunk log to be marked failed")
	}
}

func TestDocumentProcessServiceExecuteChunkMarksFailedStatusBackToRunningBeforeRetry(t *testing.T) {
	document := processDocument()
	document.Status = domain.KnowledgeDocumentStatusFailed
	documentRepo := &processDocumentRepositoryStub{document: document}
	chunkLogRepo := &processChunkLogRepositoryStub{}
	svc := NewDocumentProcessService(DocumentProcessServiceOptions{
		BaseRepo:     processBaseRepositoryStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "embed-model"}},
		DocumentRepo: documentRepo,
		ChunkRepo:    &processChunkRepositoryStub{},
		ChunkLogRepo: chunkLogRepo,
		Storage:      processStorageStub{body: "# title\n\nhello world"},
		VectorStore:  &processVectorStoreStub{},
		Embedding:    processEmbeddingStub{},
	})

	if err := svc.ExecuteChunk(context.Background(), ExecuteChunkInput{DocumentID: "doc-1", TriggeredBy: "u-1"}); err != nil {
		t.Fatalf("ExecuteChunk() error = %v", err)
	}
	if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusRunning) {
		t.Fatal("expected retry execution to mark document running before processing")
	}
	if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusSuccess) {
		t.Fatal("expected retry execution to mark document success")
	}
}

func TestDocumentProcessServiceExecuteChunkUsesDefaultOverlapWhenChunkConfigMissing(t *testing.T) {
	document := processDocument()
	document.Name = "doc.txt"
	document.FileType = "txt"
	documentRepo := &processDocumentRepositoryStub{document: document}
	chunkRepo := &processChunkRepositoryStub{}
	svc := NewDocumentProcessService(DocumentProcessServiceOptions{
		BaseRepo:     processBaseRepositoryStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "embed-model"}},
		DocumentRepo: documentRepo,
		ChunkRepo:    chunkRepo,
		ChunkLogRepo: &processChunkLogRepositoryStub{},
		Storage:      processStorageStub{body: strings.Repeat("a", 1000)},
		VectorStore:  &processVectorStoreStub{},
		Embedding:    processEmbeddingStub{},
	})

	if err := svc.ExecuteChunk(context.Background(), ExecuteChunkInput{DocumentID: "doc-1", TriggeredBy: "u-1"}); err != nil {
		t.Fatalf("ExecuteChunk() error = %v", err)
	}
	if len(chunkRepo.created) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunkRepo.created))
	}
	first := []rune(chunkRepo.created[0].Content)
	second := []rune(chunkRepo.created[1].Content)
	tail := string(first[len(first)-120:])
	head := string(second[:120])
	if tail != head {
		t.Fatalf("expected default overlap of 120 chars to be applied")
	}
}

func processDocument() domain.KnowledgeDocument {
	return domain.KnowledgeDocument{
		ID:              "doc-1",
		KnowledgeBaseID: "kb-1",
		Name:            "doc.md",
		Enabled:         true,
		FileURL:         "doc.md",
		FileType:        "md",
		ProcessMode:     domain.KnowledgeDocumentProcessModeChunk,
		Status:          domain.KnowledgeDocumentStatusRunning,
		SourceType:      domain.KnowledgeDocumentSourceFile,
	}
}

func documentStatusUpdated(updates []port.UpdateAssignments, status string) bool {
	for _, assignments := range updates {
		for _, assignment := range assignments {
			if assignment.Field == port.KnowledgeDocument.Status.Key && assignment.Value == status {
				return true
			}
		}
	}
	return false
}

func chunkLogUpdated(updates []domain.KnowledgeDocumentChunkLog, status string) bool {
	for _, update := range updates {
		if update.Status == status {
			return true
		}
	}
	return false
}
