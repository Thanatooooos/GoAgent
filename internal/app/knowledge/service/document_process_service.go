package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
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

func (s *DocumentProcessService) processDocumentChunks(ctx context.Context, document domain.KnowledgeDocument, operatorID string) (documentProcessResult, error) {
	result := documentProcessResult{}
	totalStartedAt := s.now()

	knowledgeBase, err := s.baseRepo.GetByID(ctx, document.KnowledgeBaseID)
	if err != nil {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewServiceException("failed to get knowledge base", err)
	}
	if knowledgeBase.ID == "" {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewClientException("knowledge base not found", nil)
	}

	extractStartedAt := s.now()
	text, err := s.extractDocumentText(ctx, document)
	result.ExtractDuration = elapsedMillis(extractStartedAt, s.now())
	if err != nil {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewServiceException("failed to extract knowledge document text", err)
	}
	if strings.TrimSpace(text) == "" {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewClientException("knowledge document text is empty", nil)
	}

	chunkStartedAt := s.now()
	chunks, err := s.chunker.Chunk(text, buildChunkOptions(document))
	result.ChunkDuration = elapsedMillis(chunkStartedAt, s.now())
	if err != nil {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewServiceException("failed to chunk knowledge document", err)
	}
	if len(chunks) == 0 {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewClientException("knowledge document chunks are empty", nil)
	}

	embedStartedAt := s.now()
	embedded, err := corechunk.NewEmbedder(s.embedding).AttachEmbeddingsWithModel(chunks, knowledgeBase.EmbeddingModel)
	result.EmbedDuration = elapsedMillis(embedStartedAt, s.now())
	if err != nil {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewServiceException("failed to embed knowledge document chunks", err)
	}

	domainChunks := buildKnowledgeChunks(document, embedded, operatorID)
	vectorChunks := buildChunkVectors(document, embedded)
	result.ChunkCount = len(domainChunks)

	persistStartedAt := s.now()
	if err := s.persistDocumentChunks(ctx, document.ID, domainChunks, vectorChunks, operatorID); err != nil {
		result.PersistDuration = elapsedMillis(persistStartedAt, s.now())
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, err
	}
	result.PersistDuration = elapsedMillis(persistStartedAt, s.now())
	result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
	return result, nil
}

func (s *DocumentProcessService) extractDocumentText(ctx context.Context, document domain.KnowledgeDocument) (string, error) {
	reader, err := s.storage.Open(ctx, document.FileURL)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	mimeType := detectDocumentMimeType(document)
	parser := s.parser.SelectFor(mimeType, document.Name)
	if parser == nil {
		return "", fmt.Errorf("no parser available for document: fileName=%s mimeType=%s", document.Name, mimeType)
	}

	result, err := parser.Parse(content, mimeType, map[string]any{
		"file_name": document.Name,
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func (s *DocumentProcessService) persistDocumentChunks(
	ctx context.Context,
	documentID string,
	domainChunks []domain.KnowledgeChunk,
	vectorChunks []port.ChunkVector,
	operatorID string,
) error {
	if s.transaction == nil {
		return s.persistDocumentChunksWithDeps(ctx, s.documentRepo, s.chunkRepo, s.vectorStore, documentID, domainChunks, vectorChunks, operatorID)
	}
	return s.transaction(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		return s.persistDocumentChunksWithDeps(txCtx, documentRepo, chunkRepo, vectorStore, documentID, domainChunks, vectorChunks, operatorID)
	})
}

func (s *DocumentProcessService) persistDocumentChunksWithDeps(
	ctx context.Context,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
	documentID string,
	domainChunks []domain.KnowledgeChunk,
	vectorChunks []port.ChunkVector,
	operatorID string,
) error {
	if err := chunkRepo.DeleteByDocumentID(ctx, documentID); err != nil {
		return exception.NewServiceException("failed to delete old knowledge document chunks", err)
	}
	if err := vectorStore.DeleteByDocumentID(ctx, documentID); err != nil {
		return exception.NewServiceException("failed to delete old knowledge document vectors", err)
	}
	if err := chunkRepo.CreateBatch(ctx, domainChunks); err != nil {
		return exception.NewServiceException("failed to create knowledge document chunks", err)
	}
	if err := vectorStore.UpsertDocumentChunks(ctx, vectorChunks); err != nil {
		return exception.NewServiceException("failed to upsert knowledge document vectors", err)
	}
	if err := s.updateDocumentChunkCountWithRepo(ctx, documentRepo, documentID, len(domainChunks), operatorID); err != nil {
		return err
	}
	return nil
}

func (s *DocumentProcessService) updateDocumentChunkCount(ctx context.Context, documentID string, chunkCount int, operatorID string) error {
	return s.updateDocumentChunkCountWithRepo(ctx, s.documentRepo, documentID, chunkCount, operatorID)
}

func (s *DocumentProcessService) updateDocumentChunkCountWithRepo(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, documentID string, chunkCount int, operatorID string) error {
	_, err := documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
	), port.Set(
		port.KnowledgeDocument.ChunkCount.To(chunkCount),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document chunk count", err)
	}
	return nil
}

func (s *DocumentProcessService) ensureDocumentRunning(ctx context.Context, document domain.KnowledgeDocument, operatorID string) error {
	if document.Status == domain.KnowledgeDocumentStatusRunning {
		return nil
	}
	rows, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(document.ID),
		port.KnowledgeDocument.Enabled.Eq(true),
		port.KnowledgeDocument.Deleted.Eq(false),
		port.KnowledgeDocument.Status.In(
			domain.KnowledgeDocumentStatusPending,
			domain.KnowledgeDocumentStatusFailed,
			domain.KnowledgeDocumentStatusSuccess,
		),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusRunning),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document running", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document cannot start processing", nil)
	}
	return nil
}

func (s *DocumentProcessService) markDocumentSuccess(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusSuccess),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document success", err)
	}
	return nil
}

func (s *DocumentProcessService) markDocumentFailed(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	return err
}

func (s *DocumentProcessService) createRunningChunkLog(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocumentChunkLog, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, exception.NewServiceException("failed to generate knowledge document chunk log id", err)
	}

	startTime := s.now()
	chunkLog := domain.NewKnowledgeDocumentChunkLog(fmt.Sprintf("%d", id), document.ID)
	chunkLog.Status = domain.KnowledgeDocumentChunkLogStatusRunning
	chunkLog.ProcessMode = document.ProcessMode
	chunkLog.ChunkStrategy = document.ChunkStrategy
	chunkLog.PipelineID = document.PipelineID
	chunkLog.StartTime = &startTime
	chunkLog.CreatedAt = startTime
	chunkLog.UpdatedAt = startTime

	created, err := s.chunkLogRepo.Create(ctx, chunkLog)
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, exception.NewServiceException("failed to create knowledge document chunk log", err)
	}
	return created, nil
}

func (s *DocumentProcessService) finishChunkLog(ctx context.Context, chunkLog domain.KnowledgeDocumentChunkLog, status string, result documentProcessResult, errorMessage string) error {
	endTime := s.now()
	chunkLog.Status = status
	chunkLog.ExtractDuration = result.ExtractDuration
	chunkLog.ChunkDuration = result.ChunkDuration
	chunkLog.EmbedDuration = result.EmbedDuration
	chunkLog.PersistDuration = result.PersistDuration
	chunkLog.TotalDuration = result.TotalDuration
	chunkLog.ChunkCount = result.ChunkCount
	chunkLog.ErrorMessage = truncateChunkLogError(errorMessage)
	chunkLog.EndTime = &endTime
	chunkLog.UpdatedAt = endTime

	if _, err := s.chunkLogRepo.Update(ctx, chunkLog); err != nil {
		return exception.NewServiceException("failed to update knowledge document chunk log", err)
	}
	return nil
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

func buildChunkOptions(document domain.KnowledgeDocument) corechunk.Options {
	options := corechunk.Options{
		Strategy: corechunk.Strategy(strings.TrimSpace(document.ChunkStrategy)),
	}
	if len(document.ChunkConfig) == 0 {
		return options.Normalize()
	}

	var raw struct {
		ChunkSize    int `json:"chunkSize"`
		OverlapSize  int `json:"overlapSize"`
		MinChunkSize int `json:"minChunkSize"`
		TargetChars  int `json:"targetChars"`
		OverlapChars int `json:"overlapChars"`
		MinChars     int `json:"minChars"`
	}
	if err := json.Unmarshal(document.ChunkConfig, &raw); err != nil {
		return options.Normalize()
	}

	options.ChunkSize = firstPositive(raw.ChunkSize, raw.TargetChars)
	options.OverlapSize = firstPositive(raw.OverlapSize, raw.OverlapChars)
	options.MinChunkSize = firstPositive(raw.MinChunkSize, raw.MinChars)
	return options.Normalize()
}

func buildKnowledgeChunks(document domain.KnowledgeDocument, chunks []corechunk.Chunk, operatorID string) []domain.KnowledgeChunk {
	result := make([]domain.KnowledgeChunk, 0, len(chunks))
	for index, item := range chunks {
		chunkID := strings.TrimSpace(item.ID)
		if chunkID == "" {
			chunkID = fmt.Sprintf("%s-%d", document.ID, index)
		}
		knowledgeChunk := domain.NewKnowledgeChunk(chunkID, document.KnowledgeBaseID, document.ID, item.Index, item.Text, operatorID)
		knowledgeChunk.ContentHash = contentHash(item.Text)
		knowledgeChunk.CharCount = utf8.RuneCountInString(item.Text)
		knowledgeChunk.TokenCount = len(strings.Fields(item.Text))
		result = append(result, knowledgeChunk)
	}
	return result
}

func buildChunkVectors(document domain.KnowledgeDocument, chunks []corechunk.Chunk) []port.ChunkVector {
	result := make([]port.ChunkVector, 0, len(chunks))
	for index, item := range chunks {
		chunkID := strings.TrimSpace(item.ID)
		if chunkID == "" {
			chunkID = fmt.Sprintf("%s-%d", document.ID, index)
		}
		result = append(result, port.ChunkVector{
			ChunkID:         chunkID,
			DocumentID:      document.ID,
			KnowledgeBaseID: document.KnowledgeBaseID,
			Index:           item.Index,
			Text:            item.Text,
			Embedding:       item.Embedding,
			Metadata:        buildKnowledgeVectorMetadata(document, item.Index, item.Metadata),
		})
	}
	return result
}

func buildKnowledgeVectorMetadata(document domain.KnowledgeDocument, chunkIndex int, chunkMetadata map[string]any) map[string]any {
	metadata := map[string]any{
		"document_id":       document.ID,
		"knowledge_base_id": document.KnowledgeBaseID,
		"document_name":     document.Name,
		"source_type":       document.SourceType,
		"source_file_name":  document.Name,
		"chunk_index":       chunkIndex,
	}
	for key, value := range chunkMetadata {
		metadata[key] = value
	}
	return metadata
}

func detectDocumentMimeType(document domain.KnowledgeDocument) string {
	fileType := strings.TrimSpace(strings.ToLower(document.FileType))
	if strings.Contains(fileType, "/") {
		if mediaType, _, err := mime.ParseMediaType(fileType); err == nil {
			return mediaType
		}
		return fileType
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(document.Name)), ".")
	if ext == "" {
		ext = fileType
	}
	switch ext {
	case "md", "markdown":
		return "text/markdown"
	case "txt", "text":
		return "text/plain"
	case "pdf":
		return "application/pdf"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		if ext != "" {
			if mimeType := mime.TypeByExtension("." + ext); mimeType != "" {
				if mediaType, _, err := mime.ParseMediaType(mimeType); err == nil {
					return mediaType
				}
				return mimeType
			}
		}
	}
	return "application/octet-stream"
}

func contentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func elapsedMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func truncateChunkLogError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		return message[:2000]
	}
	return message
}
