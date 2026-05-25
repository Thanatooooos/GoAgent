package longtermmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	ragcache "local/rag-project/internal/app/rag/cache"
	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/log"
)

func (r *recallService) setRecallCache(cache RecallCache, options RecallCacheOptions) {
	if r == nil {
		return
	}
	r.cache = cache
	r.cacheOptions = normalizeRecallCacheOptions(options)
}

func (r *recallService) setCacheMetrics(metrics *cachemetrics.Service) {
	if r == nil {
		return
	}
	r.cacheMetrics = metrics
}

func (r *recallService) loadRuleMemoryProjections(ctx context.Context, userID string, query string, knowledgeBaseIDs []string) ([]memoryRecallProjection, ScopeVersions, string, string, error) {
	versions := ScopeVersions{}
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
		items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs)
		if err != nil {
			return nil, ScopeVersions{}, "disabled", "cache_disabled", err
		}
		projections := projectOrderedMemoryItems(query, items)
		sortRuleMemoryProjections(projections)
		r.writeRuleRequestCache(ctx, requestKey, projections)
		return projections, versions, "disabled", "cache_disabled", nil
	}

	versions, ok := r.readScopeVersions(ctx, userID, knowledgeBaseIDs)
	if ok {
		cacheKey := RuleMemoryCacheKey{
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

		items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs)
		if err != nil {
			return nil, versions, "miss", "rule_cache_miss", err
		}
		if err := r.cache.SetRuleMemories(ctx, cacheKey, RuleMemoryCacheValue{
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
	items, err := r.loadRuleMemories(ctx, userID, knowledgeBaseIDs)
	if err != nil {
		return nil, ScopeVersions{}, "fallback", "scope_version_unavailable", err
	}
	projections := projectOrderedMemoryItems(query, items)
	sortRuleMemoryProjections(projections)
	r.writeRuleRequestCache(ctx, requestKey, projections)
	return projections, ScopeVersions{}, "fallback", "scope_version_unavailable", nil
}

func (r *recallService) loadFactRankingProjections(
	ctx context.Context,
	userID string,
	query string,
	knowledgeBaseIDs []string,
	candidateLimit int,
) ([]memoryRecallProjection, int, ScopeVersions, string, string, string, error) {
	requestKey := buildFactRequestCacheKey(userID, query, knowledgeBaseIDs, candidateLimit, r.cacheOptions.EmbeddingModel, r.cacheOptions.RankVersion, ScopeVersions{})
	if cached, hit := r.readFactRequestCache(ctx, requestKey); hit {
		return cachedFactProjectionsToRuntime(cached.Items), cached.CandidateCount, ScopeVersions{}, "request", "skipped", "", nil
	}

	if !r.canUseRedisRecallCache() {
		r.recordCacheMetric("fact_rankings", "redis", "disabled")
		r.recordCacheMetric("query_embedding", "redis", "disabled")
		ranked, candidateCount, embeddingLayer, err := r.computeFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
		r.writeFactRequestCache(ctx, requestKey, ranked, candidateCount)
		return ranked, candidateCount, ScopeVersions{}, "disabled", embeddingLayer, "cache_disabled", err
	}

	versions, ok := r.readScopeVersions(ctx, userID, knowledgeBaseIDs)
	if ok {
		requestKey = buildFactRequestCacheKey(userID, query, knowledgeBaseIDs, candidateLimit, r.cacheOptions.EmbeddingModel, r.cacheOptions.RankVersion, versions)
		if cached, hit := r.readFactRequestCache(ctx, requestKey); hit {
			return cachedFactProjectionsToRuntime(cached.Items), cached.CandidateCount, versions, "request", "skipped", "", nil
		}

		cacheKey := FactRankingCacheKey{
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
		if err := r.cache.SetFactRankings(ctx, cacheKey, FactRankingCacheValue{
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
	return ranked, candidateCount, ScopeVersions{}, "fallback", embeddingLayer, "scope_version_unavailable", err
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
		key := QueryEmbeddingCacheKey{
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

func (r *recallService) readScopeVersions(ctx context.Context, userID string, knowledgeBaseIDs []string) (ScopeVersions, bool) {
	if !r.canUseRedisRecallCache() {
		return ScopeVersions{}, false
	}
	versions, err := r.cache.GetScopeVersions(ctx, userID, knowledgeBaseIDs)
	if err != nil {
		log.Warnf("long-term memory scope version lookup failed: userID=%s err=%v", userID, err)
		return ScopeVersions{}, false
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
	items, ok := value.([]CachedMemoryItem)
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
	Items          []CachedFactProjection
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

func buildRuleRequestCacheKey(userID string, knowledgeBaseIDs []string, versions ScopeVersions) string {
	ids := trimMemoryValues(knowledgeBaseIDs)
	sort.Strings(ids)
	return fmt.Sprintf("ltm:rules:%s:%s:%d:%s", strings.TrimSpace(userID), strings.Join(ids, ","), versions.GlobalVersion, hashScopeVersions(versions.KBVersions))
}

func buildFactRequestCacheKey(userID string, query string, knowledgeBaseIDs []string, candidateLimit int, embeddingModel string, rankVersion string, versions ScopeVersions) string {
	ids := trimMemoryValues(knowledgeBaseIDs)
	sort.Strings(ids)
	return fmt.Sprintf(
		"ltm:facts:%s:%s:%s:%d:%s:%s:%d:%s",
		strings.TrimSpace(userID),
		normalizeQueryCacheText(query),
		strings.Join(ids, ","),
		candidateLimit,
		strings.TrimSpace(embeddingModel),
		strings.TrimSpace(rankVersion),
		versions.GlobalVersion,
		hashScopeVersions(versions.KBVersions),
	)
}

func buildEmbeddingRequestCacheKey(query string, modelID string) string {
	return fmt.Sprintf("embed:%s:%s", strings.TrimSpace(modelID), normalizeQueryCacheText(query))
}

func normalizeQueryCacheText(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func hashScopeVersions(values map[string]int64) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, strings.TrimSpace(key))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		parts = append(parts, fmt.Sprintf("%s=%d", strings.TrimSpace(key), value))
	}
	return strings.Join(parts, ",")
}

func memoryItemsToCached(items []domain.MemoryItem) []CachedMemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]CachedMemoryItem, 0, len(items))
	for _, item := range items {
		cached := CachedMemoryItem{
			ID:           strings.TrimSpace(item.ID),
			UserID:       strings.TrimSpace(item.UserID),
			ScopeType:    strings.TrimSpace(item.ScopeType),
			ScopeID:      strings.TrimSpace(item.ScopeID),
			Namespace:    strings.TrimSpace(item.Namespace),
			MemoryType:   strings.TrimSpace(item.MemoryType),
			Category:     strings.TrimSpace(item.Category),
			CanonicalKey: strings.TrimSpace(item.CanonicalKey),
			ValueType:    strings.TrimSpace(item.ValueType),
			ValueJSON:    strings.TrimSpace(item.ValueJSON),
			DisplayValue: strings.TrimSpace(item.DisplayValue),
			Content:      strings.TrimSpace(item.Content),
			Summary:      strings.TrimSpace(item.Summary),
			Status:       strings.TrimSpace(item.Status),
			Importance:   item.Importance,
			UpdateTime:   item.UpdateTime,
		}
		if item.LastConfirmedAt != nil {
			cached.LastConfirmedAt = *item.LastConfirmedAt
		}
		result = append(result, cached)
	}
	return result
}

func cachedMemoryItemsToDomainItems(items []CachedMemoryItem) []domain.MemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.MemoryItem, 0, len(items))
	for _, item := range items {
		current := domain.MemoryItem{
			ID:           strings.TrimSpace(item.ID),
			UserID:       strings.TrimSpace(item.UserID),
			ScopeType:    strings.TrimSpace(item.ScopeType),
			ScopeID:      strings.TrimSpace(item.ScopeID),
			Namespace:    strings.TrimSpace(item.Namespace),
			MemoryType:   strings.TrimSpace(item.MemoryType),
			Category:     strings.TrimSpace(item.Category),
			CanonicalKey: strings.TrimSpace(item.CanonicalKey),
			ValueType:    strings.TrimSpace(item.ValueType),
			ValueJSON:    strings.TrimSpace(item.ValueJSON),
			DisplayValue: strings.TrimSpace(item.DisplayValue),
			Content:      strings.TrimSpace(item.Content),
			Summary:      strings.TrimSpace(item.Summary),
			Status:       strings.TrimSpace(item.Status),
			Importance:   item.Importance,
			UpdateTime:   item.UpdateTime,
		}
		if !item.LastConfirmedAt.IsZero() {
			lastConfirmedAt := item.LastConfirmedAt
			current.LastConfirmedAt = &lastConfirmedAt
		}
		result = append(result, current)
	}
	return result
}

func runtimeFactProjectionsToCached(items []memoryRecallProjection) []CachedFactProjection {
	if len(items) == 0 {
		return nil
	}
	result := make([]CachedFactProjection, 0, len(items))
	for _, item := range items {
		result = append(result, CachedFactProjection{
			MemoryID:       strings.TrimSpace(item.item.ID),
			ScopeType:      strings.TrimSpace(item.item.ScopeType),
			ScopeID:        strings.TrimSpace(item.item.ScopeID),
			Namespace:      strings.TrimSpace(item.item.Namespace),
			MemoryType:     strings.TrimSpace(item.item.MemoryType),
			Category:       strings.TrimSpace(item.item.Category),
			CanonicalKey:   strings.TrimSpace(item.item.CanonicalKey),
			DisplayValue:   strings.TrimSpace(item.item.DisplayValue),
			Summary:        strings.TrimSpace(item.summary),
			Detail:         strings.TrimSpace(item.detail),
			KeywordMatched: item.keywordMatched,
			VectorMatched:  item.vectorMatched,
			KeywordScore:   item.keywordScore,
			VectorScore:    item.vectorScore,
			FinalScore:     item.finalScore,
			UpdateTime:     item.item.UpdateTime,
		})
	}
	return result
}

func cachedFactProjectionsToRuntime(items []CachedFactProjection) []memoryRecallProjection {
	if len(items) == 0 {
		return nil
	}
	result := make([]memoryRecallProjection, 0, len(items))
	for _, item := range items {
		result = append(result, memoryRecallProjection{
			item: domain.MemoryItem{
				ID:           strings.TrimSpace(item.MemoryID),
				ScopeType:    strings.TrimSpace(item.ScopeType),
				ScopeID:      strings.TrimSpace(item.ScopeID),
				Namespace:    strings.TrimSpace(item.Namespace),
				MemoryType:   strings.TrimSpace(item.MemoryType),
				Category:     strings.TrimSpace(item.Category),
				CanonicalKey: strings.TrimSpace(item.CanonicalKey),
				DisplayValue: strings.TrimSpace(item.DisplayValue),
				UpdateTime:   item.UpdateTime,
			},
			summary:        strings.TrimSpace(item.Summary),
			detail:         strings.TrimSpace(item.Detail),
			searchableText: normalizeRecallText(strings.TrimSpace(item.Summary) + " " + strings.TrimSpace(item.Detail)),
			keywordMatched: item.KeywordMatched,
			vectorMatched:  item.VectorMatched,
			keywordScore:   item.KeywordScore,
			vectorScore:    item.VectorScore,
			finalScore:     item.FinalScore,
		})
	}
	return result
}
