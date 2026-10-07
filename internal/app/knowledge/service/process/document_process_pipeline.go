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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	knowledgechunk "local/rag-project/internal/app/knowledge/service/chunk"
	"local/rag-project/internal/app/knowledge/service/imageevidence"
	"local/rag-project/internal/framework/exception"
)

var (
	docxBookmarkMarkerPattern = regexp.MustCompile(`(?i)\[bookmark:\s*[^\]\r\n]*\]`)
	docxPageNumberPattern     = regexp.MustCompile(`(?m)^[\t ]*(?:[—–-][\t ]*){1,2}\d+(?:[\t ]*[—–-]){1,2}[\t ]*(?:\r?\n|$)`)
)

func (s *DocumentProcessService) processDocumentChunks(ctx context.Context, document domain.KnowledgeDocument, operatorID string) (documentProcessResult, error) {
	result := documentProcessResult{}
	imageGeneration := uuid.NewString()
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
	text, parsed, source, err := s.extractDocumentForIngestion(ctx, document)
	result.ExtractDuration = elapsedMillis(extractStartedAt, s.now())
	if err != nil {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewServiceException("failed to extract knowledge document text", err)
	}
	if strings.TrimSpace(text) == "" && len(parsed.Images) == 0 {
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, exception.NewClientException("knowledge document text is empty", nil)
	}
	staged := false
	if s.imageEvidence != nil {
		staged, result.ImagePending, result.ImagePartial = s.stageDocumentImages(ctx, document, parsed, source, imageGeneration)
	}
	published := false
	defer func() {
		if staged && !published {
			_ = s.imageEvidence.AbortStage(context.Background(), document.ID, imageGeneration)
		}
	}()
	if strings.TrimSpace(text) == "" {
		result.ImageOnly = true
		if err := s.persistDocumentChunks(ctx, document.ID, nil, nil, operatorID, imageGeneration,
			staged, documentEnrichment{SummaryStatus: "skipped"}, result); err != nil {
			return result, err
		}
		published = true
		if staged {
			if err := s.imageEvidence.RefreshStatus(ctx, document.ID, imageGeneration); err != nil {
				return result, err
			}
		}
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, nil
	}

	chunkStartedAt := s.now()
	plan := buildChunkPlan(document)
	var domainChunks []domain.KnowledgeChunk
	var vectorChunks []port.ChunkVector
	var embeddedChunks []corechunk.Chunk
	parentIDs := make(map[int]string)
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
		domainChunks = buildParentChildChunks(document, pc, embedded, operatorID, imageGeneration)
		vectorChunks = buildParentChildVectors(document, pc, embedded, imageGeneration)
		embeddedChunks = embedded
		for _, child := range pc.Children {
			parentIDs[child.Chunk.Index] = versionedTextID(imageGeneration, "p", child.ParentIndex)
		}
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
		domainChunks = buildKnowledgeChunks(document, embedded, operatorID, imageGeneration)
		vectorChunks = buildChunkVectors(document, embedded, imageGeneration)
		embeddedChunks = embedded
		result.ChunkCount = len(domainChunks)
	}

	enrichment := documentEnrichment{SummaryStatus: "skipped"}
	if s.enrichmentEnabled {
		enrichment = s.enrichDocument(ctx, document, text, embeddedChunks)
		for index := range vectorChunks {
			vectorChunks[index].Metadata["chunk_summary"] = enrichment.ChunkSummaries[vectorChunks[index].Index]
			vectorChunks[index].Metadata["keywords"] = keywords(vectorChunks[index].Text)
			if enrichment.Summary != "" {
				vectorChunks[index].Metadata["document_summary"] = enrichment.Summary
			}
		}
		questionVectors, err := s.buildQuestionVectors(ctx, document, knowledgeBase.EmbeddingModel, embeddedChunks, enrichment, parentIDs, imageGeneration)
		if err != nil {
			enrichment.SummaryError = strings.TrimSpace(enrichment.SummaryError + "; question vectors: " + err.Error())
			questionVectors = nil
		}
		vectorChunks = append(vectorChunks, questionVectors...)
	}

	persistStartedAt := s.now()
	if err := s.persistDocumentChunks(ctx, document.ID, domainChunks, vectorChunks, operatorID, imageGeneration,
		staged, enrichment, result); err != nil {
		result.PersistDuration = elapsedMillis(persistStartedAt, s.now())
		result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
		return result, err
	}
	published = true
	if staged {
		if err := s.imageEvidence.RefreshStatus(ctx, document.ID, imageGeneration); err != nil {
			return result, err
		}
	}
	result.PersistDuration = elapsedMillis(persistStartedAt, s.now())
	result.TotalDuration = elapsedMillis(totalStartedAt, s.now())
	return result, nil
}

func (s *DocumentProcessService) extractDocumentText(ctx context.Context, document domain.KnowledgeDocument) (string, error) {
	text, _, _, err := s.extractDocumentForIngestion(ctx, document)
	return text, err
}

func (s *DocumentProcessService) extractDocumentForIngestion(ctx context.Context, document domain.KnowledgeDocument) (string, coreparser.ParseResult, []byte, error) {
	reader, err := s.storage.Open(ctx, document.FileURL)
	if err != nil {
		return "", coreparser.ParseResult{}, nil, err
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", coreparser.ParseResult{}, nil, err
	}

	mimeType := detectDocumentMimeType(document)
	parser := s.parser.SelectFor(mimeType, document.Name)
	if parser == nil {
		return "", coreparser.ParseResult{}, nil, fmt.Errorf("no parser available for document: fileName=%s mimeType=%s", document.Name, mimeType)
	}

	options := map[string]any{"file_name": document.Name}
	var result coreparser.ParseResult
	if s.imageEvidence != nil {
		if structured, ok := parser.(interface {
			ParseStructured(context.Context, []byte, string, map[string]any) (coreparser.ParseResult, error)
		}); ok {
			result, err = structured.ParseStructured(ctx, content, mimeType, options)
		} else {
			result, err = parser.Parse(ctx, content, mimeType, options)
		}
	} else {
		result, err = parser.Parse(ctx, content, mimeType, options)
	}
	if err != nil {
		return "", coreparser.ParseResult{}, nil, err
	}
	if mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		result.Text = cleanDOCXExtractedText(result.Text)
	}
	return result.Text, result, content, nil
}

func (s *DocumentProcessService) stageDocumentImages(ctx context.Context, document domain.KnowledgeDocument, parsed coreparser.ParseResult, source []byte, imageGeneration string) (staged, pending, degraded bool) {
	registered, err := s.imageEvidence.Register(ctx, imageevidence.RegisterInput{
		DocumentID: document.ID, KnowledgeBaseID: document.KnowledgeBaseID,
		RevisionID: imageGeneration, Stage: true,
		Source: source, Parsed: parsed,
	})
	if err != nil {
		return false, false, true
	}
	if !parsed.ImageInventoryConfirmed {
		return true, false, true
	}
	if registered.ImageCount > registered.Failures {
		return true, true, false
	}
	return true, false, registered.Failures > 0
}

// cleanDOCXExtractedText removes parser artifacts observed in DOCX files while
// leaving document content and its line structure intact.
func cleanDOCXExtractedText(text string) string {
	text = docxBookmarkMarkerPattern.ReplaceAllString(text, "")
	return docxPageNumberPattern.ReplaceAllString(text, "")
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
			ChunkSize         int   `json:"chunkSize"`
			OverlapSize       int   `json:"overlapSize"`
			MinChunkSize      int   `json:"minChunkSize"`
			TargetChars       int   `json:"targetChars"`
			OverlapChars      int   `json:"overlapChars"`
			MinChars          int   `json:"minChars"`
			EnableParentChild *bool `json:"enableParentChild"`
			ParentChunkSize   int   `json:"parentChunkSize"`
			ParentOverlapSize int   `json:"parentOverlapSize"`
			ChildChunkSize    int   `json:"childChunkSize"`
			ChildOverlapSize  int   `json:"childOverlapSize"`
		}
		if err := json.Unmarshal(document.ChunkConfig, &raw); err == nil {
			if raw.EnableParentChild != nil {
				plan.useParentChild = *raw.EnableParentChild
			}
			plan.parentOptions, plan.childOptions = corechunk.ParentChildOptions(
				strategy,
				firstPositive(raw.ParentChunkSize, raw.ChunkSize, raw.TargetChars),
				firstPositive(raw.ParentOverlapSize, raw.OverlapSize, raw.OverlapChars),
				firstPositive(raw.ChildChunkSize, 0),
				firstPositive(raw.ChildOverlapSize, 0),
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
	generation string,
) []domain.KnowledgeChunk {
	chunks := make([]domain.KnowledgeChunk, 0, len(result.Parents)+len(embedded))
	for index, parent := range result.Parents {
		parentID := versionedTextID(generation, "p", index)
		chunk := domain.NewKnowledgeChunk(parentID, document.KnowledgeBaseID, document.ID, parent.Index, parent.Text, operatorID)
		chunk.RecordType = "parent"
		chunk.ContentHash = contentHash(parent.Text)
		chunk.CharCount = utf8.RuneCountInString(parent.Text)
		chunk.TokenCount = len(strings.Fields(parent.Text))
		chunks = append(chunks, chunk)
	}
	for index, item := range embedded {
		chunkID := versionedTextID(generation, "c", item.Index)
		chunk := domain.NewKnowledgeChunk(chunkID, document.KnowledgeBaseID, document.ID, item.Index, item.Text, operatorID)
		chunk.RecordType = "child"
		chunk.ParentChunkID = versionedTextID(generation, "p", result.Children[index].ParentIndex)
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
	generation string,
) []port.ChunkVector {
	vectors := make([]port.ChunkVector, 0, len(embedded))
	for index, item := range embedded {
		parentIndex := result.Children[index].ParentIndex
		chunkID := versionedTextID(generation, "c", item.Index)
		metadata := knowledgechunk.BuildKnowledgeVectorMetadata(document, item.Index, item.Metadata)
		metadata["record_type"] = "child"
		metadata["parent_chunk_id"] = versionedTextID(generation, "p", parentIndex)
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

func buildKnowledgeChunks(document domain.KnowledgeDocument, chunks []corechunk.Chunk, operatorID, generation string) []domain.KnowledgeChunk {
	result := make([]domain.KnowledgeChunk, 0, len(chunks))
	for _, item := range chunks {
		chunkID := versionedTextID(generation, "c", item.Index)
		knowledgeChunk := domain.NewKnowledgeChunk(chunkID, document.KnowledgeBaseID, document.ID, item.Index, item.Text, operatorID)
		knowledgeChunk.ContentHash = contentHash(item.Text)
		knowledgeChunk.CharCount = utf8.RuneCountInString(item.Text)
		knowledgeChunk.TokenCount = len(strings.Fields(item.Text))
		result = append(result, knowledgeChunk)
	}
	return result
}

func buildChunkVectors(document domain.KnowledgeDocument, chunks []corechunk.Chunk, generation string) []port.ChunkVector {
	result := make([]port.ChunkVector, 0, len(chunks))
	for _, item := range chunks {
		chunkID := versionedTextID(generation, "c", item.Index)
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

func versionedTextID(generation, kind string, index int) string {
	return fmt.Sprintf("%s-%s-%d", kind, strings.ReplaceAll(generation, "-", ""), index)
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
