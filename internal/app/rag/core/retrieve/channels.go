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
		TopK:             expandChannelTopK(searchCtx.TopK, vectorGlobalTopKMultiplier()),
		ScoreThreshold:   searchCtx.ScoreThreshold,
		SearchMode:       SearchModeHybrid,
		Query:            searchCtx.Query,
	})
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("vector search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.TopK,
		"expandedTopK": expandChannelTopK(searchCtx.TopK, vectorGlobalTopKMultiplier()),
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
	hits, err := c.searcher.SearchByKeyword(ctx, strings.TrimSpace(searchCtx.Query), searchCtx.KnowledgeBaseIDs, expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier))
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("keyword search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.TopK,
		"expandedTopK": expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier),
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
	hits, err := c.searcher.SearchByMetadata(ctx, strings.TrimSpace(searchCtx.Query), searchCtx.KnowledgeBaseIDs, expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier))
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("metadata title search chunks: %w", err)
	}
	return newChannelResult(c.Name(), toRetrievedChunks(hits), startedAt, map[string]any{
		"topK":         searchCtx.TopK,
		"expandedTopK": expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier),
		"multiplier":   defaultChannelTopKMultiplier,
		"fields":       []string{"document_name", "source_file_name", "section"},
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
	}), nil
}

type factMemoryChannel struct {
	retriever FactMemoryRetriever
}

func NewFactMemoryChannel(retriever FactMemoryRetriever) SearchChannel {
	return &factMemoryChannel{retriever: retriever}
}

func (c *factMemoryChannel) Name() string  { return ChannelMemoryFact }
func (c *factMemoryChannel) Priority() int { return 15 }
func (c *factMemoryChannel) Enabled(ctx SearchContext) bool {
	if c == nil || c.retriever == nil {
		return false
	}
	switch normalizeSearchMode(ctx.SearchMode) {
	case SearchModeAuto, SearchModeSemantic, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *factMemoryChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	expandedTopK := expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier)
	result, err := c.retriever.SearchFacts(ctx, FactMemorySearchRequest{
		UserID:           strings.TrimSpace(searchCtx.UserID),
		Query:            strings.TrimSpace(searchCtx.Query),
		KnowledgeBaseIDs: append([]string(nil), searchCtx.KnowledgeBaseIDs...),
		TopK:             expandedTopK,
	})
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("fact memory search: %w", err)
	}

	metadata := map[string]any{
		"topK":              searchCtx.TopK,
		"expandedTopK":      expandedTopK,
		"multiplier":        defaultChannelTopKMultiplier,
		"rrfWeight":         defaultChannelRRFWeight(c.Name()),
		"candidateCount":    result.CandidateCount,
		"selectedCount":     result.SelectedCount,
		"selectedMemoryIDs": append([]string(nil), result.SelectedMemoryIDs...),
	}
	if len(result.ScopeCounts) > 0 {
		metadata["scopeCounts"] = result.ScopeCounts
	}
	if len(result.SourceCounts) > 0 {
		metadata["sourceCounts"] = result.SourceCounts
	}
	if len(result.ContributionCounts) > 0 {
		metadata["contributionCounts"] = result.ContributionCounts
	}

	return newChannelResult(c.Name(), result.Chunks, startedAt, metadata), nil
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
	case ChannelMemoryFact:
		return 0.9
	case ChannelKeyword:
		return 0.85
	case ChannelMetadataTitle:
		return 0.8
	default:
		return 1.0
	}
}
