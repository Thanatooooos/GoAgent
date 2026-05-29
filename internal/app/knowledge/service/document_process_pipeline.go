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
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

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

func buildChunkOptions(document domain.KnowledgeDocument) corechunk.Options {
	options := corechunk.Options{
		Strategy: corechunk.Strategy(strings.TrimSpace(document.ChunkStrategy)),
	}
	if len(document.ChunkConfig) == 0 {
		options.OverlapSize = 120
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
	if options.OverlapSize == 0 && raw.OverlapSize == 0 && raw.OverlapChars == 0 {
		options.OverlapSize = 120
	}
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
