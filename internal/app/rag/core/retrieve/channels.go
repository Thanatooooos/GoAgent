package retrieve

import (
	"context"
	"fmt"
	"strings"
	"time"

	corevector "local/rag-project/internal/app/rag/core/vector"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type vectorGlobalChannel struct {
	searcher  corevector.Searcher
	embedding aiembedding.EmbeddingService
}

func NewVectorGlobalChannel(searcher corevector.Searcher, embedding aiembedding.EmbeddingService) SearchChannel {
	return &vectorGlobalChannel{
		searcher:  searcher,
		embedding: embedding,
	}
}

func (c *vectorGlobalChannel) Name() string  { return ChannelVectorGlobal }
func (c *vectorGlobalChannel) Priority() int { return 10 }
func (c *vectorGlobalChannel) Enabled(ctx SearchContext) bool {
	if c == nil || c.searcher == nil || c.embedding == nil {
		return false
	}
	switch ctx.ResolvedMode {
	case SearchModeSemantic, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *vectorGlobalChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	vector, err := c.embedding.Embed(searchCtx.Query)
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("embed query: %w", err)
	}
	hits, err := c.searcher.Search(ctx, corevector.SearchRequest{
		Vector:           vector,
		KnowledgeBaseIDs: searchCtx.KnowledgeBaseIDs,
		TopK:             expandChannelTopK(searchCtx.TopK),
		ScoreThreshold:   searchCtx.ScoreThreshold,
		SearchMode:       searchCtx.ResolvedMode,
		Query:            searchCtx.Query,
	})
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("vector search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"requestedMode": searchCtx.RequestedMode,
		"resolvedMode":  searchCtx.ResolvedMode,
		"topK":          searchCtx.TopK,
		"expandedTopK":  expandChannelTopK(searchCtx.TopK),
		"modeSource":    searchCtx.ModeDecision.Source,
	}), nil
}

type keywordChannel struct {
	searcher corevector.Searcher
}

func NewKeywordChannel(searcher corevector.Searcher) SearchChannel {
	return &keywordChannel{searcher: searcher}
}

func (c *keywordChannel) Name() string  { return ChannelKeyword }
func (c *keywordChannel) Priority() int { return 20 }
func (c *keywordChannel) Enabled(ctx SearchContext) bool {
	if c == nil || c.searcher == nil {
		return false
	}
	switch ctx.ResolvedMode {
	case SearchModeKeyword, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *keywordChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	hits, err := c.searcher.SearchByKeyword(ctx, strings.TrimSpace(searchCtx.Query), searchCtx.KnowledgeBaseIDs, expandChannelTopK(searchCtx.TopK))
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("keyword search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"requestedMode": searchCtx.RequestedMode,
		"resolvedMode":  searchCtx.ResolvedMode,
		"topK":          searchCtx.TopK,
		"expandedTopK":  expandChannelTopK(searchCtx.TopK),
		"modeSource":    searchCtx.ModeDecision.Source,
	}), nil
}

func expandChannelTopK(topK int) int {
	if topK <= 0 {
		topK = DefaultTopK
	}
	return topK * 2
}
