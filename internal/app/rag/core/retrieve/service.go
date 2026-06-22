package retrieve

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
	UserID           string
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
	ChannelRetrieved map[string][]convention.RetrievedChunk
	PipelineTrace    *PipelineTrace
}

type Service interface {
	Retrieve(ctx context.Context, request Request) (Result, error)
	RetrieveByVector(ctx context.Context, vector []float32, request Request) (Result, error)
}

type Engine struct {
	searcher   corevector.Searcher
	embedding  aiembedding.EmbeddingService
	reranker   airerank.RerankService
	factMemory FactMemoryRetriever
	channels   []SearchChannel
	processors []SearchResultPostProcessor
}

func NewEngine(searcher corevector.Searcher, embedding aiembedding.EmbeddingService, reranker airerank.RerankService) *Engine {
	engine := &Engine{
		searcher:  searcher,
		embedding: embedding,
		reranker:  reranker,
	}
	engine.rebuildChannels()
	engine.processors = []SearchResultPostProcessor{
		NewFusionPostProcessor(),
		NewDedupPostProcessor(),
		NewRerankPostProcessor(reranker),
	}
	return engine
}

func (e *Engine) SetFactMemoryRetriever(retriever FactMemoryRetriever) {
	if e == nil {
		return
	}
	e.factMemory = retriever
	e.rebuildChannels()
}

func (e *Engine) rebuildChannels() {
	if e == nil {
		return
	}
	channels := []SearchChannel{
		NewVectorGlobalChannel(e.searcher, e.embedding),
		NewKeywordChannel(e.searcher),
		NewMetadataTitleChannel(e.searcher),
	}
	if e.factMemory != nil {
		channels = append(channels, NewFactMemoryChannel(e.factMemory))
	}
	e.channels = channels
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

	chunks, trace, err := e.executeProcessors(ctx, searchCtx, channelResults)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
		SearchChannels:   collectSearchChannels(channelResults),
		ChannelStats:     collectChannelStats(channelResults),
		ChannelRetrieved: collectChannelRetrieved(channelResults),
		PipelineTrace:    clonePipelineTrace(trace),
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
		ChannelRetrieved: map[string][]convention.RetrievedChunk{
			ChannelVectorGlobal: append([]convention.RetrievedChunk(nil), chunks...),
		},
	}, nil
}

type channelExecutionResult struct {
	result SearchChannelResult
	err    error
	ok     bool
}

func (e *Engine) executeChannels(ctx context.Context, searchCtx SearchContext) ([]SearchChannelResult, error) {
	enabled := make([]SearchChannel, 0, len(e.channels))
	for _, channel := range e.channels {
		if channel != nil && channel.Enabled(searchCtx) {
			enabled = append(enabled, channel)
		}
	}
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no search channels enabled")
	}

	slots := make([]channelExecutionResult, len(enabled))
	var wg sync.WaitGroup
	for i, channel := range enabled {
		i, channel := i, channel
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := channel.Search(ctx, searchCtx)
			if err != nil {
				slots[i] = channelExecutionResult{
					result: SearchChannelResult{
						ChannelName: channel.Name(),
						Error:       err.Error(),
						Metadata: map[string]any{
							"status": "failed",
						},
					},
					err: err,
				}
				return
			}
			slots[i] = channelExecutionResult{result: result, ok: true}
		}()
	}
	wg.Wait()

	results := make([]SearchChannelResult, 0, len(slots))
	successCount := 0
	var firstErr error
	for i, slot := range slots {
		if slot.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("search channel %s: %w", enabled[i].Name(), slot.err)
		}
		if slot.ok {
			successCount++
		}
		results = append(results, slot.result)
	}
	if successCount == 0 && firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func (e *Engine) executeProcessors(ctx context.Context, searchCtx SearchContext, channelResults []SearchChannelResult) ([]convention.RetrievedChunk, *PipelineTrace, error) {
	current := []convention.RetrievedChunk{}
	trace := &PipelineTrace{}
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
			Trace:          trace,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("post processor %s: %w", processor.Name(), err)
		}
		current = next
	}
	trace.FinalChunkIDs = chunkIDs(current)
	return current, trace, nil
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
