package chunk

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type chunkServiceDocumentRepoStub struct {
	document     domain.KnowledgeDocument
	updateFields []port.UpdateAssignments
}

func (s *chunkServiceDocumentRepoStub) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}
func (s *chunkServiceDocumentRepoStub) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}
func (s *chunkServiceDocumentRepoStub) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	return 0, nil
}
func (s *chunkServiceDocumentRepoStub) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	s.updateFields = append(s.updateFields, set)
	return 1, nil
}
func (s *chunkServiceDocumentRepoStub) Delete(ctx context.Context, id string) error { return nil }
func (s *chunkServiceDocumentRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	return s.document, nil
}
func (s *chunkServiceDocumentRepoStub) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}
func (s *chunkServiceDocumentRepoStub) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}
func (s *chunkServiceDocumentRepoStub) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	return nil, nil
}

type chunkServiceChunkRepoStub struct {
	chunks               []domain.KnowledgeChunk
	countValue           int
	deletedID            string
	deleteByDocumentUsed bool
}

func (s *chunkServiceChunkRepoStub) Create(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	s.chunks = append(s.chunks, chunk)
	return chunk, nil
}
func (s *chunkServiceChunkRepoStub) CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error {
	s.chunks = append([]domain.KnowledgeChunk{}, chunks...)
	return nil
}
func (s *chunkServiceChunkRepoStub) Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error) {
	return chunk, nil
}
func (s *chunkServiceChunkRepoStub) Delete(ctx context.Context, id string) error {
	s.deletedID = id
	filtered := make([]domain.KnowledgeChunk, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		if chunk.ID != id {
			filtered = append(filtered, chunk)
		}
	}
	s.chunks = filtered
	return nil
}
func (s *chunkServiceChunkRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	s.deleteByDocumentUsed = true
	return nil
}
func (s *chunkServiceChunkRepoStub) UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}
func (s *chunkServiceChunkRepoStub) UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}
func (s *chunkServiceChunkRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error) {
	for _, chunk := range s.chunks {
		if chunk.ID == id {
			return chunk, nil
		}
	}
	return domain.KnowledgeChunk{}, nil
}
func (s *chunkServiceChunkRepoStub) CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error) {
	return s.countValue, nil
}
func (s *chunkServiceChunkRepoStub) List(ctx context.Context, filter port.KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error) {
	return append([]domain.KnowledgeChunk{}, s.chunks...), nil
}

type chunkServiceBaseRepoStub struct {
	base domain.KnowledgeBase
}

func (s chunkServiceBaseRepoStub) Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	return domain.KnowledgeBase{}, nil
}
func (s chunkServiceBaseRepoStub) Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	return domain.KnowledgeBase{}, nil
}
func (s chunkServiceBaseRepoStub) UpdateWhere(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error) {
	return 0, nil
}
func (s chunkServiceBaseRepoStub) Delete(ctx context.Context, id string) error { return nil }
func (s chunkServiceBaseRepoStub) GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error) {
	return s.base, nil
}
func (s chunkServiceBaseRepoStub) GetByName(ctx context.Context, name string) (int, error) {
	return 0, nil
}
func (s chunkServiceBaseRepoStub) Count(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error) {
	return 0, nil
}
func (s chunkServiceBaseRepoStub) List(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error) {
	return nil, nil
}

type chunkServiceVectorStoreStub struct {
	deletedChunkID  string
	deletedChunkIDs []string
	updatedChunkID  string
	updatedChunk    port.ChunkVector
}

func (s *chunkServiceVectorStoreStub) UpsertDocumentChunks(ctx context.Context, chunks []port.ChunkVector) error {
	return nil
}
func (s *chunkServiceVectorStoreStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return nil
}
func (s *chunkServiceVectorStoreStub) DeleteChunk(ctx context.Context, chunkID string) error {
	s.deletedChunkID = chunkID
	return nil
}
func (s *chunkServiceVectorStoreStub) DeleteChunks(ctx context.Context, chunkIDs []string) error {
	s.deletedChunkIDs = append([]string{}, chunkIDs...)
	return nil
}
func (s *chunkServiceVectorStoreStub) UpdateChunk(ctx context.Context, chunk port.ChunkVector) error {
	s.updatedChunkID = chunk.ChunkID
	s.updatedChunk = chunk
	return nil
}

type chunkServiceEmbeddingStub struct{}

func (chunkServiceEmbeddingStub) Embed(text string) ([]float32, error) { return []float32{1}, nil }
func (chunkServiceEmbeddingStub) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return []float32{1}, nil
}
func (chunkServiceEmbeddingStub) EmbedBatch(texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for range texts {
		result = append(result, []float32{1})
	}
	return result, nil
}
func (chunkServiceEmbeddingStub) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return chunkServiceEmbeddingStub{}.EmbedBatch(texts)
}
func (chunkServiceEmbeddingStub) Dimension() int { return 1 }

var _ aiembedding.EmbeddingService = chunkServiceEmbeddingStub{}

type chunkServiceTransactionStub struct {
	called bool
	err    error
}

func (s *chunkServiceTransactionStub) run(
	ctx context.Context,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
) KnowledgeChunkMutationTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
	) error {
		s.called = true
		if s.err != nil {
			return s.err
		}
		return fn(ctx, documentRepo, chunkRepo, vectorStore)
	}
}

func TestKnowledgeChunkServicePageUsesCountRepository(t *testing.T) {
	t.Parallel()

	chunkRepo := &chunkServiceChunkRepoStub{
		countValue: 2,
		chunks: []domain.KnowledgeChunk{
			{ID: "c1", DocumentID: "doc-1"},
			{ID: "c2", DocumentID: "doc-1"},
		},
	}
	service := NewKnowledgeChunkService(nil, nil, chunkRepo, nil, nil)

	result, err := service.Page(context.Background(), PageKnowledgeChunkInput{
		DocumentID: "doc-1",
		Page:       1,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("expected total from CountByDocumentID, got %d", result.Total)
	}
}

func TestKnowledgeChunkServiceDeleteUsesSingleDelete(t *testing.T) {
	t.Parallel()

	documentRepo := &chunkServiceDocumentRepoStub{
		document: domain.KnowledgeDocument{
			ID:              "doc-1",
			KnowledgeBaseID: "kb-1",
			Name:            "doc.md",
			UpdatedBy:       "alice",
		},
	}
	chunkRepo := &chunkServiceChunkRepoStub{
		chunks: []domain.KnowledgeChunk{
			{ID: "c1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", Content: "one", Enabled: true},
			{ID: "c2", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", Content: "two", Enabled: true},
		},
	}
	vectorStore := &chunkServiceVectorStoreStub{}
	service := NewKnowledgeChunkService(
		chunkServiceBaseRepoStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "emb"}},
		documentRepo,
		chunkRepo,
		vectorStore,
		chunkServiceEmbeddingStub{},
	)

	if err := service.Delete(context.Background(), DeleteKnowledgeChunkInput{
		DocumentID: "doc-1",
		ChunkID:    "c1",
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if chunkRepo.deletedID != "c1" {
		t.Fatalf("expected single delete to target c1, got %q", chunkRepo.deletedID)
	}
	if chunkRepo.deleteByDocumentUsed {
		t.Fatal("Delete() should not call DeleteByDocumentID for single chunk delete")
	}
	if vectorStore.deletedChunkID != "c1" {
		t.Fatalf("expected Delete() to delete vector for c1, got %q", vectorStore.deletedChunkID)
	}
}

func TestKnowledgeChunkServiceDeleteRunsInsideTransaction(t *testing.T) {
	t.Parallel()

	documentRepo := &chunkServiceDocumentRepoStub{
		document: domain.KnowledgeDocument{
			ID:              "doc-1",
			KnowledgeBaseID: "kb-1",
			Name:            "doc.md",
			UpdatedBy:       "alice",
		},
	}
	chunkRepo := &chunkServiceChunkRepoStub{
		chunks: []domain.KnowledgeChunk{
			{ID: "c1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", Content: "one", Enabled: true},
		},
	}
	vectorStore := &chunkServiceVectorStoreStub{}
	tx := &chunkServiceTransactionStub{}
	service := NewKnowledgeChunkService(
		chunkServiceBaseRepoStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "emb"}},
		documentRepo,
		chunkRepo,
		vectorStore,
		chunkServiceEmbeddingStub{},
		tx.run(context.Background(), documentRepo, chunkRepo, vectorStore),
	)

	if err := service.Delete(context.Background(), DeleteKnowledgeChunkInput{
		DocumentID: "doc-1",
		ChunkID:    "c1",
	}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !tx.called {
		t.Fatal("Delete() should run inside transaction")
	}
}

func TestKnowledgeChunkServiceDeletePropagatesTransactionError(t *testing.T) {
	t.Parallel()

	documentRepo := &chunkServiceDocumentRepoStub{
		document: domain.KnowledgeDocument{
			ID:              "doc-1",
			KnowledgeBaseID: "kb-1",
			Name:            "doc.md",
			UpdatedBy:       "alice",
		},
	}
	chunkRepo := &chunkServiceChunkRepoStub{
		chunks: []domain.KnowledgeChunk{
			{ID: "c1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", Content: "one", Enabled: true},
		},
	}
	vectorStore := &chunkServiceVectorStoreStub{}
	tx := &chunkServiceTransactionStub{err: errors.New("rollback")}
	service := NewKnowledgeChunkService(
		chunkServiceBaseRepoStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "emb"}},
		documentRepo,
		chunkRepo,
		vectorStore,
		chunkServiceEmbeddingStub{},
		tx.run(context.Background(), documentRepo, chunkRepo, vectorStore),
	)

	if err := service.Delete(context.Background(), DeleteKnowledgeChunkInput{
		DocumentID: "doc-1",
		ChunkID:    "c1",
	}); err == nil {
		t.Fatal("Delete() should return transaction error")
	}
	if !tx.called {
		t.Fatal("Delete() should attempt transaction before failing")
	}
}

func TestKnowledgeChunkServiceUpdateCarriesDocumentMetadataToVector(t *testing.T) {
	t.Parallel()

	documentRepo := &chunkServiceDocumentRepoStub{
		document: domain.KnowledgeDocument{
			ID:              "doc-1",
			KnowledgeBaseID: "kb-1",
			Name:            "doc.md",
			SourceType:      domain.KnowledgeDocumentSourceFile,
		},
	}
	chunkRepo := &chunkServiceChunkRepoStub{
		chunks: []domain.KnowledgeChunk{
			{ID: "c1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", ChunkIndex: 3, Content: "before", Enabled: true},
		},
	}
	vectorStore := &chunkServiceVectorStoreStub{}
	service := NewKnowledgeChunkService(
		chunkServiceBaseRepoStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "emb"}},
		documentRepo,
		chunkRepo,
		vectorStore,
		chunkServiceEmbeddingStub{},
	)

	if err := service.Update(context.Background(), UpdateKnowledgeChunkInput{
		DocumentID: "doc-1",
		ChunkID:    "c1",
		Content:    "after",
		OperatorID: "alice",
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if vectorStore.updatedChunkID != "c1" {
		t.Fatalf("expected UpdateChunk for c1, got %q", vectorStore.updatedChunkID)
	}
	if got := vectorStore.updatedChunk.Metadata["document_name"]; got != "doc.md" {
		t.Fatalf("expected document_name metadata, got %v", got)
	}
	if got := vectorStore.updatedChunk.Metadata["source_type"]; got != domain.KnowledgeDocumentSourceFile {
		t.Fatalf("expected source_type metadata, got %v", got)
	}
	if got := vectorStore.updatedChunk.Metadata["source_file_name"]; got != "doc.md" {
		t.Fatalf("expected source_file_name metadata, got %v", got)
	}
	if got := vectorStore.updatedChunk.Metadata["chunk_index"]; got != 3 {
		t.Fatalf("expected chunk_index metadata, got %v", got)
	}
}
