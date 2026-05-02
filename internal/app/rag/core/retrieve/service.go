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

const DefaultTopK = 5

type Request struct {
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float32
	RerankTopN       int
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
	if e == nil || e.embedding == nil {
		return Result{}, fmt.Errorf("embedding service is required")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return Result{}, nil
	}

	vector, err := e.embedding.Embed(query)
	if err != nil {
		return Result{}, fmt.Errorf("embed query: %w", err)
	}
	return e.RetrieveByVector(ctx, vector, request)
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
		builder.WriteString("] ")
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
