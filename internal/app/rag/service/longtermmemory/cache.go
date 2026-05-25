package longtermmemory

import (
	"local/rag-project/internal/app/rag/port"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
)

type ScopeVersions = port.ScopeVersions
type CachedMemoryItem = port.CachedMemoryItem
type RuleMemoryCacheKey = port.RuleMemoryCacheKey
type RuleMemoryCacheValue = port.RuleMemoryCacheValue
type QueryEmbeddingCacheKey = port.QueryEmbeddingCacheKey
type CachedFactProjection = port.CachedFactProjection
type FactRankingCacheKey = port.FactRankingCacheKey
type FactRankingCacheValue = port.FactRankingCacheValue
type RecallCache = port.MemoryRecallCache
type RecallCacheOptions = memorytypes.RecallCacheOptions

func normalizeRecallCacheOptions(options RecallCacheOptions) RecallCacheOptions {
	return memorytypes.NormalizeRecallCacheOptions(options)
}
