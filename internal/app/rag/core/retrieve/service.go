package retrieve

import (
	"context"
	"fmt"
	"strings"

	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/convention"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
	airerank "local/rag-project/internal/infra-ai/rerank"
)

const (
	DefaultTopK = 5

	// SearchModeAuto 自动选择更合适的检索模式。
	SearchModeAuto = "auto"
	// SearchModeSemantic 纯向量语义检索。
	SearchModeSemantic = "semantic"
	// SearchModeKeyword 纯关键词检索。
	SearchModeKeyword = "keyword"
	// SearchModeHybrid 混合检索（向量+关键词 RRF 融合）。
	SearchModeHybrid = "hybrid"
)

type Request struct {
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float32
	RerankTopN       int
	// SearchMode 检索模式：semantic / keyword / hybrid，默认 semantic。
	SearchMode string
}

type Result struct {
	Chunks           []convention.RetrievedChunk
	KnowledgeContext string
}

type Service interface {
	Retrieve(ctx context.Context, request Request) (Result, error)
	RetrieveByVector(ctx context.Context, vector []float32, request Request) (Result, error)
}

type Engine struct {
	searcher  corevector.Searcher
	embedding aiembedding.EmbeddingService
	reranker  airerank.RerankService
}

func NewEngine(searcher corevector.Searcher, embedding aiembedding.EmbeddingService, reranker airerank.RerankService) *Engine {
	return &Engine{
		searcher:  searcher,
		embedding: embedding,
		reranker:  reranker,
	}
}

func (e *Engine) Retrieve(ctx context.Context, request Request) (Result, error) {
	if e == nil {
		return Result{}, fmt.Errorf("retrieve engine is required")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return Result{}, nil
	}

	mode := resolveSearchMode(request)
	switch mode {
	case SearchModeKeyword:
		return e.retrieveByKeyword(ctx, request)
	case SearchModeHybrid:
		return e.retrieveHybrid(ctx, request)
	default:
		return e.retrieveSemantic(ctx, request)
	}
}

// retrieveSemantic 纯向量语义检索（原有行为）。
func (e *Engine) retrieveSemantic(ctx context.Context, request Request) (Result, error) {
	if e.embedding == nil {
		return Result{}, fmt.Errorf("embedding service is required")
	}
	query := strings.TrimSpace(request.Query)
	vector, err := e.embedding.Embed(query)
	if err != nil {
		return Result{}, fmt.Errorf("embed query: %w", err)
	}
	return e.RetrieveByVector(ctx, vector, request)
}

// retrieveByKeyword 纯关键词检索。
func (e *Engine) retrieveByKeyword(ctx context.Context, request Request) (Result, error) {
	if e.searcher == nil {
		return Result{}, fmt.Errorf("searcher is required")
	}
	topK := request.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	hits, err := e.searcher.SearchByKeyword(ctx, strings.TrimSpace(request.Query), request.KnowledgeBaseIDs, topK)
	if err != nil {
		return Result{}, fmt.Errorf("keyword search chunks: %w", err)
	}

	chunks := toRetrievedChunks(hits)
	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
	}, nil
}

// retrieveHybrid 混合检索：语义 + 关键词并行后 RRF 融合，再 rerank。
func (e *Engine) retrieveHybrid(ctx context.Context, request Request) (Result, error) {
	if e.searcher == nil {
		return Result{}, fmt.Errorf("searcher is required")
	}
	if e.embedding == nil {
		return Result{}, fmt.Errorf("embedding service is required")
	}
	query := strings.TrimSpace(request.Query)
	topK := request.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	// 并行执行语义检索和关键词检索。
	type searchResult struct {
		hits []corevector.SearchHit
		err  error
	}
	vectorCh := make(chan searchResult, 1)
	keywordCh := make(chan searchResult, 1)

	go func() {
		vector, err := e.embedding.Embed(query)
		if err != nil {
			vectorCh <- searchResult{err: err}
			return
		}
		hits, err := e.searcher.Search(ctx, corevector.SearchRequest{
			Vector:           vector,
			KnowledgeBaseIDs: request.KnowledgeBaseIDs,
			TopK:             topK * 2,
			ScoreThreshold:   request.ScoreThreshold,
		})
		vectorCh <- searchResult{hits: hits, err: err}
	}()

	go func() {
		hits, err := e.searcher.SearchByKeyword(ctx, query, request.KnowledgeBaseIDs, topK*2)
		keywordCh <- searchResult{hits: hits, err: err}
	}()

	vectorResult := <-vectorCh
	keywordResult := <-keywordCh

	// 两路都失败时降级为空结果。
	if vectorResult.err != nil && keywordResult.err != nil {
		return Result{}, fmt.Errorf("hybrid search both channels failed: vector=%v keyword=%v", vectorResult.err, keywordResult.err)
	}

	var chunks []convention.RetrievedChunk
	if vectorResult.err == nil && keywordResult.err == nil {
		chunks = RRFusion(vectorResult.hits, keywordResult.hits, defaultRRFK)
	} else if vectorResult.err == nil {
		chunks = toRetrievedChunks(vectorResult.hits)
	} else {
		chunks = toRetrievedChunks(keywordResult.hits)
	}

	// 融合后可选 rerank。
	if e.reranker != nil && len(chunks) > 1 {
		rerankTopN := request.RerankTopN
		if rerankTopN <= 0 || rerankTopN > len(chunks) {
			rerankTopN = len(chunks)
		}
		reranked, rerankErr := e.reranker.Rerank(query, chunks, rerankTopN)
		if rerankErr == nil && len(reranked) > 0 {
			chunks = reranked
		}
	}

	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
	}, nil
}

func (e *Engine) RetrieveByVector(ctx context.Context, vector []float32, request Request) (Result, error) {
	if e == nil || e.searcher == nil {
		return Result{}, fmt.Errorf("vector searcher is required")
	}
	if len(vector) == 0 {
		return Result{}, nil
	}

	topK := request.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	hits, err := e.searcher.Search(ctx, corevector.SearchRequest{
		Vector:           vector,
		KnowledgeBaseIDs: request.KnowledgeBaseIDs,
		TopK:             topK,
		ScoreThreshold:   request.ScoreThreshold,
	})
	if err != nil {
		return Result{}, fmt.Errorf("search chunks: %w", err)
	}

	chunks := toRetrievedChunks(hits)
	if e.reranker != nil && len(chunks) > 1 {
		topN := request.RerankTopN
		if topN <= 0 || topN > len(chunks) {
			topN = len(chunks)
		}
		reranked, rerankErr := e.reranker.Rerank(strings.TrimSpace(request.Query), chunks, topN)
		if rerankErr == nil && len(reranked) > 0 {
			chunks = reranked
		}
	}

	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
	}, nil
}

func BuildKnowledgeContext(chunks []convention.RetrievedChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var builder strings.Builder
	for idx, chunk := range chunks {
		if idx > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("[")
		builder.WriteString(fmt.Sprintf("%d", idx+1))
		builder.WriteString("]")

		// 如果 chunk 携带章节信息，在编号后附上来源标注。
		if section, ok := chunk.Metadata["section"]; ok {
			if sectionStr, ok := section.(string); ok && strings.TrimSpace(sectionStr) != "" {
				builder.WriteString(" (")
				builder.WriteString(strings.TrimSpace(sectionStr))
				builder.WriteString(")")
			}
		}

		builder.WriteString(" ")
		builder.WriteString(strings.TrimSpace(chunk.Text))
	}
	return strings.TrimSpace(builder.String())
}

func toRetrievedChunks(hits []corevector.SearchHit) []convention.RetrievedChunk {
	if len(hits) == 0 {
		return []convention.RetrievedChunk{}
	}

	result := make([]convention.RetrievedChunk, 0, len(hits))
	for _, hit := range hits {
		result = append(result, convention.RetrievedChunk{
			ID:              hit.ChunkID,
			Text:            hit.Text,
			Score:           hit.Score,
			DocumentID:      hit.DocumentID,
			KnowledgeBaseID: hit.KnowledgeBaseID,
			ChunkIndex:      hit.Index,
			Metadata:        hit.Metadata,
		})
	}
	return result
}

func resolveSearchMode(request Request) string {
	mode := strings.TrimSpace(strings.ToLower(request.SearchMode))
	switch mode {
	case SearchModeSemantic, SearchModeKeyword, SearchModeHybrid:
		return mode
	case "", SearchModeAuto:
		return inferSearchModeFromQuery(request.Query)
	default:
		return inferSearchModeFromQuery(request.Query)
	}
}

func inferSearchModeFromQuery(query string) string {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return SearchModeSemantic
	}

	hybridHints := []string{
		"`", "/", "\\", ".go", ".java", ".py", ".sql", ".yaml", ".yml", ".json",
		"报错", "异常", "错误", "error", "stack trace", "panic", "nil pointer",
		"配置", "参数", "字段", "函数", "接口", "类", "命令", "sql", "http", "api",
		"nginx", "docker", "k8s", "kubectl", "redis", "mysql", "postgres",
		"v1", "v2", "404", "500",
	}
	for _, hint := range hybridHints {
		if strings.Contains(query, hint) {
			return SearchModeHybrid
		}
	}

	keywordHints := []string{
		"包含", "出现", "叫做", "名称", "标题", "匹配", "搜索词", "关键字",
		"contains", "match", "keyword", "named",
	}
	for _, hint := range keywordHints {
		if strings.Contains(query, hint) {
			return SearchModeKeyword
		}
	}

	semanticHints := []string{
		"什么是", "含义", "定义", "原理", "作用", "为什么", "区别", "优点", "缺点", "场景",
		"how", "why", "what is", "difference", "principle", "overview",
	}
	for _, hint := range semanticHints {
		if strings.Contains(query, hint) {
			return SearchModeSemantic
		}
	}

	return SearchModeHybrid
}
