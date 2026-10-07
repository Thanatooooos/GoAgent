package process

import (
	"context"
	"strings"
	"time"

	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
	"local/rag-project/internal/framework/exception"
	aichat "local/rag-project/internal/infra-ai/chat"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type ExecuteChunkInput struct {
	TaskID      string
	DocumentID  string
	TriggeredBy string
}

type DocumentProcessServiceOptions struct {
	Jobs              port.ChunkJobs
	LeaseDuration     time.Duration
	BaseRepo          port.KnowledgeBaseRepository
	DocumentRepo      port.KnowledgeDocumentRepository
	ChunkRepo         port.KnowledgeChunkRepository
	ChunkLogRepo      port.KnowledgeDocumentChunkLogRepository
	Storage           port.FileStorage
	VectorStore       port.VectorStore
	Transaction       DocumentChunkPersistenceTransaction
	Parser            *coreparser.Selector
	Chunker           *corechunk.Selector
	Embedding         aiembedding.EmbeddingService
	Chat              aichat.LLMService
	EnrichmentEnabled *bool
	Now               func() time.Time
	ImageEvidence     *imageevidence.Service
}

type DocumentChunkPersistenceTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
) error

type DocumentProcessService struct {
	jobs              port.ChunkJobs
	leaseDuration     time.Duration
	baseRepo          port.KnowledgeBaseRepository
	documentRepo      port.KnowledgeDocumentRepository
	chunkRepo         port.KnowledgeChunkRepository
	chunkLogRepo      port.KnowledgeDocumentChunkLogRepository
	storage           port.FileStorage
	vectorStore       port.VectorStore
	transaction       DocumentChunkPersistenceTransaction
	parser            *coreparser.Selector
	chunker           *corechunk.Selector
	embedding         aiembedding.EmbeddingService
	chat              aichat.LLMService
	enrichmentEnabled bool
	now               func() time.Time
	imageEvidence     *imageevidence.Service
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
	enrichmentEnabled := true
	if options.EnrichmentEnabled != nil {
		enrichmentEnabled = *options.EnrichmentEnabled
	}

	return &DocumentProcessService{
		jobs:              options.Jobs,
		leaseDuration:     options.LeaseDuration,
		baseRepo:          options.BaseRepo,
		documentRepo:      options.DocumentRepo,
		chunkRepo:         options.ChunkRepo,
		chunkLogRepo:      options.ChunkLogRepo,
		storage:           options.Storage,
		vectorStore:       options.VectorStore,
		transaction:       options.Transaction,
		parser:            parserSelector,
		chunker:           chunkSelector,
		embedding:         options.Embedding,
		chat:              options.Chat,
		enrichmentEnabled: enrichmentEnabled,
		now:               now,
		imageEvidence:     options.ImageEvidence,
	}
}

func (s *DocumentProcessService) ExecuteChunk(ctx context.Context, input ExecuteChunkInput) error {
	if s != nil && s.jobs != nil {
		return s.executeOwnedChunk(ctx, input)
	}
	return s.executeChunk(ctx, input)
}

func (s *DocumentProcessService) executeChunk(ctx context.Context, input ExecuteChunkInput) error {
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
	if job, ok := port.CurrentChunkJob(ctx); ok {
		document = job.Document
		err = nil
	}
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return exception.NewClientException("knowledge document not found", nil)
	}
	if !document.Enabled {
		return exception.NewClientException("knowledge document is disabled", nil)
	}
	if _, owned := port.CurrentChunkJob(ctx); !owned {
		if err := s.ensureDocumentRunning(ctx, document, operatorID); err != nil {
			return err
		}
	}

	chunkLog, err := s.createRunningChunkLog(ctx, document)
	if err != nil {
		if _, owned := port.CurrentChunkJob(ctx); owned {
			return err
		}
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return err
	}

	if document.ProcessMode != "" && document.ProcessMode != domain.KnowledgeDocumentProcessModeChunk {
		if _, owned := port.CurrentChunkJob(ctx); owned {
			return exception.NewClientException("knowledge document process mode is not supported", nil)
		}
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, documentProcessResult{}, "knowledge document process mode is not supported")
		return exception.NewClientException("knowledge document process mode is not supported", nil)
	}

	result, err := s.processDocumentChunks(ctx, document, operatorID)
	if _, owned := port.CurrentChunkJob(ctx); owned {
		return err
	}
	if err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, result, chunkLogError(err))
		return err
	}

	if err := s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusSuccess, result, ""); err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return err
	}
	if result.ImagePending {
		return nil
	}
	if result.ImagePartial {
		if result.ImageOnly {
			return s.markDocumentFailed(ctx, document.ID, operatorID)
		}
		return s.markDocumentPartial(ctx, document.ID, operatorID)
	}
	return s.markDocumentSuccess(ctx, document.ID, operatorID)
}

func (s *DocumentProcessService) ProcessRefreshedDocument(ctx context.Context, document domain.KnowledgeDocument) error {
	if s != nil && s.jobs != nil {
		return s.processOwnedRefresh(ctx, document)
	}
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
		_ = s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusFailed, result, chunkLogError(err))
		return err
	}
	if err := s.finishChunkLog(ctx, chunkLog, domain.KnowledgeDocumentChunkLogStatusSuccess, result, ""); err != nil {
		return err
	}
	if result.ImagePending {
		return nil
	}
	if result.ImagePartial {
		if result.ImageOnly {
			return s.markDocumentFailed(ctx, document.ID, "system")
		}
		return s.markDocumentPartial(ctx, document.ID, "system")
	}
	return nil
}

type documentProcessResult struct {
	ExtractDuration int64
	ChunkDuration   int64
	EmbedDuration   int64
	PersistDuration int64
	TotalDuration   int64
	ChunkCount      int
	ImagePartial    bool
	ImagePending    bool
	ImageOnly       bool
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
