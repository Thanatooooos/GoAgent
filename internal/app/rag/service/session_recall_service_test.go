package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ragcache "local/rag-project/internal/app/rag/cache"
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

type sessionChunkRepoStub struct {
	existsFn      func(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error)
	fingerprintFn func(ctx context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error)
	searchFn      func(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error)
}

func (s sessionChunkRepoStub) CreateBatch(context.Context, []domain.SessionChunk) error {
	return nil
}

func (s sessionChunkRepoStub) ExistsRecallable(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error) {
	return s.existsFn(ctx, conversationID, userID, excludeMessageID)
}

func (s sessionChunkRepoStub) GetRecallFingerprint(ctx context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error) {
	if s.fingerprintFn != nil {
		return s.fingerprintFn(ctx, conversationID, userID, excludeMessageID)
	}
	exists, err := s.existsFn(ctx, conversationID, userID, excludeMessageID)
	return domain.SessionRecallFingerprint{Exists: exists}, err
}

func (s sessionChunkRepoStub) SearchRecallableByVector(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error) {
	return s.searchFn(ctx, conversationID, userID, excludeMessageID, vector, topK)
}

type sessionRecallEmbeddingStub struct {
	vectors    [][]float32
	embedCalls int
}

func (s *sessionRecallEmbeddingStub) Embed(text string) ([]float32, error) {
	s.embedCalls++
	if len(s.vectors) == 0 {
		return []float32{0.1, 0.2}, nil
	}
	vector := s.vectors[0]
	s.vectors = s.vectors[1:]
	return vector, nil
}

func (s *sessionRecallEmbeddingStub) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return s.Embed(text)
}

func (s *sessionRecallEmbeddingStub) EmbedBatch(texts []string) ([][]float32, error) {
	return nil, nil
}

func (s *sessionRecallEmbeddingStub) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return nil, nil
}

func (s *sessionRecallEmbeddingStub) Dimension() int {
	return 2
}

func TestSessionRecallServiceSkipsEmbeddingWhenNoRecallableChunks(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	searchCalls := 0
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return false, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				searchCalls++
				return nil, nil
			},
		},
		embedding,
		SessionRecallOptions{Enabled: true},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID:   "conv-1",
		UserID:           "user-1",
		Query:            "panic retriever timeout",
		ExcludeMessageID: "msg-current",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if result.Used {
		t.Fatalf("expected unused result, got %+v", result)
	}
	if embedding.embedCalls != 0 {
		t.Fatalf("expected no embedding call, got %d", embedding.embedCalls)
	}
	if searchCalls != 0 {
		t.Fatalf("expected no search call, got %d", searchCalls)
	}
}

func TestSessionRecallServiceBuildsContextWithPerMessageLimit(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-1", MessageID: "msg-1", ChunkIndex: 1, Content: "panic retriever timeout at line 42", ContentSummary: "chunk summary 1", TokenEstimate: 8},
						Score:        0.95,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-2", MessageID: "msg-1", ChunkIndex: 2, Content: "second detail also mentions panic retriever timeout", ContentSummary: "chunk summary 2", TokenEstimate: 9},
						Score:        0.90,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-3", MessageID: "msg-1", ChunkIndex: 3, Content: "third detail should be dropped by per-message limit", ContentSummary: "chunk summary 3", TokenEstimate: 8},
						Score:        0.85,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-4", MessageID: "msg-2", ChunkIndex: 1, Content: "another message with timeout retriever detail", ContentSummary: "chunk summary 4", TokenEstimate: 8},
						Score:        0.80,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{
			Enabled:             true,
			MaxExcerpts:         3,
			MaxChunksPerMessage: 2,
			MaxPromptTokens:     200,
			Estimator:           fixedTokenEstimator{factor: 1},
		},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if !result.Used {
		t.Fatalf("expected used result, got %+v", result)
	}
	if len(result.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d: %+v", len(result.Hits), result.Hits)
	}
	if result.Hits[0].MessageID != "msg-1" || result.Hits[1].MessageID != "msg-1" || result.Hits[2].MessageID != "msg-2" {
		t.Fatalf("unexpected hit ordering: %+v", result.Hits)
	}
	if strings.Contains(result.Context, "chunk summary 3") {
		t.Fatalf("expected third chunk from same message to be filtered out, got %q", result.Context)
	}
	if result.skippedPerMessageLimit != 1 {
		t.Fatalf("expected one hit to be skipped by per-message limit, got %+v", result)
	}
	if !strings.Contains(result.Context, "摘要：chunk summary 1") || !strings.Contains(result.Context, "原文片段：") {
		t.Fatalf("expected summary + excerpt context, got %q", result.Context)
	}
}

func TestSessionRecallServiceChoosesBestExcerptWindow(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{
							ID:             "chunk-1",
							MessageID:      "msg-1",
							ChunkIndex:     1,
							Content:        strings.Join([]string{"alpha filler", "panic retriever timeout detail", "omega filler"}, "\n"),
							ContentSummary: "target summary",
							TokenEstimate:  80,
						},
						Score: 0.91,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{
			Enabled:              true,
			MaxExcerpts:          1,
			MaxChunksPerMessage:  1,
			ExcerptTargetTokens:  24,
			ExcerptOverlapTokens: 4,
			MaxPromptTokens:      100,
			Estimator:            fixedTokenEstimator{factor: 1},
		},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected one hit, got %+v", result.Hits)
	}
	if !strings.Contains(result.Hits[0].Excerpt, "panic retriever timeout") {
		t.Fatalf("expected best excerpt window to be selected, got %q", result.Hits[0].Excerpt)
	}
	if strings.Contains(result.Hits[0].Excerpt, "alpha filler") && !strings.Contains(result.Hits[0].Excerpt, "panic retriever timeout") {
		t.Fatalf("expected lexical overlap to outrank earlier filler window, got %q", result.Hits[0].Excerpt)
	}
}

func TestSessionRecallServiceUsesConversationCacheAndInvalidatesOnFingerprintChange(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	searchCalls := 0
	fingerprint := domain.SessionRecallFingerprint{
		Exists:           true,
		RecallableCount:  1,
		LatestUpdateTime: time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		LatestChunkID:    "chunk-1",
		LatestMessageID:  "msg-1",
	}
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			fingerprintFn: func(context.Context, string, string, string) (domain.SessionRecallFingerprint, error) {
				return fingerprint, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				searchCalls++
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{ID: fingerprint.LatestChunkID, MessageID: "msg-1", ChunkIndex: 1, Content: "panic retriever timeout at line 42", ContentSummary: "chunk summary 1", TokenEstimate: 8},
						Score:        0.95,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{Enabled: true},
	)
	cacheAware := service.(interface {
		SetCacheSupport(cache longtermmemory.RecallCache, options SessionRecallCacheOptions)
		SetCacheMetrics(metrics *ragcachemetrics.Service)
	})
	cacheAware.SetCacheSupport(nil, SessionRecallCacheOptions{
		Enabled:                  true,
		RequestScopeEnabled:      true,
		ConversationScopeEnabled: true,
		ConversationMaxEntries:   8,
		ConversationTTL:          time.Minute,
		EmptyResultTTL:           30 * time.Second,
		EmbeddingTTL:             time.Minute,
	})
	metrics := ragcachemetrics.NewService()
	cacheAware.SetCacheMetrics(metrics)

	ctx1 := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result1, err := service.Recall(ctx1, SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if result1.CacheLayer != "miss" {
		t.Fatalf("expected first call to compute, got %+v", result1)
	}

	ctx2 := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result2, err := service.Recall(ctx2, SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if result2.CacheLayer != "conversation" {
		t.Fatalf("expected conversation cache hit, got %+v", result2)
	}
	if searchCalls != 1 || embedding.embedCalls != 1 {
		t.Fatalf("expected one compute before conversation hit, searchCalls=%d embedCalls=%d", searchCalls, embedding.embedCalls)
	}

	fingerprint.LatestChunkID = "chunk-2"
	fingerprint.LatestUpdateTime = fingerprint.LatestUpdateTime.Add(time.Minute)
	ctx3 := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result3, err := service.Recall(ctx3, SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if result3.CacheLayer != "miss" {
		t.Fatalf("expected cache miss after fingerprint change, got %+v", result3)
	}
	if searchCalls != 2 {
		t.Fatalf("expected recompute after fingerprint change, got %d searches", searchCalls)
	}
	if metrics.Snapshot().FingerprintInvalidations != 1 {
		t.Fatalf("expected one fingerprint invalidation, got %+v", metrics.Snapshot())
	}
}

func TestSessionRecallServiceFingerprintLookupFailureFailsOpen(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	searchCalls := 0
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			fingerprintFn: func(context.Context, string, string, string) (domain.SessionRecallFingerprint, error) {
				return domain.SessionRecallFingerprint{}, errors.New("fingerprint query failed")
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				searchCalls++
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{
							ID:             "chunk-1",
							MessageID:      "msg-1",
							ChunkIndex:     1,
							Content:        "panic retriever timeout at line 42",
							ContentSummary: "chunk summary 1",
							TokenEstimate:  8,
						},
						Score: 0.95,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{Enabled: true},
	)
	cacheAware := service.(interface {
		SetCacheSupport(cache longtermmemory.RecallCache, options SessionRecallCacheOptions)
		SetCacheMetrics(metrics *ragcachemetrics.Service)
	})
	cacheAware.SetCacheSupport(nil, SessionRecallCacheOptions{
		Enabled:                  true,
		RequestScopeEnabled:      true,
		ConversationScopeEnabled: true,
		ConversationMaxEntries:   8,
		ConversationTTL:          time.Minute,
		EmptyResultTTL:           30 * time.Second,
		EmbeddingTTL:             time.Minute,
	})
	metrics := ragcachemetrics.NewService()
	cacheAware.SetCacheMetrics(metrics)

	ctx1 := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result1, err := service.Recall(ctx1, SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if !result1.Used {
		t.Fatalf("expected fail-open recall result, got %+v", result1)
	}
	if result1.CacheLayer != "miss" {
		t.Fatalf("expected computed miss result, got %+v", result1)
	}
	if !strings.Contains(result1.RecomputeReason, "fingerprint_unavailable") {
		t.Fatalf("expected fingerprint_unavailable recompute reason, got %+v", result1)
	}

	ctx2 := ragcache.WithRequestCache(context.Background(), ragcache.NewRequestCache(16))
	result2, err := service.Recall(ctx2, SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if !result2.Used {
		t.Fatalf("expected second fail-open recall result, got %+v", result2)
	}
	if result2.CacheLayer == "conversation" || result2.CacheLayer == "request" {
		t.Fatalf("expected fingerprint failure path to bypass reusable caches, got %+v", result2)
	}
	if searchCalls != 2 || embedding.embedCalls != 2 {
		t.Fatalf("expected recompute on each call when fingerprint is unavailable, searchCalls=%d embedCalls=%d", searchCalls, embedding.embedCalls)
	}
	snapshot := metrics.Snapshot()
	foundFallback := false
	for _, event := range snapshot.Events {
		if event.CacheKind == "session_recall" && event.Layer == "conversation" && event.Outcome == "fallback" && event.Count > 0 {
			foundFallback = true
			break
		}
	}
	if !foundFallback {
		t.Fatalf("expected conversation fallback metric, got %+v", snapshot)
	}
}

func TestSessionRecallConversationCacheEvictionCleansMappings(t *testing.T) {
	cache := newSessionRecallConversationCache(1)
	first := SessionRecallResult{Used: true, CacheLayer: "miss"}
	second := SessionRecallResult{Used: true, CacheLayer: "miss"}

	if evicted := cache.Set("base-1", "base-1|fp-1", first, time.Minute); evicted {
		t.Fatalf("did not expect first insert to evict")
	}
	if len(cache.baseKeys) != 1 || len(cache.fullKeys) != 1 {
		t.Fatalf("expected first mappings to be stored, got base=%d full=%d", len(cache.baseKeys), len(cache.fullKeys))
	}

	if evicted := cache.Set("base-2", "base-2|fp-2", second, time.Minute); !evicted {
		t.Fatalf("expected second insert to evict first entry")
	}

	if _, ok := cache.baseKeys["base-1"]; ok {
		t.Fatalf("expected base-1 mapping to be removed after eviction, got %+v", cache.baseKeys)
	}
	if _, ok := cache.fullKeys["base-1|fp-1"]; ok {
		t.Fatalf("expected full key mapping to be removed after eviction, got %+v", cache.fullKeys)
	}
	if current := cache.baseKeys["base-2"]; current != "base-2|fp-2" {
		t.Fatalf("expected remaining mapping for base-2, got %+v", cache.baseKeys)
	}
}
