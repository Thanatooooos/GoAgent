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

	SearchModeAuto     = "auto"
	SearchModeSemantic = "semantic"
	SearchModeKeyword  = "keyword"
	SearchModeHybrid   = "hybrid"
)

type Request struct {
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float32
	RerankTopN       int
	SearchMode       string
}

type Result struct {
	Chunks           []convention.RetrievedChunk
	KnowledgeContext string
	SearchChannels   []string
	ChannelStats     []ChannelStat
}

type Service interface {
	Retrieve(ctx context.Context, request Request) (Result, error)
	RetrieveByVector(ctx context.Context, vector []float32, request Request) (Result, error)
}

type Engine struct {
	searcher   corevector.Searcher
	embedding  aiembedding.EmbeddingService
	reranker   airerank.RerankService
	channels   []SearchChannel
	processors []SearchResultPostProcessor
}

func NewEngine(searcher corevector.Searcher, embedding aiembedding.EmbeddingService, reranker airerank.RerankService) *Engine {
	engine := &Engine{
		searcher:  searcher,
		embedding: embedding,
		reranker:  reranker,
	}
	engine.channels = []SearchChannel{
		NewVectorGlobalChannel(searcher, embedding),
		NewKeywordChannel(searcher),
	}
	engine.processors = []SearchResultPostProcessor{
		NewFusionPostProcessor(),
		NewDedupPostProcessor(),
		NewRerankPostProcessor(reranker),
	}
	return engine
}

func (e *Engine) Retrieve(ctx context.Context, request Request) (Result, error) {
	if e == nil {
		return Result{}, fmt.Errorf("retrieve engine is required")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return Result{}, nil
	}

	searchCtx := buildSearchContext(request)
	channelResults, err := e.executeChannels(ctx, searchCtx)
	if err != nil {
		return Result{}, err
	}

	chunks, err := e.executeProcessors(ctx, searchCtx, channelResults)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
		SearchChannels:   collectSearchChannels(channelResults),
		ChannelStats:     collectChannelStats(channelResults),
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
		SearchChannels:   []string{ChannelVectorGlobal},
		ChannelStats: []ChannelStat{
			{
				Name:       ChannelVectorGlobal,
				ChunkCount: len(chunks),
			},
		},
	}, nil
}

func (e *Engine) executeChannels(ctx context.Context, searchCtx SearchContext) ([]SearchChannelResult, error) {
	results := make([]SearchChannelResult, 0, len(e.channels))
	successCount := 0
	var firstErr error
	for _, channel := range e.channels {
		if channel == nil || !channel.Enabled(searchCtx) {
			continue
		}
		result, err := channel.Search(ctx, searchCtx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("search channel %s: %w", channel.Name(), err)
			}
			results = append(results, SearchChannelResult{
				ChannelName: channel.Name(),
				Error:       err.Error(),
				Metadata: map[string]any{
					"status":        "failed",
					"requestedMode": searchCtx.RequestedMode,
					"resolvedMode":  searchCtx.ResolvedMode,
					"modeSource":    searchCtx.ModeDecision.Source,
				},
			})
			continue
		}
		results = append(results, result)
		successCount++
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no search channels enabled for mode %s", searchCtx.ResolvedMode)
	}
	if successCount == 0 && firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func (e *Engine) executeProcessors(ctx context.Context, searchCtx SearchContext, channelResults []SearchChannelResult) ([]convention.RetrievedChunk, error) {
	current := []convention.RetrievedChunk{}
	processors := e.processors
	if len(processors) == 0 {
		processors = []SearchResultPostProcessor{
			NewFusionPostProcessor(),
			NewDedupPostProcessor(),
			NewRerankPostProcessor(e.reranker),
		}
	}
	for _, processor := range processors {
		if processor == nil || !processor.Enabled(searchCtx) {
			continue
		}
		next, err := processor.Process(ctx, SearchProcessInput{
			Context:        searchCtx,
			ChannelResults: channelResults,
			Chunks:         current,
		})
		if err != nil {
			return nil, fmt.Errorf("post processor %s: %w", processor.Name(), err)
		}
		current = next
	}
	return current, nil
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
	return AnalyzeSearchMode(request).ResolvedMode
}

func inferSearchModeFromQuery(query string) string {
	return AnalyzeSearchMode(Request{
		Query:      query,
		SearchMode: SearchModeAuto,
	}).ResolvedMode
}
