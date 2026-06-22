package recall

import (
	"context"
	"strings"

	ragcache "local/rag-project/internal/app/rag/cache"
	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/port"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/log"
)

func (r *recallService) SetRecallCache(cache port.MemoryRecallCache, options memorytypes.RecallCacheOptions) {
	if r == nil {
		return
	}
	r.cache = cache
	r.cacheOptions = memorytypes.NormalizeRecallCacheOptions(options)
}

func (r *recallService) SetCacheMetrics(metrics *cachemetrics.Service) {
	if r == nil {
		return
	}
	r.cacheMetrics = metrics
}

func (r *recallService) loadRuleMemoryProjections(ctx context.Context, userID string, query string, knowledgeBaseIDs []string, scopeTypes []string, statuses []string) ([]memoryRecallProjection, port.ScopeVersions, string, string, error) {
	scopeTypes = trimMemoryValues(scopeTypes)
	statuses = normalizeRecallStatuses(statuses)
	if len(scopeTypes) > 0 || !isDefaultRecallStatuses(statuses) {
		items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs, scopeTypes, statuses)
		if err != nil {
			return nil, port.ScopeVersions{}, "disabled", "filtered", err
		}
		projections := projectOrderedMemoryItems(query, items)
		sortRuleMemoryProjections(projections)
		return projections, port.ScopeVersions{}, "disabled", "filtered", nil
	}

	versions := port.ScopeVersions{}
	if r.canUseRedisRecallCache() {
		if loaded, ok := r.readScopeVersions(ctx, userID, knowledgeBaseIDs); ok {
			versions = loaded
		}
	}
	requestKey := buildRuleRequestCacheKey(userID, knowledgeBaseIDs, versions)
	if projections, hit := r.readRuleRequestCache(ctx, requestKey, query); hit {
		sortRuleMemoryProjections(projections)
		return projections, versions, "request", "", nil
	}

	if !r.canUseRedisRecallCache() {
		r.recordCacheMetric("rule_memories", "redis", "disabled")
		items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs, nil, statuses)
		if err != nil {
			return nil, port.ScopeVersions{}, "disabled", "cache_disabled", err
		}
		projections := projectOrderedMemoryItems(query, items)
		sortRuleMemoryProjections(projections)
		r.writeRuleRequestCache(ctx, requestKey, projections)
		return projections, versions, "disabled", "cache_disabled", nil
	}

	versions, ok := r.readScopeVersions(ctx, userID, knowledgeBaseIDs)
	if ok {
		cacheKey := port.RuleMemoryCacheKey{
			UserID:           userID,
			KnowledgeBaseIDs: knowledgeBaseIDs,
			ScopeVersions:    versions,
		}
		value, hit, err := r.cache.GetRuleMemories(ctx, cacheKey)
		if err != nil {
			r.recordCacheMetric("rule_memories", "redis", "error")
			r.recordDecodeFailure(err)
			log.Warnf("long-term memory rule cache get failed: userID=%s err=%v", userID, err)
		} else if hit {
			r.recordCacheMetric("rule_memories", "redis", "hit")
			projections := projectOrderedMemoryItems(query, cachedMemoryItemsToDomainItems(value.Items))
			sortRuleMemoryProjections(projections)
			r.writeRuleRequestCache(ctx, requestKey, projections)
			return projections, versions, "redis", "", nil
		}
		r.recordCacheMetric("rule_memories", "redis", "miss")

		items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs, nil, statuses)
		if err != nil {
			return nil, versions, "miss", "rule_cache_miss", err
		}
		if err := r.cache.SetRuleMemories(ctx, cacheKey, port.RuleMemoryCacheValue{
			Items: memoryItemsToCached(items),
		}, r.cacheOptions.RuleTTL); err != nil {
			r.recordCacheMetric("rule_memories", "redis", "error")
			log.Warnf("long-term memory rule cache set failed: userID=%s err=%v", userID, err)
		} else {
			r.recordCacheMetric("rule_memories", "redis", "set")
		}
		projections := projectOrderedMemoryItems(query, items)
		sortRuleMemoryProjections(projections)
		r.writeRuleRequestCache(ctx, requestKey, projections)
		return projections, versions, "miss", "rule_cache_miss", nil
	}

	r.recordCacheMetric("rule_memories", "redis", "fallback")
	items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs, nil, statuses)
	if err != nil {
		return nil, port.ScopeVersions{}, "fallback", "scope_version_unavailable", err
	}
	projections := projectOrderedMemoryItems(query, items)
	sortRuleMemoryProjections(projections)
	r.writeRuleRequestCache(ctx, requestKey, projections)
	return projections, port.ScopeVersions{}, "fallback", "scope_version_unavailable", nil
}

func (r *recallService) loadFactRankingProjections(
	ctx context.Context,
	userID string,
	query string,
	knowledgeBaseIDs []string,
	candidateLimit int,
) ([]memoryRecallProjection, int, port.ScopeVersions, string, string, string, error) {
	requestKey := buildFactRequestCacheKey(userID, query, knowledgeBaseIDs, candidateLimit, r.cacheOptions.EmbeddingModel, r.cacheOptions.RankVersion, port.ScopeVersions{})
	if cached, hit := r.readFactRequestCache(ctx, requestKey); hit {
		return cachedFactProjectionsToRuntime(cached.Items), cached.CandidateCount, port.ScopeVersions{}, "request", "skipped", "", nil
	}

	if !r.canUseRedisRecallCache() {
		r.recordCacheMetric("fact_rankings", "redis", "disabled")
		r.recordCacheMetric("query_embedding", "redis", "disabled")
		ranked, candidateCount, embeddingLayer, err := r.computeFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
		r.writeFactRequestCache(ctx, requestKey, ranked, candidateCount)
		return ranked, candidateCount, port.ScopeVersions{}, "disabled", embeddingLayer, "cache_disabled", err
	}

	versions, ok := r.readScopeVersions(ctx, userID, knowledgeBaseIDs)
	if ok {
		requestKey = buildFactRequestCacheKey(userID, query, knowledgeBaseIDs, candidateLimit, r.cacheOptions.EmbeddingModel, r.cacheOptions.RankVersion, versions)
		if cached, hit := r.readFactRequestCache(ctx, requestKey); hit {
			return cachedFactProjectionsToRuntime(cached.Items), cached.CandidateCount, versions, "request", "skipped", "", nil
		}

		cacheKey := port.FactRankingCacheKey{
			UserID:           userID,
			Query:            query,
			KnowledgeBaseIDs: knowledgeBaseIDs,
			CandidateLimit:   candidateLimit,
			EmbeddingModel:   strings.TrimSpace(r.cacheOptions.EmbeddingModel),
			RankVersion:      strings.TrimSpace(r.cacheOptions.RankVersion),
			ScopeVersions:    versions,
		}
		value, hit, err := r.cache.GetFactRankings(ctx, cacheKey)
		if err != nil {
			r.recordCacheMetric("fact_rankings", "redis", "error")
			r.recordDecodeFailure(err)
			log.Warnf("long-term memory fact cache get failed: userID=%s err=%v", userID, err)
		} else if hit {
			r.recordCacheMetric("fact_rankings", "redis", "hit")
			projections := cachedFactProjectionsToRuntime(value.Items)
			r.writeFactRequestCache(ctx, requestKey, projections, value.CandidateCount)
			return projections, value.CandidateCount, versions, "redis", "skipped", "", nil
		}
		r.recordCacheMetric("fact_rankings", "redis", "miss")

		ranked, candidateCount, embeddingLayer, err := r.computeFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
		if err != nil {
			return nil, 0, versions, "miss", embeddingLayer, "fact_cache_miss", err
		}
		ttl := r.cacheOptions.FactTTL
		if len(ranked) == 0 {
			ttl = r.cacheOptions.EmptyFactTTL
		}
		if err := r.cache.SetFactRankings(ctx, cacheKey, port.FactRankingCacheValue{
			CandidateCount: candidateCount,
			Items:          runtimeFactProjectionsToCached(ranked),
		}, ttl); err != nil {
			r.recordCacheMetric("fact_rankings", "redis", "error")
			log.Warnf("long-term memory fact cache set failed: userID=%s err=%v", userID, err)
		} else {
			r.recordCacheMetric("fact_rankings", "redis", "set")
		}
		r.writeFactRequestCache(ctx, requestKey, ranked, candidateCount)
		return ranked, candidateCount, versions, "miss", embeddingLayer, "fact_cache_miss", nil
	}

	r.recordCacheMetric("fact_rankings", "redis", "fallback")
	ranked, candidateCount, embeddingLayer, err := r.computeFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
	if err == nil {
		r.writeFactRequestCache(ctx, requestKey, ranked, candidateCount)
	}
	return ranked, candidateCount, port.ScopeVersions{}, "fallback", embeddingLayer, "scope_version_unavailable", err
}

func (r *recallService) computeFactRankingProjections(
	ctx context.Context,
	userID string,
	query string,
	knowledgeBaseIDs []string,
	candidateLimit int,
) ([]memoryRecallProjection, int, string, error) {
	candidates, vectorScores, embeddingLayer, err := r.loadFactMemoryCandidatesWithLimit(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
	if err != nil {
		return nil, 0, embeddingLayer, err
	}
	ranked := rankRecallMemories(query, candidates, vectorScores)
	if len(vectorScores) > 0 {
		ranked = rerankRecallMemoriesWithVectorScores(ranked, vectorScores)
	}
	return ranked, len(candidates), embeddingLayer, nil
}

func (r *recallService) embedQuery(ctx context.Context, query string) ([]float32, string, error) {
	if r == nil || r.embedding == nil {
		return nil, "disabled", nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "skipped", nil
	}

	modelID := strings.TrimSpace(r.cacheOptions.EmbeddingModel)
	requestKey := buildEmbeddingRequestCacheKey(query, modelID)
	if vector, hit := r.readEmbeddingRequestCache(ctx, requestKey); hit {
		return vector, "request", nil
	}
	if r.canUseRedisRecallCache() {
		key := port.QueryEmbeddingCacheKey{
			Query:          query,
			EmbeddingModel: modelID,
		}
		value, hit, err := r.cache.GetQueryEmbedding(ctx, key)
		if err != nil {
			r.recordCacheMetric("query_embedding", "redis", "error")
			r.recordDecodeFailure(err)
			log.Warnf("long-term memory embedding cache get failed: model=%s err=%v", modelID, err)
		} else if hit {
			r.recordCacheMetric("query_embedding", "redis", "hit")
			vector := append([]float32(nil), value...)
			r.writeEmbeddingRequestCache(ctx, requestKey, vector)
			return vector, "redis", nil
		}
		r.recordCacheMetric("query_embedding", "redis", "miss")

		vector, err := r.embedQueryDirect(query, modelID)
		if err != nil {
			return nil, "computed", err
		}
		if len(vector) > 0 {
			if err := r.cache.SetQueryEmbedding(ctx, key, vector, r.cacheOptions.EmbeddingTTL); err != nil {
				r.recordCacheMetric("query_embedding", "redis", "error")
				log.Warnf("long-term memory embedding cache set failed: model=%s err=%v", modelID, err)
			} else {
				r.recordCacheMetric("query_embedding", "redis", "set")
			}
		}
		r.writeEmbeddingRequestCache(ctx, requestKey, vector)
		return vector, "computed", nil
	}

	vector, err := r.embedQueryDirect(query, modelID)
	if err != nil {
		return nil, "computed", err
	}
	r.writeEmbeddingRequestCache(ctx, requestKey, vector)
	return vector, "computed", nil
}

func (r *recallService) embedQueryDirect(query string, modelID string) ([]float32, error) {
	if strings.TrimSpace(modelID) != "" {
		return r.embedding.EmbedWithModel(query, modelID)
	}
	return r.embedding.Embed(query)
}

func (r *recallService) canUseRecallCache() bool {
	return r.canUseRequestScopeCache() || r.canUseRedisRecallCache()
}

func (r *recallService) canUseRequestScopeCache() bool {
	return r != nil && r.cacheOptions.RequestScopeEnabled
}

func (r *recallService) canUseRedisRecallCache() bool {
	return r != nil && r.cache != nil && r.cacheOptions.Enabled
}

func (r *recallService) readScopeVersions(ctx context.Context, userID string, knowledgeBaseIDs []string) (port.ScopeVersions, bool) {
	if !r.canUseRedisRecallCache() {
		return port.ScopeVersions{}, false
	}
	versions, err := r.cache.GetScopeVersions(ctx, userID, knowledgeBaseIDs)
	if err != nil {
		if r.cacheMetrics != nil {
			r.cacheMetrics.RecordScopeVersionLookupFailure()
		}
		log.Warnf("long-term memory scope version lookup failed: userID=%s err=%v", userID, err)
		return port.ScopeVersions{}, false
	}
	return versions, true
}

func (r *recallService) readRuleRequestCache(ctx context.Context, key string, query string) ([]memoryRecallProjection, bool) {
	if !r.cacheOptions.RequestScopeEnabled {
		r.recordCacheMetric("rule_memories", "request", "disabled")
		return nil, false
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		r.recordCacheMetric("rule_memories", "request", "disabled")
		return nil, false
	}
	value, hit := cache.Get(key)
	if !hit {
		r.recordCacheMetric("rule_memories", "request", "miss")
		return nil, false
	}
	items, ok := value.([]port.CachedMemoryItem)
	if !ok {
		r.recordCacheMetric("rule_memories", "request", "error")
		return nil, false
	}
	r.recordCacheMetric("rule_memories", "request", "hit")
	return projectOrderedMemoryItems(query, cachedMemoryItemsToDomainItems(items)), true
}

func (r *recallService) writeRuleRequestCache(ctx context.Context, key string, projections []memoryRecallProjection) {
	if !r.cacheOptions.RequestScopeEnabled {
		return
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	if cache.Set(key, memoryItemsToCached(projectedMemoryItems(projections))) {
		r.recordLocalEviction()
	}
	r.recordCacheMetric("rule_memories", "request", "set")
}

type factRequestCacheValue struct {
	CandidateCount int
	Items          []port.CachedFactProjection
}

func (r *recallService) readFactRequestCache(ctx context.Context, key string) (factRequestCacheValue, bool) {
	if !r.cacheOptions.RequestScopeEnabled {
		r.recordCacheMetric("fact_rankings", "request", "disabled")
		return factRequestCacheValue{}, false
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		r.recordCacheMetric("fact_rankings", "request", "disabled")
		return factRequestCacheValue{}, false
	}
	value, hit := cache.Get(key)
	if !hit {
		r.recordCacheMetric("fact_rankings", "request", "miss")
		return factRequestCacheValue{}, false
	}
	current, ok := value.(factRequestCacheValue)
	if !ok {
		r.recordCacheMetric("fact_rankings", "request", "error")
		return factRequestCacheValue{}, false
	}
	r.recordCacheMetric("fact_rankings", "request", "hit")
	return current, true
}

func (r *recallService) writeFactRequestCache(ctx context.Context, key string, projections []memoryRecallProjection, candidateCount int) {
	if !r.cacheOptions.RequestScopeEnabled {
		return
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	if cache.Set(key, factRequestCacheValue{
		CandidateCount: candidateCount,
		Items:          runtimeFactProjectionsToCached(projections),
	}) {
		r.recordLocalEviction()
	}
	r.recordCacheMetric("fact_rankings", "request", "set")
}

func (r *recallService) readEmbeddingRequestCache(ctx context.Context, key string) ([]float32, bool) {
	if !r.cacheOptions.RequestScopeEnabled {
		r.recordCacheMetric("query_embedding", "request", "disabled")
		return nil, false
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		r.recordCacheMetric("query_embedding", "request", "disabled")
		return nil, false
	}
	value, hit := cache.Get(key)
	if !hit {
		r.recordCacheMetric("query_embedding", "request", "miss")
		return nil, false
	}
	vector, ok := value.([]float32)
	if !ok {
		r.recordCacheMetric("query_embedding", "request", "error")
		return nil, false
	}
	r.recordCacheMetric("query_embedding", "request", "hit")
	return append([]float32(nil), vector...), true
}

func (r *recallService) writeEmbeddingRequestCache(ctx context.Context, key string, vector []float32) {
	if !r.cacheOptions.RequestScopeEnabled || len(vector) == 0 {
		return
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	if cache.Set(key, append([]float32(nil), vector...)) {
		r.recordLocalEviction()
	}
	r.recordCacheMetric("query_embedding", "request", "set")
}

func (r *recallService) recordCacheMetric(cacheKind string, layer string, outcome string) {
	if r == nil || r.cacheMetrics == nil {
		return
	}
	r.cacheMetrics.Record(cacheKind, layer, outcome)
}

func (r *recallService) recordLocalEviction() {
	if r == nil || r.cacheMetrics == nil {
		return
	}
	r.cacheMetrics.RecordLocalEviction()
}

func (r *recallService) recordDecodeFailure(err error) {
	if r == nil || r.cacheMetrics == nil || err == nil {
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "unmarshal") {
		r.cacheMetrics.RecordRedisDecodeFailure()
	}
}
