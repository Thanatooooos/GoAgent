package process

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
	knowledgechunk "local/rag-project/internal/app/knowledge/service/chunk"
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
	plan := buildChunkPlan(document)
	var domainChunks []domain.KnowledgeChunk
	var vectorChunks []port.ChunkVector
	if plan.useParentChild {
		pc, err := corechunk.SplitParentChild(text, plan.parentOptions, plan.childOptions)
		if err != nil {
			result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
			return result, exception.NewServiceException("failed to split parent child chunks", err)
		}
		childChunks := make([]corechunk.Chunk, 0, len(pc.Children))
		for _, child := range pc.Children {
			childChunks = append(childChunks, child.Chunk)
		}
		embedStartedAt := s.now()
		embedded, err := corechunk.NewEmbedder(s.embedding).AttachEmbeddingsWithModel(childChunks, knowledgeBase.EmbeddingModel)
		result.EmbedDuration = elapsedMillis(embedStartedAt, s.now())
		if err != nil {
			result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
			return result, exception.NewServiceException("failed to embed knowledge document child chunks", err)
		}
		domainChunks = buildParentChildChunks(document, pc, embedded, operatorID)
		vectorChunks = buildParentChildVectors(document, pc, embedded)
		result.ChunkCount = len(pc.Children)
	} else {
		chunks, err := s.chunker.Chunk(text, plan.flatOptions)
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
		domainChunks = buildKnowledgeChunks(document, embedded, operatorID)
		vectorChunks = buildChunkVectors(document, embedded)
		result.ChunkCount = len(domainChunks)
	}

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

type documentChunkPlan struct {
	useParentChild bool
	flatOptions    corechunk.Options
	parentOptions  corechunk.Options
	childOptions   corechunk.Options
}

func buildChunkPlan(document domain.KnowledgeDocument) documentChunkPlan {
	strategy := corechunk.Strategy(strings.TrimSpace(document.ChunkStrategy))
	plan := documentChunkPlan{useParentChild: true}
	var flatChunkSize, flatOverlapSize, flatMinChunkSize int
	hasConfig := len(document.ChunkConfig) > 0
	if hasConfig {
		var raw struct {
			ChunkSize         int  `json:"chunkSize"`
			OverlapSize       int  `json:"overlapSize"`
			MinChunkSize      int  `json:"minChunkSize"`
			TargetChars       int  `json:"targetChars"`
			OverlapChars      int  `json:"overlapChars"`
			MinChars          int  `json:"minChars"`
			EnableParentChild *bool `json:"enableParentChild"`
			ParentChunkSize   int  `json:"parentChunkSize"`
			ParentOverlapSize int  `json:"parentOverlapSize"`
			ChildChunkSize    int  `json:"childChunkSize"`
			ChildOverlapSize  int  `json:"childOverlapSize"`
		}
		if err := json.Unmarshal(document.ChunkConfig, &raw); err == nil {
			if raw.EnableParentChild != nil {
				plan.useParentChild = *raw.EnableParentChild
			}
			plan.parentOptions, plan.childOptions = corechunk.ParentChildOptions(
				strategy,
				firstPositive(raw.ParentChunkSize, 0),
				raw.ParentOverlapSize,
				firstPositive(raw.ChildChunkSize, 0),
				raw.ChildOverlapSize,
			)
			flatChunkSize = firstPositive(raw.ChunkSize, raw.TargetChars)
			flatOverlapSize = firstPositive(raw.OverlapSize, raw.OverlapChars)
			flatMinChunkSize = firstPositive(raw.MinChunkSize, raw.MinChars)
		}
	}
	if plan.useParentChild {
		if !hasConfig {
			plan.parentOptions, plan.childOptions = corechunk.ParentChildOptions(strategy, 0, 0, 0, 0)
		}
		return plan
	}
	plan.flatOptions = corechunk.Options{
		Strategy:     strategy,
		ChunkSize:    flatChunkSize,
		OverlapSize:  flatOverlapSize,
		MinChunkSize: flatMinChunkSize,
	}
	if !hasConfig || (flatOverlapSize == 0 && flatChunkSize == 0) {
		plan.flatOptions.OverlapSize = 120
	}
	plan.flatOptions = plan.flatOptions.Normalize()
	return plan
}

func buildParentChildChunks(
	document domain.KnowledgeDocument,
	result corechunk.ParentChildResult,
	embedded []corechunk.Chunk,
	operatorID string,
) []domain.KnowledgeChunk {
	chunks := make([]domain.KnowledgeChunk, 0, len(result.Parents)+len(embedded))
	for index, parent := range result.Parents {
		parentID := fmt.Sprintf("%s-p-%d", document.ID, index)
		chunk := domain.NewKnowledgeChunk(parentID, document.KnowledgeBaseID, document.ID, parent.Index, parent.Text, operatorID)
		chunk.RecordType = "parent"
		chunk.ContentHash = contentHash(parent.Text)
		chunk.CharCount = utf8.RuneCountInString(parent.Text)
		chunk.TokenCount = len(strings.Fields(parent.Text))
		chunks = append(chunks, chunk)
	}
	for index, item := range embedded {
		chunkID := fmt.Sprintf("%s-%d", document.ID, item.Index)
		chunk := domain.NewKnowledgeChunk(chunkID, document.KnowledgeBaseID, document.ID, item.Index, item.Text, operatorID)
		chunk.RecordType = "child"
		chunk.ParentChunkID = fmt.Sprintf("%s-p-%d", document.ID, result.Children[index].ParentIndex)
		chunk.ContentHash = contentHash(item.Text)
		chunk.CharCount = utf8.RuneCountInString(item.Text)
		chunk.TokenCount = len(strings.Fields(item.Text))
		chunks = append(chunks, chunk)
	}
	return chunks
}

func buildParentChildVectors(
	document domain.KnowledgeDocument,
	result corechunk.ParentChildResult,
	embedded []corechunk.Chunk,
) []port.ChunkVector {
	vectors := make([]port.ChunkVector, 0, len(embedded))
	for index, item := range embedded {
		parentIndex := result.Children[index].ParentIndex
		chunkID := fmt.Sprintf("%s-%d", document.ID, item.Index)
		metadata := knowledgechunk.BuildKnowledgeVectorMetadata(document, item.Index, item.Metadata)
		metadata["record_type"] = "child"
		metadata["parent_chunk_id"] = fmt.Sprintf("%s-p-%d", document.ID, parentIndex)
		if parentIndex >= 0 && parentIndex < len(result.Parents) {
			metadata["parent_content"] = result.Parents[parentIndex].Text
		}
		vectors = append(vectors, port.ChunkVector{
			ChunkID:         chunkID,
			DocumentID:      document.ID,
			KnowledgeBaseID: document.KnowledgeBaseID,
			Index:           item.Index,
			Text:            item.Text,
			Embedding:       item.Embedding,
			Metadata:        metadata,
		})
	}
	return vectors
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
			Metadata:        knowledgechunk.BuildKnowledgeVectorMetadata(document, item.Index, item.Metadata),
		})
	}
	return result
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
