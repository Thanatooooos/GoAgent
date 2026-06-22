package runner

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
)

func TestFetcherNodeRunnerReadsLocalFile(t *testing.T) {
	tempFile, err := os.CreateTemp("", "ingestion-fetcher-*.md")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.WriteString("# title\nhello world"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	runner := NewFetcherNodeRunner(nil, nil)
	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			SourceType:     domain.TaskSourceTypeFile,
			SourceLocation: tempPath,
			SourceFileName: "sample.md",
		},
	}, domain.PipelineNode{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(state.Source.Bytes) != "# title\nhello world" {
		t.Fatalf("unexpected source bytes: %q", string(state.Source.Bytes))
	}
	if state.Source.ContentType == "" || state.Source.ContentType == "application/octet-stream" {
		t.Fatalf("unexpected content type: %q", state.Source.ContentType)
	}
	if output["contentLength"] != len(state.Source.Bytes) {
		t.Fatalf("unexpected contentLength output: %#v", output["contentLength"])
	}
}

func TestEnhancerNodeRunnerAddsContextAndMetadata(t *testing.T) {
	runner := NewEnhancerNodeRunner()

	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{ID: "task-enhancer"},
		Source: ingestionworkflow.SourcePayload{
			Type:        domain.TaskSourceTypeFile,
			FileName:    "guide.md",
			ContentType: "text/markdown",
		},
		Parsed: ingestionworkflow.ParsedDocument{
			Title:   "RAG 指南",
			Content: "RAG 用于将检索与生成结合，帮助知识库问答提升准确率。",
		},
	}, domain.PipelineNode{
		NodeID:   "enhance-1",
		NodeType: domain.PipelineNodeTypeEnhancer,
		Settings: map[string]any{
			"tasks": []any{
				map[string]any{"type": "context_enhance"},
				map[string]any{"type": "keywords"},
				map[string]any{"type": "questions"},
				map[string]any{"type": "metadata"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(state.Parsed.Content, "标题: RAG 指南") {
		t.Fatalf("expected enhanced content to include title header, got %q", state.Parsed.Content)
	}
	if len(state.Parsed.Metadata["keywords"].([]string)) == 0 {
		t.Fatalf("expected parsed keywords metadata, got %#v", state.Parsed.Metadata["keywords"])
	}
	if len(state.Parsed.Metadata["questions"].([]string)) == 0 {
		t.Fatalf("expected parsed questions metadata, got %#v", state.Parsed.Metadata["questions"])
	}
	if output["mode"] != "heuristic" {
		t.Fatalf("expected heuristic mode, got %#v", output["mode"])
	}
	if _, ok := state.Artifacts["enhancer"]; !ok {
		t.Fatalf("expected enhancer artifact to be populated")
	}
}

func TestEnricherNodeRunnerAddsChunkMetadata(t *testing.T) {
	runner := NewEnricherNodeRunner()

	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{ID: "task-enricher"},
		Source: ingestionworkflow.SourcePayload{
			Type:        domain.TaskSourceTypeFile,
			FileName:    "guide.md",
			ContentType: "text/markdown",
		},
		Parsed: ingestionworkflow.ParsedDocument{
			Title:   "RAG 指南",
			Content: "RAG 用于将检索与生成结合，帮助知识库问答提升准确率。",
			Metadata: map[string]any{
				"document_owner": "team-rag",
			},
		},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "第一块内容介绍 RAG 的作用。"},
			{Index: 1, Content: "第二块内容介绍检索增强生成的流程。"},
		},
	}, domain.PipelineNode{
		NodeID:   "enrich-1",
		NodeType: domain.PipelineNodeTypeEnricher,
		Settings: map[string]any{
			"attachDocumentMetadata": true,
			"tasks": []any{
				map[string]any{"type": "keywords"},
				map[string]any{"type": "summary"},
				map[string]any{"type": "metadata"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	first := state.Chunks[0].Metadata
	if first["document_owner"] != "team-rag" {
		t.Fatalf("expected attached document metadata, got %#v", first["document_owner"])
	}
	if _, ok := first["summary"].(string); !ok {
		t.Fatalf("expected summary metadata, got %#v", first["summary"])
	}
	if _, ok := first["keywords"].([]string); !ok {
		t.Fatalf("expected keywords metadata, got %#v", first["keywords"])
	}
	if output["attachDocumentMetadata"] != true {
		t.Fatalf("expected attachDocumentMetadata=true, got %#v", output["attachDocumentMetadata"])
	}
	if _, ok := state.Artifacts["enricher"]; !ok {
		t.Fatalf("expected enricher artifact to be populated")
	}
}

func TestIndexerNodeRunnerWritesKnowledgeChunksAndVectors(t *testing.T) {
	chunkRepo := &indexerChunkRepoStub{}
	vectorStore := &indexerVectorStoreStub{}
	embedding := &indexerEmbeddingStub{
		vectors: [][]float32{
			{0.1, 0.2},
			{0.3, 0.4},
		},
	}

	runner := NewIndexerNodeRunner(nil, chunkRepo, vectorStore, embedding)
	runner.now = func() time.Time {
		return time.Unix(1714713600, 0)
	}

	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			ID:        "task-1",
			CreatedBy: "tester",
			Metadata: map[string]any{
				"knowledgeBaseId": "kb-1",
			},
		},
		Source: ingestionworkflow.SourcePayload{
			Type:        domain.TaskSourceTypeFile,
			Location:    "/tmp/sample.md",
			FileName:    "sample.md",
			ContentType: "text/markdown",
		},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "hello world"},
			{Index: 1, Content: "bye world"},
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"embeddingModel": "text-embedding-3-small",
			"metadataFields": []string{"knowledgeBaseId"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if state.IndexResult.Target != IndexTargetKnowledge {
		t.Fatalf("unexpected index target: %q", state.IndexResult.Target)
	}
	if chunkRepo.deletedDocumentID != "task-1" {
		t.Fatalf("expected DeleteByDocumentID() to use task id, got %q", chunkRepo.deletedDocumentID)
	}
	if len(chunkRepo.created) != 2 {
		t.Fatalf("expected 2 created chunks, got %d", len(chunkRepo.created))
	}
	if len(vectorStore.upserted) != 2 {
		t.Fatalf("expected 2 upserted vectors, got %d", len(vectorStore.upserted))
	}
	if vectorStore.upserted[0].KnowledgeBaseID != "kb-1" {
		t.Fatalf("unexpected vector knowledge base id: %q", vectorStore.upserted[0].KnowledgeBaseID)
	}
	if vectorStore.upserted[0].Metadata["knowledgeBaseId"] != "kb-1" {
		t.Fatalf("expected selected metadata field to be included")
	}
	if output["embeddingModel"] != "text-embedding-3-small" {
		t.Fatalf("unexpected embedding model output: %#v", output["embeddingModel"])
	}
	if output["chunkWriteMode"] != "replace" {
		t.Fatalf("expected chunkWriteMode replace, got %#v", output["chunkWriteMode"])
	}
	if output["vectorWriteMode"] != "replace" {
		t.Fatalf("expected vectorWriteMode replace, got %#v", output["vectorWriteMode"])
	}
}

func TestIndexerNodeRunnerReusesExistingKnowledgeChunksWhenUnchanged(t *testing.T) {
	chunkRepo := &indexerChunkRepoStub{
		listResult: []knowledgedomain.KnowledgeChunk{
			{
				ID:              "doc-1-0",
				KnowledgeBaseID: "kb-1",
				DocumentID:      "doc-1",
				ChunkIndex:      0,
				Content:         "same content",
				ContentHash:     hashText("same content"),
			},
		},
	}
	vectorStore := &indexerVectorStoreStub{}
	embedding := &indexerEmbeddingStub{
		vectors: [][]float32{{0.1, 0.2}},
	}

	runner := NewIndexerNodeRunner(nil, chunkRepo, vectorStore, embedding)

	_, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			ID:        "task-reuse",
			CreatedBy: "tester",
			Metadata: map[string]any{
				"knowledgeBaseId": "kb-1",
				"documentId":      "doc-1",
			},
		},
		Source: ingestionworkflow.SourcePayload{Type: domain.TaskSourceTypeFile},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "same content"},
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"embeddingModel": "text-embedding-3-small",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if chunkRepo.deleteCalls != 0 {
		t.Fatalf("expected unchanged chunks to skip delete, got %d", chunkRepo.deleteCalls)
	}
	if len(chunkRepo.created) != 0 {
		t.Fatalf("expected unchanged chunks to skip create, got %d", len(chunkRepo.created))
	}
	if vectorStore.deleteCalls != 1 {
		t.Fatalf("expected vector delete once, got %d", vectorStore.deleteCalls)
	}
	if output["chunkWriteMode"] != "reuse" {
		t.Fatalf("expected chunkWriteMode reuse, got %#v", output["chunkWriteMode"])
	}
}

type indexerChunkRepoStub struct {
	deletedDocumentID string
	deleteCalls       int
	created           []knowledgedomain.KnowledgeChunk
	listResult        []knowledgedomain.KnowledgeChunk
}

func (s *indexerChunkRepoStub) Create(ctx context.Context, chunk knowledgedomain.KnowledgeChunk) (knowledgedomain.KnowledgeChunk, error) {
	return chunk, nil
}

func (s *indexerChunkRepoStub) CreateBatch(ctx context.Context, chunks []knowledgedomain.KnowledgeChunk) error {
	s.created = append([]knowledgedomain.KnowledgeChunk(nil), chunks...)
	return nil
}

func (s *indexerChunkRepoStub) Update(ctx context.Context, chunk knowledgedomain.KnowledgeChunk) (knowledgedomain.KnowledgeChunk, error) {
	return chunk, nil
}

func (s *indexerChunkRepoStub) Delete(ctx context.Context, id string) error { return nil }

func (s *indexerChunkRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	s.deletedDocumentID = documentID
	s.deleteCalls++
	return nil
}

func (s *indexerChunkRepoStub) UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s *indexerChunkRepoStub) UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error) {
	return 0, nil
}

func (s *indexerChunkRepoStub) GetByID(ctx context.Context, id string) (knowledgedomain.KnowledgeChunk, error) {
	return knowledgedomain.KnowledgeChunk{}, nil
}

func (s *indexerChunkRepoStub) CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error) {
	return 0, nil
}

func (s *indexerChunkRepoStub) List(ctx context.Context, filter knowledgeport.KnowledgeChunkListFilter) ([]knowledgedomain.KnowledgeChunk, error) {
	return append([]knowledgedomain.KnowledgeChunk(nil), s.listResult...), nil
}

type indexerVectorStoreStub struct {
	deletedDocumentID string
	deleteCalls       int
	upserted          []knowledgeport.ChunkVector
}

func (s *indexerVectorStoreStub) UpsertDocumentChunks(ctx context.Context, chunks []knowledgeport.ChunkVector) error {
	s.upserted = append([]knowledgeport.ChunkVector(nil), chunks...)
	return nil
}

func (s *indexerVectorStoreStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	s.deletedDocumentID = documentID
	s.deleteCalls++
	return nil
}

func (s *indexerVectorStoreStub) DeleteChunk(ctx context.Context, chunkID string) error { return nil }

func (s *indexerVectorStoreStub) DeleteChunks(ctx context.Context, chunkIDs []string) error {
	return nil
}

func (s *indexerVectorStoreStub) UpdateChunk(ctx context.Context, chunk knowledgeport.ChunkVector) error {
	return nil
}

type indexerEmbeddingStub struct {
	vectors [][]float32
}

func (s *indexerEmbeddingStub) Embed(text string) ([]float32, error) {
	return s.vectors[0], nil
}

func (s *indexerEmbeddingStub) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return s.vectors[0], nil
}

func (s *indexerEmbeddingStub) EmbedBatch(texts []string) ([][]float32, error) {
	return s.vectors, nil
}

func (s *indexerEmbeddingStub) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return s.vectors, nil
}

func (s *indexerEmbeddingStub) Dimension() int {
	if len(s.vectors) == 0 || len(s.vectors[0]) == 0 {
		return 0
	}
	return len(s.vectors[0])
}

// --- feishu fetcher 测试 ---

type feishuClientStub struct {
	content []byte
	err     error
}

func (s *feishuClientStub) FetchDocumentContent(ctx context.Context, documentID string) ([]byte, error) {
	return s.content, s.err
}

func TestFetcherNodeRunnerFeishuSource(t *testing.T) {
	runner := NewFetcherNodeRunner(nil, nil)

	feishuStub := &feishuClientStub{
		content: []byte("# feishu document\n\nhello from feishu"),
	}
	runner.feishuClient = feishuStub // 直接注入 stub（绕过接口）

	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			SourceType:     domain.TaskSourceTypeFeishu,
			SourceLocation: "https://xxx.feishu.cn/docx/DOC001",
			SourceFileName: "feishu.md",
			Metadata: map[string]any{
				"appId":     "app-1",
				"appSecret": "secret-1",
			},
		},
	}, domain.PipelineNode{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(state.Source.Bytes) != "# feishu document\n\nhello from feishu" {
		t.Fatalf("unexpected source bytes: %q", string(state.Source.Bytes))
	}
	if state.Source.Type != domain.TaskSourceTypeFeishu {
		t.Fatalf("unexpected source type: %q", state.Source.Type)
	}
	if state.Source.Metadata["source"] != "feishu" {
		t.Fatalf("expected source metadata 'feishu', got %v", state.Source.Metadata["source"])
	}
	if output["documentId"] != "DOC001" {
		t.Fatalf("expected documentId in output, got %v", output["documentId"])
	}
}

func TestFetcherNodeRunnerFeishuMissingClient(t *testing.T) {
	runner := NewFetcherNodeRunner(nil, nil)
	// 未注入 feishuClient 且未提供 appId/appSecret。

	_, _, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			SourceType:     domain.TaskSourceTypeFeishu,
			SourceLocation: "DOC001",
		},
	}, domain.PipelineNode{})
	if err == nil {
		t.Fatal("expected error for missing feishu client, got nil")
	}
}

func TestFetcherNodeRunnerFeishuSettingsCredentials(t *testing.T) {
	// 通过 node settings 传入 appId/appSecret 应创建新 client。
	runner := NewFetcherNodeRunner(nil, nil)

	_, _, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			SourceType: domain.TaskSourceTypeFeishu,
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"sourceType":     "feishu",
			"sourceLocation": "INVALID_DOC_ID",
			"appId":          "settings-app",
			"appSecret":      "settings-secret",
		},
	})
	// 因为没有真正发起 HTTP 请求，stub 是 nil，会报 client required 错误
	if err == nil {
		t.Fatal("expected error (no real feishu api), got nil")
	}
}

// --- indexer 补偿清理测试 ---

type indexerFailingChunkRepoStub struct {
	indexerChunkRepoStub
	failOnCreate      bool
	failOnDelete      bool
	failOnDeleteAfter int
}

func (s *indexerFailingChunkRepoStub) CreateBatch(ctx context.Context, chunks []knowledgedomain.KnowledgeChunk) error {
	if s.failOnCreate {
		return context.DeadlineExceeded
	}
	s.created = append([]knowledgedomain.KnowledgeChunk(nil), chunks...)
	return nil
}

func (s *indexerFailingChunkRepoStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	s.deleteCalls++
	if s.failOnDeleteAfter > 0 && s.deleteCalls > s.failOnDeleteAfter {
		return context.DeadlineExceeded
	}
	if s.failOnDelete {
		return context.DeadlineExceeded
	}
	s.deletedDocumentID = documentID
	return nil
}

type indexerFailingVectorStoreStub struct {
	indexerVectorStoreStub
	failOnUpsert bool
	failOnDelete bool
}

func (s *indexerFailingVectorStoreStub) UpsertDocumentChunks(ctx context.Context, chunks []knowledgeport.ChunkVector) error {
	if s.failOnUpsert {
		return context.DeadlineExceeded
	}
	s.upserted = append([]knowledgeport.ChunkVector(nil), chunks...)
	return nil
}

func (s *indexerFailingVectorStoreStub) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if s.failOnDelete {
		return context.DeadlineExceeded
	}
	s.deletedDocumentID = documentID
	return nil
}

func TestIndexerCompensationChunksCleanedOnVectorFailure(t *testing.T) {
	// chunk 写入成功但 vector 写入失败 → defer 清理应删除已写入的 chunks。
	chunkRepo := &indexerFailingChunkRepoStub{}
	vectorStore := &indexerFailingVectorStoreStub{
		failOnUpsert: true,
	}
	embedding := &indexerEmbeddingStub{
		vectors: [][]float32{{0.1, 0.2}},
	}

	runner := NewIndexerNodeRunner(nil, chunkRepo, vectorStore, embedding)

	_, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			ID:        "task-comp-1",
			CreatedBy: "tester",
			Metadata: map[string]any{
				"knowledgeBaseId": "kb-1",
			},
		},
		Source: ingestionworkflow.SourcePayload{
			Type:     domain.TaskSourceTypeFile,
			Location: "/tmp/test.md",
			FileName: "test.md",
		},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "test content"},
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"embeddingModel": "text-embedding-3-small",
		},
	})
	if err == nil {
		t.Fatal("expected error from vector upsert failure, got nil")
	}

	// 补偿应触发 DeleteByDocumentID 清理已写入的 chunks。
	if chunkRepo.deletedDocumentID != "task-comp-1" {
		t.Fatalf("expected compensation DeleteByDocumentID for chunks, got %q", chunkRepo.deletedDocumentID)
	}
	compensation, ok := output["compensation"].(map[string]any)
	if !ok {
		t.Fatalf("expected compensation output, got %#v", output["compensation"])
	}
	if attempted, _ := compensation["attempted"].(bool); !attempted {
		t.Fatalf("expected compensation attempted, got %#v", compensation["attempted"])
	}
}

func TestIndexerNoCompensationWhenAllSucceed(t *testing.T) {
	chunkRepo := &indexerFailingChunkRepoStub{}
	vectorStore := &indexerFailingVectorStoreStub{}
	embedding := &indexerEmbeddingStub{
		vectors: [][]float32{{0.1, 0.2}},
	}

	runner := NewIndexerNodeRunner(nil, chunkRepo, vectorStore, embedding)

	_, _, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			ID:        "task-ok",
			CreatedBy: "tester",
			Metadata: map[string]any{
				"knowledgeBaseId": "kb-1",
			},
		},
		Source: ingestionworkflow.SourcePayload{
			Type: domain.TaskSourceTypeFile,
		},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "test"},
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"embeddingModel": "text-embedding-3-small",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// 成功时 DeleteByDocumentID 仅由正常写入流程调用（2 次：chunks + vectors），不应由补偿额外触发。
	if len(chunkRepo.created) != 1 {
		t.Fatalf("expected 1 chunk created, got %d", len(chunkRepo.created))
	}
	if len(vectorStore.upserted) != 1 {
		t.Fatalf("expected 1 vector upserted, got %d", len(vectorStore.upserted))
	}
}

func TestIndexerCompensationFailureIsReturned(t *testing.T) {
	chunkRepo := &indexerFailingChunkRepoStub{failOnDeleteAfter: 1}
	vectorStore := &indexerFailingVectorStoreStub{
		failOnUpsert: true,
	}
	embedding := &indexerEmbeddingStub{
		vectors: [][]float32{{0.1, 0.2}},
	}

	runner := NewIndexerNodeRunner(nil, chunkRepo, vectorStore, embedding)

	_, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Task: domain.Task{
			ID:        "task-comp-fail",
			CreatedBy: "tester",
			Metadata: map[string]any{
				"knowledgeBaseId": "kb-1",
			},
		},
		Source: ingestionworkflow.SourcePayload{
			Type: domain.TaskSourceTypeFile,
		},
		Chunks: []ingestionworkflow.ChunkPayload{
			{Index: 0, Content: "test content"},
		},
	}, domain.PipelineNode{
		Settings: map[string]any{
			"embeddingModel": "text-embedding-3-small",
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "chunk cleanup failed") {
		t.Fatalf("expected compensation failure in error, got %v", err)
	}
	compensation, ok := output["compensation"].(map[string]any)
	if !ok {
		t.Fatalf("expected compensation output, got %#v", output["compensation"])
	}
	rawErrors, ok := compensation["errors"].([]string)
	if !ok || len(rawErrors) == 0 {
		t.Fatalf("expected compensation errors, got %#v", compensation["errors"])
	}
}
