package retrieve

import (
	"context"
	"sort"
	"strings"
	"time"

	"local/rag-project/internal/framework/convention"
)

const (
	ChannelVectorGlobal  = "vector_global"
	ChannelKeyword       = "keyword"
	ChannelMetadataTitle = "metadata_title"
)

type SearchContext struct {
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float32
	RerankTopN       int
	SearchMode       string
	RouteHints       map[string]any
	IntentCandidates []string
}

type SearchChannel interface {
	Name() string
	Priority() int
	Enabled(ctx SearchContext) bool
	Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error)
}

type SearchChannelResult struct {
	ChannelName string
	Chunks      []convention.RetrievedChunk
	LatencyMs   int64
	Error       string
	Metadata    map[string]any
}

type ChannelStat struct {
	Name       string         `json:"name"`
	ChunkCount int            `json:"chunkCount"`
	LatencyMs  int64          `json:"latencyMs"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SearchProcessInput struct {
	Context        SearchContext
	ChannelResults []SearchChannelResult
	Chunks         []convention.RetrievedChunk
}

type SearchResultPostProcessor interface {
	Name() string
	Order() int
	Enabled(ctx SearchContext) bool
	Process(ctx context.Context, input SearchProcessInput) ([]convention.RetrievedChunk, error)
}

func buildSearchContext(request Request) SearchContext {
	topK := request.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}
	searchMode := normalizeSearchMode(request.SearchMode)
	return SearchContext{
		Query:            strings.TrimSpace(request.Query),
		KnowledgeBaseIDs: append([]string(nil), request.KnowledgeBaseIDs...),
		TopK:             topK,
		ScoreThreshold:   request.ScoreThreshold,
		RerankTopN:       request.RerankTopN,
		SearchMode:       searchMode,
		RouteHints:       map[string]any{},
	}
}

func normalizeSearchMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", SearchModeAuto:
		return SearchModeAuto
	case SearchModeSemantic:
		return SearchModeSemantic
	case SearchModeKeyword:
		return SearchModeKeyword
	case SearchModeHybrid:
		return SearchModeHybrid
	default:
		return SearchModeAuto
	}
}

func collectSearchChannels(results []SearchChannelResult) []string {
	nameSet := make(map[string]struct{}, len(results))
	for _, result := range results {
		name := strings.TrimSpace(result.ChannelName)
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func collectChannelStats(results []SearchChannelResult) []ChannelStat {
	stats := make([]ChannelStat, 0, len(results))
	for _, result := range results {
		stats = append(stats, ChannelStat{
			Name:       strings.TrimSpace(result.ChannelName),
			ChunkCount: len(result.Chunks),
			LatencyMs:  result.LatencyMs,
			Error:      strings.TrimSpace(result.Error),
			Metadata:   cloneMetadata(result.Metadata),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})
	return stats
}

func mergeResultMetadata(results []Result) ([]string, []ChannelStat) {
	nameSet := map[string]struct{}{}
	mergedStats := map[string]ChannelStat{}
	for _, result := range results {
		for _, name := range result.SearchChannels {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			nameSet[name] = struct{}{}
		}
		for _, stat := range result.ChannelStats {
			name := strings.TrimSpace(stat.Name)
			if name == "" {
				continue
			}
			merged := mergedStats[name]
			merged.Name = name
			merged.ChunkCount += stat.ChunkCount
			merged.LatencyMs += stat.LatencyMs
			if merged.Error == "" {
				merged.Error = strings.TrimSpace(stat.Error)
			}
			if merged.Metadata == nil && len(stat.Metadata) > 0 {
				merged.Metadata = cloneMetadata(stat.Metadata)
			}
			mergedStats[name] = merged
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	stats := make([]ChannelStat, 0, len(mergedStats))
	for _, stat := range mergedStats {
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})
	return names, stats
}

func MergeResults(results []Result, topK int) Result {
	if len(results) == 0 {
		return Result{}
	}
	chunkMap := map[string]convention.RetrievedChunk{}
	for _, result := range results {
		for _, chunk := range result.Chunks {
			if existing, ok := chunkMap[chunk.ID]; ok {
				if chunk.Score > existing.Score {
					chunkMap[chunk.ID] = chunk
				}
				continue
			}
			chunkMap[chunk.ID] = chunk
		}
	}

	chunks := make([]convention.RetrievedChunk, 0, len(chunkMap))
	for _, chunk := range chunkMap {
		chunks = append(chunks, chunk)
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})
	if topK > 0 && len(chunks) > topK {
		chunks = chunks[:topK]
	}

	searchChannels, channelStats := mergeResultMetadata(results)
	return Result{
		Chunks:           chunks,
		KnowledgeContext: BuildKnowledgeContext(chunks),
		SearchChannels:   searchChannels,
		ChannelStats:     channelStats,
	}
}

func newChannelResult(name string, chunks []convention.RetrievedChunk, startedAt time.Time, metadata map[string]any) SearchChannelResult {
	return SearchChannelResult{
		ChannelName: strings.TrimSpace(name),
		Chunks:      chunks,
		LatencyMs:   time.Since(startedAt).Milliseconds(),
		Metadata:    cloneMetadata(metadata),
	}
}

func cloneMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
