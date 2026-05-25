package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ragcache "local/rag-project/internal/app/rag/cache"
	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/log"
)

type sessionRecallConversationCache struct {
	mu       sync.Mutex
	store    *ragcache.TTLLRUCache
	baseKeys map[string]string
	fullKeys map[string]string
}

func newSessionRecallConversationCache(maxEntries int) *sessionRecallConversationCache {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	cache := &sessionRecallConversationCache{
		store:    ragcache.NewTTLLRUCache(maxEntries),
		baseKeys: make(map[string]string, maxEntries),
		fullKeys: make(map[string]string, maxEntries),
	}
	cache.store.SetOnEvict(cache.onStoreEvicted)
	return cache
}

func (c *sessionRecallConversationCache) Get(baseKey string, fullKey string) (SessionRecallResult, bool, bool) {
	if c == nil || c.store == nil || baseKey == "" || fullKey == "" {
		return SessionRecallResult{}, false, false
	}
	c.mu.Lock()
	current := c.baseKeys[baseKey]
	c.mu.Unlock()

	if current != "" && current != fullKey {
		c.deleteMapping(baseKey, current)
		c.store.Delete(current)
		return SessionRecallResult{}, false, true
	}

	value, hit := c.store.Get(fullKey)
	if !hit {
		c.deleteMapping(baseKey, fullKey)
		return SessionRecallResult{}, false, false
	}
	result, ok := value.(SessionRecallResult)
	if !ok {
		c.deleteMapping(baseKey, fullKey)
		c.store.Delete(fullKey)
		return SessionRecallResult{}, false, false
	}
	return result, true, false
}

func (c *sessionRecallConversationCache) Set(baseKey string, fullKey string, value SessionRecallResult, ttl time.Duration) bool {
	if c == nil || c.store == nil || baseKey == "" || fullKey == "" {
		return false
	}
	c.mu.Lock()
	current := c.baseKeys[baseKey]
	c.baseKeys[baseKey] = fullKey
	c.fullKeys[fullKey] = baseKey
	if current != "" && current != fullKey {
		delete(c.fullKeys, current)
	}
	c.mu.Unlock()

	if current != "" && current != fullKey {
		c.store.Delete(current)
	}
	return c.store.Set(fullKey, value, ttl)
}

func (c *sessionRecallConversationCache) deleteMapping(baseKey string, fullKey string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if current := c.baseKeys[baseKey]; current == fullKey {
		delete(c.baseKeys, baseKey)
	}
	if mappedBaseKey := c.fullKeys[fullKey]; mappedBaseKey == baseKey {
		delete(c.fullKeys, fullKey)
	}
}

func (c *sessionRecallConversationCache) onStoreEvicted(fullKey string) {
	if c == nil || fullKey == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	baseKey := c.fullKeys[fullKey]
	if baseKey != "" {
		if current := c.baseKeys[baseKey]; current == fullKey {
			delete(c.baseKeys, baseKey)
		}
		delete(c.fullKeys, fullKey)
	}
}

func (s *defaultSessionRecallService) SetCacheSupport(cache longtermmemory.RecallCache, options SessionRecallCacheOptions) {
	if s == nil {
		return
	}
	s.sharedRecallCache = cache
	s.cacheOptions = normalizeSessionRecallCacheOptions(options)
	if s.cacheOptions.ConversationScopeEnabled {
		s.conversationCache = newSessionRecallConversationCache(s.cacheOptions.ConversationMaxEntries)
	} else {
		s.conversationCache = nil
	}
}

func (s *defaultSessionRecallService) SetCacheMetrics(metrics *cachemetrics.Service) {
	if s == nil {
		return
	}
	s.cacheMetrics = metrics
}

func normalizeSessionRecallCacheOptions(options SessionRecallCacheOptions) SessionRecallCacheOptions {
	if options.ConversationMaxEntries <= 0 {
		options.ConversationMaxEntries = 1000
	}
	if options.ConversationTTL <= 0 {
		options.ConversationTTL = 10 * time.Minute
	}
	if options.EmptyResultTTL <= 0 {
		options.EmptyResultTTL = 30 * time.Second
	}
	if options.EmbeddingTTL <= 0 {
		options.EmbeddingTTL = 30 * time.Minute
	}
	return options
}

func (s *defaultSessionRecallService) canUseSessionRecallCache() bool {
	return s != nil && s.cacheOptions.Enabled
}

func (s *defaultSessionRecallService) readRecallFingerprint(ctx context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error) {
	if s == nil || s.repo == nil {
		return domain.SessionRecallFingerprint{}, nil
	}
	if reader, ok := s.repo.(interface {
		GetRecallFingerprint(ctx context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error)
	}); ok {
		return reader.GetRecallFingerprint(ctx, conversationID, userID, excludeMessageID)
	}
	exists, err := s.repo.ExistsRecallable(ctx, conversationID, userID, excludeMessageID)
	if err != nil {
		return domain.SessionRecallFingerprint{}, err
	}
	return domain.SessionRecallFingerprint{Exists: exists}, nil
}

func (s *defaultSessionRecallService) readSessionRecallRequestCache(ctx context.Context, key string) (SessionRecallResult, bool) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.RequestScopeEnabled {
		s.recordCacheMetric("session_recall", "request", "disabled")
		return SessionRecallResult{}, false
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		s.recordCacheMetric("session_recall", "request", "disabled")
		return SessionRecallResult{}, false
	}
	value, hit := cache.Get(key)
	if !hit {
		s.recordCacheMetric("session_recall", "request", "miss")
		return SessionRecallResult{}, false
	}
	result, ok := value.(SessionRecallResult)
	if !ok {
		s.recordCacheMetric("session_recall", "request", "error")
		return SessionRecallResult{}, false
	}
	s.recordCacheMetric("session_recall", "request", "hit")
	return result, true
}

func (s *defaultSessionRecallService) writeSessionRecallRequestCache(ctx context.Context, key string, result SessionRecallResult) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.RequestScopeEnabled {
		return
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	if cache.Set(key, result) {
		s.recordLocalEviction()
	}
	s.recordCacheMetric("session_recall", "request", "set")
}

func (s *defaultSessionRecallService) readConversationCache(baseKey string, fullKey string) (SessionRecallResult, bool) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.ConversationScopeEnabled || s.conversationCache == nil {
		s.recordCacheMetric("session_recall", "conversation", "disabled")
		return SessionRecallResult{}, false
	}
	result, hit, invalidated := s.conversationCache.Get(baseKey, fullKey)
	if invalidated {
		s.recordFingerprintInvalidation()
	}
	if !hit {
		s.recordCacheMetric("session_recall", "conversation", "miss")
		return SessionRecallResult{}, false
	}
	s.recordCacheMetric("session_recall", "conversation", "hit")
	return result, true
}

func (s *defaultSessionRecallService) writeConversationCache(baseKey string, fullKey string, result SessionRecallResult) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.ConversationScopeEnabled || s.conversationCache == nil {
		return
	}
	ttl := s.cacheOptions.ConversationTTL
	if !result.Used {
		ttl = s.cacheOptions.EmptyResultTTL
	}
	if s.conversationCache.Set(baseKey, fullKey, result, ttl) {
		s.recordLocalEviction()
	}
	s.recordCacheMetric("session_recall", "conversation", "set")
}

func (s *defaultSessionRecallService) writeSessionRecallCaches(ctx context.Context, baseKey string, fullKey string, result SessionRecallResult) {
	s.writeSessionRecallRequestCache(ctx, fullKey, result)
	s.writeConversationCache(baseKey, fullKey, result)
}

func (s *defaultSessionRecallService) embedQuery(ctx context.Context, query string) ([]float32, string, error) {
	if s == nil || s.embedding == nil {
		return nil, "disabled", nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "skipped", nil
	}

	requestKey := buildSharedEmbeddingCacheKey(query, s.cacheOptions.EmbeddingModel)
	if vector, hit := s.readEmbeddingRequestCache(ctx, requestKey); hit {
		return vector, "request", nil
	}

	if s.sharedRecallCache != nil && s.cacheOptions.Enabled {
		cacheKey := longtermmemory.QueryEmbeddingCacheKey{
			Query:          query,
			EmbeddingModel: strings.TrimSpace(s.cacheOptions.EmbeddingModel),
		}
		vector, hit, err := s.sharedRecallCache.GetQueryEmbedding(ctx, cacheKey)
		if err != nil {
			s.recordCacheMetric("query_embedding", "redis", "error")
			s.recordDecodeFailure(err)
			log.Warnf("session recall embedding cache get failed: model=%s err=%v", strings.TrimSpace(s.cacheOptions.EmbeddingModel), err)
		} else if hit {
			s.recordCacheMetric("query_embedding", "redis", "hit")
			s.writeEmbeddingRequestCache(ctx, requestKey, vector)
			return append([]float32(nil), vector...), "redis", nil
		}
		s.recordCacheMetric("query_embedding", "redis", "miss")

		vector, err = s.embedQueryDirect(query)
		if err != nil {
			return nil, "computed", err
		}
		if len(vector) > 0 {
			if err := s.sharedRecallCache.SetQueryEmbedding(ctx, cacheKey, vector, s.cacheOptions.EmbeddingTTL); err != nil {
				s.recordCacheMetric("query_embedding", "redis", "error")
				log.Warnf("session recall embedding cache set failed: model=%s err=%v", strings.TrimSpace(s.cacheOptions.EmbeddingModel), err)
			} else {
				s.recordCacheMetric("query_embedding", "redis", "set")
			}
		}
		s.writeEmbeddingRequestCache(ctx, requestKey, vector)
		return vector, "computed", nil
	}

	vector, err := s.embedQueryDirect(query)
	if err != nil {
		return nil, "computed", err
	}
	s.writeEmbeddingRequestCache(ctx, requestKey, vector)
	return vector, "computed", nil
}

func (s *defaultSessionRecallService) embedQueryDirect(query string) ([]float32, error) {
	modelID := strings.TrimSpace(s.cacheOptions.EmbeddingModel)
	if modelID != "" {
		return s.embedding.EmbedWithModel(query, modelID)
	}
	return s.embedding.Embed(query)
}

func (s *defaultSessionRecallService) readEmbeddingRequestCache(ctx context.Context, key string) ([]float32, bool) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.RequestScopeEnabled {
		s.recordCacheMetric("query_embedding", "request", "disabled")
		return nil, false
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		s.recordCacheMetric("query_embedding", "request", "disabled")
		return nil, false
	}
	value, hit := cache.Get(key)
	if !hit {
		s.recordCacheMetric("query_embedding", "request", "miss")
		return nil, false
	}
	vector, ok := value.([]float32)
	if !ok {
		s.recordCacheMetric("query_embedding", "request", "error")
		return nil, false
	}
	s.recordCacheMetric("query_embedding", "request", "hit")
	return append([]float32(nil), vector...), true
}

func (s *defaultSessionRecallService) writeEmbeddingRequestCache(ctx context.Context, key string, vector []float32) {
	if !s.cacheOptions.Enabled || !s.cacheOptions.RequestScopeEnabled || len(vector) == 0 {
		return
	}
	cache := ragcache.RequestCacheFromContext(ctx)
	if cache == nil {
		return
	}
	if cache.Set(key, append([]float32(nil), vector...)) {
		s.recordLocalEviction()
	}
	s.recordCacheMetric("query_embedding", "request", "set")
}

func (s *defaultSessionRecallService) recordCacheMetric(cacheKind string, layer string, outcome string) {
	if s == nil || s.cacheMetrics == nil {
		return
	}
	s.cacheMetrics.Record(cacheKind, layer, outcome)
}

func (s *defaultSessionRecallService) recordLocalEviction() {
	if s == nil || s.cacheMetrics == nil {
		return
	}
	s.cacheMetrics.RecordLocalEviction()
}

func (s *defaultSessionRecallService) recordFingerprintInvalidation() {
	if s == nil || s.cacheMetrics == nil {
		return
	}
	s.cacheMetrics.RecordFingerprintInvalidation()
}

func (s *defaultSessionRecallService) recordDecodeFailure(err error) {
	if s == nil || s.cacheMetrics == nil || err == nil {
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "unmarshal") {
		s.cacheMetrics.RecordRedisDecodeFailure()
	}
}

func buildSharedEmbeddingCacheKey(query string, modelID string) string {
	return fmt.Sprintf("embed:%s:%s", strings.TrimSpace(modelID), strings.ToLower(strings.TrimSpace(query)))
}

func buildSessionRecallFingerprintKey(fingerprint domain.SessionRecallFingerprint) string {
	if !fingerprint.Exists {
		return "none"
	}
	return fmt.Sprintf(
		"%d|%d|%s|%s",
		fingerprint.RecallableCount,
		fingerprint.LatestUpdateTime.UTC().UnixNano(),
		strings.TrimSpace(fingerprint.LatestChunkID),
		strings.TrimSpace(fingerprint.LatestMessageID),
	)
}

func buildSessionRecallBaseKey(conversationID string, userID string, query string, excludeMessageID string, options SessionRecallOptions) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%d|%d|%d|%d|%d",
		strings.TrimSpace(conversationID),
		strings.TrimSpace(userID),
		strings.ToLower(strings.TrimSpace(query)),
		strings.TrimSpace(excludeMessageID),
		options.MaxExcerpts,
		options.MaxChunksPerMessage,
		options.MaxPromptTokens,
		options.ExcerptTargetTokens,
		options.ExcerptOverlapTokens,
	)
}

func buildSessionRecallCacheKey(baseKey string, fingerprint string) string {
	return baseKey + "|" + strings.TrimSpace(fingerprint)
}
