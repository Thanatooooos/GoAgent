package retrieve

import (
	"context"
	"fmt"
	"strings"
	"time"

	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/config"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

const defaultChannelTopKMultiplier = 2

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
	switch normalizeSearchMode(ctx.SearchMode) {
	case SearchModeAuto, SearchModeSemantic, SearchModeHybrid:
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
		TopK:             expandChannelTopK(searchCtx.RecallBudget, vectorGlobalTopKMultiplier()),
		ScoreThreshold:   searchCtx.ScoreThreshold,
		SearchMode:       SearchModeHybrid,
		Query:            searchCtx.Query,
	})
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("vector search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.RecallBudget,
		"expandedTopK": expandChannelTopK(searchCtx.RecallBudget, vectorGlobalTopKMultiplier()),
		"multiplier":   vectorGlobalTopKMultiplier(),
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
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
	switch normalizeSearchMode(ctx.SearchMode) {
	case SearchModeAuto, SearchModeKeyword, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *keywordChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	hits, err := c.searcher.SearchByKeyword(ctx, strings.TrimSpace(searchCtx.Query), searchCtx.KnowledgeBaseIDs, expandChannelTopK(searchCtx.RecallBudget, defaultChannelTopKMultiplier))
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("keyword search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.RecallBudget,
		"expandedTopK": expandChannelTopK(searchCtx.RecallBudget, defaultChannelTopKMultiplier),
		"multiplier":   defaultChannelTopKMultiplier,
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
	}), nil
}

type metadataTitleChannel struct {
	searcher corevector.Searcher
}

func NewMetadataTitleChannel(searcher corevector.Searcher) SearchChannel {
	return &metadataTitleChannel{searcher: searcher}
}

func (c *metadataTitleChannel) Name() string  { return ChannelMetadataTitle }
func (c *metadataTitleChannel) Priority() int { return 25 }
func (c *metadataTitleChannel) Enabled(ctx SearchContext) bool {
	if c == nil || c.searcher == nil {
		return false
	}
	if !metadataTitleChannelEnabled() {
		return false
	}
	switch normalizeSearchMode(ctx.SearchMode) {
	case SearchModeAuto, SearchModeKeyword, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *metadataTitleChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	hits, err := c.searcher.SearchByMetadata(ctx, strings.TrimSpace(searchCtx.Query), searchCtx.KnowledgeBaseIDs, expandChannelTopK(searchCtx.RecallBudget, defaultChannelTopKMultiplier))
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("metadata title search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.RecallBudget,
		"expandedTopK": expandChannelTopK(searchCtx.RecallBudget, defaultChannelTopKMultiplier),
		"multiplier":   defaultChannelTopKMultiplier,
		"fields":       []string{"document_name", "source_file_name", "section"},
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
	}), nil
}

func expandChannelTopK(topK int, multiplier int) int {
	if topK <= 0 {
		topK = DefaultTopK
	}
	if multiplier <= 0 {
		multiplier = defaultChannelTopKMultiplier
	}
	return topK * multiplier
}

func vectorGlobalTopKMultiplier() int {
	cfg := config.Get()
	if cfg == nil {
		return defaultChannelTopKMultiplier
	}
	value := cfg.Rag.Search.Channels.VectorGlobal.TopKMultiplier
	if value <= 0 {
		return defaultChannelTopKMultiplier
	}
	return value
}

func metadataTitleChannelEnabled() bool {
	cfg := config.Get()
	if cfg == nil || cfg.Rag.Search.Channels.MetadataTitle.Enabled == nil {
		return true
	}
	return *cfg.Rag.Search.Channels.MetadataTitle.Enabled
}

func defaultChannelRRFWeight(channelName string) float32 {
	switch strings.TrimSpace(channelName) {
	case ChannelVectorGlobal:
		return 1.0
	case ChannelKeyword:
		if cfg := config.Get(); cfg != nil && cfg.Rag.Search.Channels.Keyword.RRFWeight > 0 {
			return float32(cfg.Rag.Search.Channels.Keyword.RRFWeight)
		}
		return 0.85
	case ChannelMetadataTitle:
		return 0.8
	case ChannelWikiPage:
		return 0.75
	default:
		return 1.0
	}
}
