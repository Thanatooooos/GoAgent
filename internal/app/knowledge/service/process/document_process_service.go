package process

import (
	"context"
	"strings"
	"time"

	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type ExecuteChunkInput struct {
	DocumentID  string
	TriggeredBy string
}

type DocumentProcessServiceOptions struct {
	BaseRepo     port.KnowledgeBaseRepository
	DocumentRepo port.KnowledgeDocumentRepository
	ChunkRepo    port.KnowledgeChunkRepository
	ChunkLogRepo port.KnowledgeDocumentChunkLogRepository
	Storage      port.FileStorage
	VectorStore  port.VectorStore
	Transaction  DocumentChunkPersistenceTransaction
	Parser       *coreparser.Selector
	Chunker      *corechunk.Selector
	Embedding    aiembedding.EmbeddingService
	Now          func() time.Time
}

type DocumentChunkPersistenceTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
) error

type DocumentProcessService struct {
	baseRepo     port.KnowledgeBaseRepository
	documentRepo port.KnowledgeDocumentRepository
	chunkRepo    port.KnowledgeChunkRepository
	chunkLogRepo port.KnowledgeDocumentChunkLogRepository
	storage      port.FileStorage
	vectorStore  port.VectorStore
	transaction  DocumentChunkPersistenceTransaction
	parser       *coreparser.Selector
	chunker      *corechunk.Selector
	embedding    aiembedding.EmbeddingService
	now          func() time.Time
}

func NewDocumentProcessService(options DocumentProcessServiceOptions) *DocumentProcessService {
	parserSelector := options.Parser
	if parserSelector == nil {
		parserSelector = coreparser.NewDefaultSelector(nil)
	}

	chunkSelector := options.Chunker
	if chunkSelector == nil {
		chunkSelector = corechunk.NewDefaultSelector()
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &DocumentProcessService{
		baseRepo:     options.BaseRepo,
		documentRepo: options.DocumentRepo,
		chunkRepo:    options.ChunkRepo,
		chunkLogRepo: options.ChunkLogRepo,
		storage:      options.Storage,
		vectorStore:  options.VectorStore,
		transaction:  options.Transaction,
		parser:       parserSelector,
		chunker:      chunkSelector,
		embedding:    options.Embedding,
		now:          now,
	}
}

func (s *DocumentProcessService) ExecuteChunk(ctx context.Context, input ExecuteChunkInput) error {
	if err := s.validateDependencies(); err != nil {
		return err
	}

	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return exception.NewClientException("knowledge document id is required", nil)
	}

	operatorID := strings.TrimSpace(input.TriggeredBy)
	if operatorID == "" {
		operatorID = "system"
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return exception.NewClientException("knowledge document not found", nil)
	}
	if !document.Enabled {
		return exception.NewClientException("knowledge document is disabled", nil)
	}
	if err := s.ensureDocumentRunning(ctx, document, operatorID); err != nil {
		return err
	}

	chunkLog, err := s.createRunningChunkLog(ctx, document)
	if err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return err
	}

	if document.ProcessMode != "" && document.ProcessMode != domain.KnowledgeDocumentProcessModeChunk {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, documentProcessResult{}, "knowledge document process mode is not supported")
		return exception.NewClientException("knowledge document process mode is not supported", nil)
	}

	result, err := s.processDocumentChunks(ctx, document, operatorID)
	if err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, result, err.Error())
		return err
	}

	if err := s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusSuccess, result, ""); err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return err
	}
	return s.markDocumentSuccess(ctx, document.ID, operatorID)
}

func (s *DocumentProcessService) ProcessRefreshedDocument(ctx context.Context, document domain.KnowledgeDocument) error {
	if err := s.validateDependencies(); err != nil {
		return err
	}
	if document.ID == "" {
		return exception.NewClientException("knowledge document id is required", nil)
	}
	if document.ProcessMode != "" && document.ProcessMode != domain.KnowledgeDocumentProcessModeChunk {
		return exception.NewClientException("knowledge document process mode is not supported", nil)
	}

	chunkLog, err := s.createRunningChunkLog(ctx, document)
	if err != nil {
		return err
	}
	result, err := s.processDocumentChunks(ctx, document, "system")
	if err != nil {
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, result, err.Error())
		return err
	}
	return s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusSuccess, result, "")
}

type documentProcessResult struct {
	ExtractDuration int64
	ChunkDuration   int64
	EmbedDuration   int64
	PersistDuration int64
	TotalDuration   int64
	ChunkCount      int
}

func (s *DocumentProcessService) validateDependencies() error {
	if s == nil {
		return exception.NewServiceException("document process service is required", nil)
	}
	if s.baseRepo == nil {
		return exception.NewServiceException("knowledge base repository is required", nil)
	}
	if s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.chunkRepo == nil {
		return exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	if s.chunkLogRepo == nil {
		return exception.NewServiceException("knowledge document chunk log repository is required", nil)
	}
	if s.storage == nil {
		return exception.NewServiceException("file storage is required", nil)
	}
	if s.vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	if s.parser == nil {
		return exception.NewServiceException("document parser selector is required", nil)
	}
	if s.chunker == nil {
		return exception.NewServiceException("document chunk selector is required", nil)
	}
	if s.embedding == nil {
		return exception.NewServiceException("embedding service is required", nil)
	}
	if s.now == nil {
		return exception.NewServiceException("document process clock is required", nil)
	}
	return nil
}
