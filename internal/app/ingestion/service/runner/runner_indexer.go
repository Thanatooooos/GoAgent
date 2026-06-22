package runner

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	corechunk "local/rag-project/internal/app/core/chunk"
	"local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

const (
	// IndexTargetKnowledge 表示写入 knowledge chunk 与 vector 下游。
	IndexTargetKnowledge = "knowledge"
	// IndexTargetVector 表示仅写入 vector 下游。
	IndexTargetVector = "vector"
)

// IndexerNodeRunner 提供真实索引写入实现。
type IndexerNodeRunner struct {
	baseRepo    knowledgeport.KnowledgeBaseRepository
	chunkRepo   knowledgeport.KnowledgeChunkRepository
	vectorStore knowledgeport.VectorStore
	embedding   aiembedding.EmbeddingService
	now         func() time.Time
}

// NewIndexerNodeRunner 创建 indexer 运行器。
func NewIndexerNodeRunner(
	baseRepo knowledgeport.KnowledgeBaseRepository,
	chunkRepo knowledgeport.KnowledgeChunkRepository,
	vectorStore knowledgeport.VectorStore,
	embedding aiembedding.EmbeddingService,
) *IndexerNodeRunner {
	return &IndexerNodeRunner{
		baseRepo:    baseRepo,
		chunkRepo:   chunkRepo,
		vectorStore: vectorStore,
		embedding:   embedding,
		now:         time.Now,
	}
}

// NodeType 返回当前运行器负责的节点类型。
func (r *IndexerNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeIndexer
}

// Run 将 chunk 结果写入真实下游，含失败补偿清理以保证重试可恢复。
func (r *IndexerNodeRunner) Run(ctx context.Context, state ingestionworkflow.ExecutionState, node domain.PipelineNode) (_ ingestionworkflow.ExecutionState, output map[string]any, runErr error) {
	if len(state.Chunks) == 0 {
		return state, nil, exception.NewClientException("indexer requires chunks", nil)
	}
	if r == nil || r.vectorStore == nil {
		return state, nil, exception.NewServiceException("vector store is required", nil)
	}
	if r.embedding == nil {
		return state, nil, exception.NewServiceException("embedding service is required", nil)
	}

	target := strings.ToLower(pickFirstNonEmpty(
		readStringSetting(node.Settings, "target"),
		IndexTargetKnowledge,
	))
	if target != IndexTargetKnowledge && target != IndexTargetVector {
		return state, nil, exception.NewClientException("indexer target must be knowledge or vector", nil)
	}

	knowledgeBaseID := pickFirstNonEmpty(
		readStringSetting(node.Settings, "knowledgeBaseId"),
		readStringSetting(state.Task.Metadata, "knowledgeBaseId"),
		readStringSetting(state.Task.Metadata, "kbId"),
	)
	if knowledgeBaseID == "" {
		return state, nil, exception.NewClientException("indexer requires knowledgeBaseId", nil)
	}

	documentID := pickFirstNonEmpty(
		readStringSetting(node.Settings, "documentId"),
		readStringSetting(state.Task.Metadata, "documentId"),
		state.Task.ID,
	)
	documentName := pickFirstNonEmpty(
		readStringSetting(node.Settings, "documentName"),
		readStringSetting(state.Task.Metadata, "documentName"),
		state.Source.FileName,
		state.Parsed.Title,
		state.Task.SourceFileName,
	)
	embeddingModel := pickFirstNonEmpty(
		readStringSetting(node.Settings, "embeddingModel"),
		readStringSetting(state.Task.Metadata, "embeddingModel"),
	)
	metadataFields := readStringSliceSetting(node.Settings, "metadataFields")

	base, err := r.loadKnowledgeBase(ctx, knowledgeBaseID)
	if err != nil {
		return state, nil, err
	}
	if embeddingModel == "" {
		embeddingModel = strings.TrimSpace(base.EmbeddingModel)
	}
	output = map[string]any{
		"target":          target,
		"knowledgeBaseId": knowledgeBaseID,
		"documentId":      documentID,
		"documentName":    documentName,
		"embeddingModel":  embeddingModel,
		"chunkCount":      len(state.Chunks),
	}

	embedded, err := r.embedChunks(state.Chunks, embeddingModel)
	if err != nil {
		return state, output, err
	}
	contentFingerprint := hashChunkFingerprints(embedded)
	output["contentFingerprint"] = contentFingerprint

	// 补偿清理：失败时清除已写入的部分数据，确保重试从干净状态开始。
	var (
		chunksWritten      bool
		vectorsWritten     bool
		chunkWriteMode     = "skipped"
		compensationErrors []string
	)
	defer func() {
		if runErr != nil {
			if chunksWritten && r != nil && r.chunkRepo != nil {
				if cleanupErr := r.chunkRepo.DeleteByDocumentID(ctx, documentID); cleanupErr != nil {
					compensationErrors = append(compensationErrors, fmt.Sprintf("chunk cleanup failed: %v", cleanupErr))
				}
			}
			if vectorsWritten && r != nil && r.vectorStore != nil {
				if cleanupErr := r.vectorStore.DeleteByDocumentID(ctx, documentID); cleanupErr != nil {
					compensationErrors = append(compensationErrors, fmt.Sprintf("vector cleanup failed: %v", cleanupErr))
				}
			}
			if len(output) == 0 {
				output = map[string]any{}
			}
			if len(compensationErrors) > 0 {
				output["compensation"] = map[string]any{
					"attempted": true,
					"errors":    compensationErrors,
				}
				runErr = errors.Join(append([]error{runErr}, stringSliceToErrors(compensationErrors)...)...)
			} else {
				output["compensation"] = map[string]any{
					"attempted": chunksWritten || vectorsWritten,
					"errors":    []string{},
				}
			}
		}
	}()

	vectorChunks := r.buildVectorChunks(state, documentID, documentName, knowledgeBaseID, metadataFields, embedded)
	if target == IndexTargetKnowledge {
		if r.chunkRepo == nil {
			return state, output, exception.NewServiceException("knowledge chunk repository is required", nil)
		}
		domainChunks := r.buildKnowledgeChunks(documentID, knowledgeBaseID, state.Task.CreatedBy, embedded)
		chunksMatch, err := r.knowledgeChunksMatch(ctx, documentID, domainChunks)
		if err != nil {
			return state, output, err
		}
		if chunksMatch {
			chunkWriteMode = "reuse"
		} else {
			if err := r.chunkRepo.DeleteByDocumentID(ctx, documentID); err != nil {
				return state, output, exception.NewServiceException("failed to delete existing knowledge chunks", err)
			}
			if err := r.chunkRepo.CreateBatch(ctx, domainChunks); err != nil {
				return state, output, exception.NewServiceException("failed to create knowledge chunks", err)
			}
			chunksWritten = true
			chunkWriteMode = "replace"
		}
	}
	output["chunkWriteMode"] = chunkWriteMode
	if err := r.vectorStore.DeleteByDocumentID(ctx, documentID); err != nil {
		return state, output, exception.NewServiceException("failed to delete existing knowledge vectors", err)
	}
	if err := r.vectorStore.UpsertDocumentChunks(ctx, vectorChunks); err != nil {
		return state, output, exception.NewServiceException("failed to upsert knowledge vectors", err)
	}
	vectorsWritten = true
	output["vectorWriteMode"] = "replace"

	next := state.Clone()
	next.IndexResult = ingestionworkflow.IndexResult{
		Target:     target,
		ChunkCount: len(state.Chunks),
		Metadata: map[string]any{
			"knowledgeBaseId": knowledgeBaseID,
			"documentId":      documentID,
			"documentName":    documentName,
			"embeddingModel":  embeddingModel,
			"chunkWriteMode":  chunkWriteMode,
		},
	}
	return next, output, nil
}

func (r *IndexerNodeRunner) loadKnowledgeBase(ctx context.Context, knowledgeBaseID string) (knowledgedomain.KnowledgeBase, error) {
	if r == nil || r.baseRepo == nil {
		return knowledgedomain.KnowledgeBase{}, nil
	}
	item, err := r.baseRepo.GetByID(ctx, knowledgeBaseID)
	if err != nil {
		return knowledgedomain.KnowledgeBase{}, exception.NewServiceException("failed to load knowledge base", err)
	}
	if strings.TrimSpace(item.ID) == "" {
		return knowledgedomain.KnowledgeBase{}, exception.NewClientException("knowledge base not found", nil)
	}
	return item, nil
}

func (r *IndexerNodeRunner) embedChunks(chunks []ingestionworkflow.ChunkPayload, embeddingModel string) ([]corechunk.Chunk, error) {
	coreChunks := make([]corechunk.Chunk, 0, len(chunks))
	for _, item := range chunks {
		coreChunks = append(coreChunks, corechunk.Chunk{
			Index:    item.Index,
			Text:     item.Content,
			Metadata: item.Metadata,
		})
	}

	embedded, err := corechunk.NewEmbedder(r.embedding).AttachEmbeddingsWithModel(coreChunks, embeddingModel)
	if err != nil {
		return nil, exception.NewServiceException("failed to embed ingestion chunks", err)
	}
	return embedded, nil
}

func (r *IndexerNodeRunner) buildKnowledgeChunks(
	documentID string,
	knowledgeBaseID string,
	operatorID string,
	chunks []corechunk.Chunk,
) []knowledgedomain.KnowledgeChunk {
	result := make([]knowledgedomain.KnowledgeChunk, 0, len(chunks))
	operatorID = pickFirstNonEmpty(operatorID, "system")
	now := r.now()
	for index, item := range chunks {
		chunkID := fmt.Sprintf("%s-%d", documentID, index)
		chunk := knowledgedomain.NewKnowledgeChunk(chunkID, knowledgeBaseID, documentID, item.Index, item.Text, operatorID)
		chunk.ContentHash = hashText(item.Text)
		chunk.CharCount = utf8.RuneCountInString(item.Text)
		chunk.TokenCount = len(strings.Fields(item.Text))
		chunk.CreatedAt = now
		chunk.UpdatedAt = now
		result = append(result, chunk)
	}
	return result
}

// knowledgeChunksMatch 判断本次 chunk 结果是否与现有知识块一致，用于避免无意义重写。
func (r *IndexerNodeRunner) knowledgeChunksMatch(ctx context.Context, documentID string, desired []knowledgedomain.KnowledgeChunk) (bool, error) {
	if r == nil || r.chunkRepo == nil {
		return false, nil
	}
	existing, err := r.chunkRepo.List(ctx, knowledgeport.KnowledgeChunkListFilter{
		DocumentID: documentID,
		ListOptions: knowledgeport.ListOptions{
			Limit: len(desired) + 1,
		},
	})
	if err != nil {
		return false, exception.NewServiceException("failed to list existing knowledge chunks", err)
	}
	if len(existing) != len(desired) {
		return false, nil
	}
	for index := range desired {
		if !knowledgeChunkEquals(existing[index], desired[index]) {
			return false, nil
		}
	}
	return true, nil
}

func (r *IndexerNodeRunner) buildVectorChunks(
	state ingestionworkflow.ExecutionState,
	documentID string,
	documentName string,
	knowledgeBaseID string,
	metadataFields []string,
	chunks []corechunk.Chunk,
) []knowledgeport.ChunkVector {
	result := make([]knowledgeport.ChunkVector, 0, len(chunks))
	for index, item := range chunks {
		chunkID := fmt.Sprintf("%s-%d", documentID, index)
		result = append(result, knowledgeport.ChunkVector{
			ChunkID:         chunkID,
			DocumentID:      documentID,
			KnowledgeBaseID: knowledgeBaseID,
			Index:           item.Index,
			Text:            item.Text,
			Embedding:       item.Embedding,
			Metadata:        buildIndexMetadata(state, documentID, documentName, knowledgeBaseID, item, metadataFields),
		})
	}
	return result
}

// buildIndexMetadata 构建写入向量存储的元数据，合并 chunk 级语义元数据。
func buildIndexMetadata(
	state ingestionworkflow.ExecutionState,
	documentID string,
	documentName string,
	knowledgeBaseID string,
	chunk corechunk.Chunk,
	metadataFields []string,
) map[string]any {
	result := map[string]any{
		"task_id":             state.Task.ID,
		"document_id":         documentID,
		"document_name":       documentName,
		"knowledge_base_id":   knowledgeBaseID,
		"source_type":         state.Source.Type,
		"source_location":     state.Source.Location,
		"source_file_name":    state.Source.FileName,
		"source_content_type": state.Source.ContentType,
		"chunk_index":         chunk.Index,
	}
	// 合并 chunk 级语义元数据（heading 层级、section、代码语言等）。
	for key, value := range chunk.Metadata {
		result[key] = value
	}
	for _, field := range metadataFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if value, ok := state.Task.Metadata[field]; ok {
			result[field] = value
		}
	}
	return result
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// hashChunkFingerprints 计算 chunk 集合指纹，便于排障和幂等核对。
func hashChunkFingerprints(chunks []corechunk.Chunk) string {
	if len(chunks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(chunks))
	for _, item := range chunks {
		parts = append(parts, fmt.Sprintf("%d:%s", item.Index, hashText(item.Text)))
	}
	return hashText(strings.Join(parts, "|"))
}

// knowledgeChunkEquals 用于判断现有 chunk 是否可复用。
func knowledgeChunkEquals(left knowledgedomain.KnowledgeChunk, right knowledgedomain.KnowledgeChunk) bool {
	return left.ID == right.ID &&
		left.DocumentID == right.DocumentID &&
		left.KnowledgeBaseID == right.KnowledgeBaseID &&
		left.ChunkIndex == right.ChunkIndex &&
		left.ContentHash == right.ContentHash &&
		left.Content == right.Content
}

// stringSliceToErrors 将补偿错误文本转换为 error，便于 errors.Join。
func stringSliceToErrors(items []string) []error {
	if len(items) == 0 {
		return nil
	}
	result := make([]error, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		result = append(result, errors.New(item))
	}
	return result
}
