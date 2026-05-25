package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/log"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

var sessionRecallASCIITokenPattern = regexp.MustCompile(`[A-Za-z0-9_./:-]+`)

type SessionRecallInput struct {
	ConversationID   string
	UserID           string
	Query            string
	ExcludeMessageID string
}

type SessionRecallHit struct {
	MessageID     string
	ChunkIndex    int
	Summary       string
	Excerpt       string
	Score         float32
	TokenEstimate int
	ExcerptTokens int
	SourceChunkID string
}

type SessionRecallResult struct {
	Used                bool
	Hits                []SessionRecallHit
	Context             string
	TopScore            float32
	CacheEnabled        bool
	CacheLayer          string
	RecallFingerprint   string
	EmbeddingCacheLayer string
	RecomputeReason     string

	candidateCount         int
	skippedPerMessageLimit int
	truncatedBy            string
}

type SessionRecallService interface {
	Recall(ctx context.Context, input SessionRecallInput) (SessionRecallResult, error)
}

type SessionRecallOptions struct {
	Enabled              bool
	MaxExcerpts          int
	MaxChunksPerMessage  int
	ExcerptTargetTokens  int
	ExcerptOverlapTokens int
	MaxPromptTokens      int
	Estimator            TokenEstimator
}

type SessionRecallCacheOptions struct {
	Enabled                  bool
	RequestScopeEnabled      bool
	ConversationScopeEnabled bool
	ConversationMaxEntries   int
	ConversationTTL          time.Duration
	EmptyResultTTL           time.Duration
	EmbeddingTTL             time.Duration
	EmbeddingModel           string
}

type defaultSessionRecallService struct {
	repo              port.SessionChunkRepository
	embedding         aiembedding.EmbeddingService
	options           SessionRecallOptions
	cacheOptions      SessionRecallCacheOptions
	conversationCache *sessionRecallConversationCache
	sharedRecallCache longtermmemory.RecallCache
	cacheMetrics      *cachemetrics.Service
}

func NewSessionRecallService(repo port.SessionChunkRepository, embedding aiembedding.EmbeddingService, options SessionRecallOptions) SessionRecallService {
	if options.MaxExcerpts <= 0 {
		options.MaxExcerpts = 3
	}
	if options.MaxChunksPerMessage <= 0 {
		options.MaxChunksPerMessage = 2
	}
	if options.ExcerptTargetTokens <= 0 {
		options.ExcerptTargetTokens = 500
	}
	if options.ExcerptOverlapTokens < 0 {
		options.ExcerptOverlapTokens = 0
	}
	if options.MaxPromptTokens <= 0 {
		options.MaxPromptTokens = 1500
	}
	if options.Estimator == nil {
		options.Estimator = RoughTokenEstimator{}
	}
	return &defaultSessionRecallService{
		repo:      repo,
		embedding: embedding,
		options:   options,
	}
}

func (s *defaultSessionRecallService) Recall(ctx context.Context, input SessionRecallInput) (SessionRecallResult, error) {
	if s == nil || !s.options.Enabled {
		return SessionRecallResult{}, nil
	}
	if s.repo == nil {
		return SessionRecallResult{}, fmt.Errorf("session chunk repository is required")
	}
	if s.embedding == nil {
		return SessionRecallResult{}, fmt.Errorf("embedding service is required")
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	query := strings.TrimSpace(input.Query)
	excludeMessageID := strings.TrimSpace(input.ExcludeMessageID)
	if conversationID == "" || userID == "" || query == "" {
		return SessionRecallResult{}, nil
	}

	fingerprint, err := s.readRecallFingerprint(ctx, conversationID, userID, excludeMessageID)
	fingerprintAvailable := err == nil
	if err != nil {
		log.Warnf("session recall fingerprint lookup failed: conversationID=%s userID=%s err=%v", conversationID, userID, err)
		s.recordCacheMetric("session_recall", "conversation", "fallback")
	}
	fingerprintKey := buildSessionRecallFingerprintKey(fingerprint)
	baseKey := buildSessionRecallBaseKey(conversationID, userID, query, excludeMessageID, s.options)
	fullKey := buildSessionRecallCacheKey(baseKey, fingerprintKey)
	if fingerprintAvailable && !fingerprint.Exists {
		result := SessionRecallResult{
			CacheEnabled:      s.canUseSessionRecallCache(),
			CacheLayer:        "miss",
			RecallFingerprint: fingerprintKey,
			RecomputeReason:   "no_recallable_chunks",
		}
		s.writeSessionRecallCaches(ctx, baseKey, fullKey, result)
		return result, nil
	}

	if fingerprintAvailable {
		if result, hit := s.readSessionRecallRequestCache(ctx, fullKey); hit {
			result.CacheEnabled = s.canUseSessionRecallCache()
			result.CacheLayer = "request"
			result.RecallFingerprint = fingerprintKey
			return result, nil
		}

		if result, hit := s.readConversationCache(baseKey, fullKey); hit {
			result.CacheEnabled = s.canUseSessionRecallCache()
			result.CacheLayer = "conversation"
			result.RecallFingerprint = fingerprintKey
			s.writeSessionRecallRequestCache(ctx, fullKey, result)
			return result, nil
		}
	}

	vector, embeddingLayer, err := s.embedQuery(ctx, query)
	if err != nil {
		return SessionRecallResult{}, fmt.Errorf("embed session recall query: %w", err)
	}

	searchTopK := maxInt(s.options.MaxExcerpts*2, 6)
	candidates, err := s.repo.SearchRecallableByVector(ctx, conversationID, userID, excludeMessageID, vector, searchTopK)
	if err != nil {
		return SessionRecallResult{}, err
	}
	if len(candidates) == 0 {
		result := SessionRecallResult{
			CacheEnabled:        s.canUseSessionRecallCache(),
			CacheLayer:          "miss",
			RecallFingerprint:   fingerprintKey,
			EmbeddingCacheLayer: embeddingLayer,
			RecomputeReason:     sessionRecallRecomputeReason(fingerprintAvailable, "candidate_search_empty"),
		}
		if fingerprintAvailable {
			s.writeSessionRecallCaches(ctx, baseKey, fullKey, result)
		}
		return result, nil
	}

	hits, selection := s.buildHits(query, candidates)
	if len(hits) == 0 {
		result := SessionRecallResult{
			CacheEnabled:           s.canUseSessionRecallCache(),
			CacheLayer:             "miss",
			RecallFingerprint:      fingerprintKey,
			EmbeddingCacheLayer:    embeddingLayer,
			RecomputeReason:        sessionRecallRecomputeReason(fingerprintAvailable, selection.truncatedBy),
			TopScore:               candidates[0].Score,
			candidateCount:         len(candidates),
			skippedPerMessageLimit: selection.skippedPerMessageLimit,
			truncatedBy:            selection.truncatedBy,
		}
		if fingerprintAvailable {
			s.writeSessionRecallCaches(ctx, baseKey, fullKey, result)
		}
		return result, nil
	}

	result := SessionRecallResult{
		Used:                   true,
		Hits:                   hits,
		Context:                buildSessionRecallContext(hits),
		TopScore:               candidates[0].Score,
		CacheEnabled:           s.canUseSessionRecallCache(),
		CacheLayer:             "miss",
		RecallFingerprint:      fingerprintKey,
		EmbeddingCacheLayer:    embeddingLayer,
		RecomputeReason:        sessionRecallRecomputeReason(fingerprintAvailable, "conversation_cache_miss"),
		candidateCount:         len(candidates),
		skippedPerMessageLimit: selection.skippedPerMessageLimit,
		truncatedBy:            selection.truncatedBy,
	}
	if fingerprintAvailable {
		s.writeSessionRecallCaches(ctx, baseKey, fullKey, result)
	}
	return result, nil
}

func sessionRecallRecomputeReason(fingerprintAvailable bool, reason string) string {
	reason = strings.TrimSpace(reason)
	if fingerprintAvailable {
		return reason
	}
	if reason == "" {
		return "fingerprint_unavailable"
	}
	return "fingerprint_unavailable;" + reason
}

func (s *defaultSessionRecallService) buildHits(query string, candidates []domain.SessionChunkSearchHit) ([]SessionRecallHit, SessionRecallResult) {
	perMessageCount := make(map[string]int)
	selected := make([]SessionRecallHit, 0, minInt(len(candidates), s.options.MaxExcerpts))
	totalTokens := 0
	selection := SessionRecallResult{}

	for _, candidate := range candidates {
		messageID := strings.TrimSpace(candidate.MessageID)
		if messageID == "" {
			continue
		}
		if perMessageCount[messageID] >= s.options.MaxChunksPerMessage {
			selection.skippedPerMessageLimit++
			continue
		}

		excerpt, excerptTokens := s.selectExcerpt(query, candidate)
		if strings.TrimSpace(excerpt) == "" {
			continue
		}
		if len(selected) >= s.options.MaxExcerpts {
			selection.truncatedBy = "max_excerpts"
			break
		}
		if totalTokens+excerptTokens > s.options.MaxPromptTokens {
			selection.truncatedBy = "max_prompt_tokens"
			break
		}

		summary := strings.TrimSpace(candidate.ContentSummary)
		if summary == "" {
			summary = truncateRunes(strings.TrimSpace(excerpt), 160)
		}
		selected = append(selected, SessionRecallHit{
			MessageID:     messageID,
			ChunkIndex:    candidate.ChunkIndex,
			Summary:       summary,
			Excerpt:       excerpt,
			Score:         candidate.Score,
			TokenEstimate: candidate.TokenEstimate,
			ExcerptTokens: excerptTokens,
			SourceChunkID: strings.TrimSpace(candidate.ID),
		})
		perMessageCount[messageID]++
		totalTokens += excerptTokens
	}

	return selected, selection
}

func (s *defaultSessionRecallService) selectExcerpt(query string, candidate domain.SessionChunkSearchHit) (string, int) {
	content := strings.TrimSpace(candidate.Content)
	if content == "" {
		return "", 0
	}

	tokenEstimate := candidate.TokenEstimate
	if tokenEstimate <= 0 {
		tokenEstimate = s.options.Estimator.EstimateTokens(content)
	}
	if tokenEstimate <= s.options.ExcerptTargetTokens {
		return content, maxInt(tokenEstimate, 1)
	}

	windows := splitTextByTokenBudget(content, s.options.ExcerptTargetTokens, s.options.ExcerptOverlapTokens, s.options.Estimator)
	if len(windows) == 0 {
		return content, maxInt(tokenEstimate, 1)
	}

	bestIndex := 0
	bestScore := -1
	for idx, window := range windows {
		score := scoreSessionRecallWindow(query, window)
		if score > bestScore {
			bestScore = score
			bestIndex = idx
		}
	}

	excerpt := strings.TrimSpace(windows[bestIndex])
	return excerpt, maxInt(s.options.Estimator.EstimateTokens(excerpt), 1)
}

func buildSessionRecallContext(hits []SessionRecallHit) string {
	if len(hits) == 0 {
		return ""
	}

	var builder strings.Builder
	for idx, hit := range hits {
		if idx > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("[%d] 来源消息 ID=%s，分段=%d\n", idx+1, strings.TrimSpace(hit.MessageID), hit.ChunkIndex))
		builder.WriteString("摘要：")
		builder.WriteString(strings.TrimSpace(hit.Summary))
		builder.WriteString("\n原文片段：\n")
		builder.WriteString(strings.TrimSpace(hit.Excerpt))
	}
	return strings.TrimSpace(builder.String())
}

func scoreSessionRecallWindow(query string, window string) int {
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	lowerWindow := strings.ToLower(strings.TrimSpace(window))
	if lowerQuery == "" || lowerWindow == "" {
		return 0
	}

	score := 0
	seenASCII := make(map[string]struct{})
	for _, token := range sessionRecallASCIITokenPattern.FindAllString(lowerQuery, -1) {
		token = strings.TrimSpace(token)
		if len(token) < 2 {
			continue
		}
		if _, ok := seenASCII[token]; ok {
			continue
		}
		seenASCII[token] = struct{}{}
		if strings.Contains(lowerWindow, token) {
			score++
		}
	}

	queryCompact := compactLowerString(lowerQuery)
	windowCompact := compactLowerString(lowerWindow)
	if containsCJKString(queryCompact) {
		seenBigrams := make(map[string]struct{})
		for _, bigram := range buildDistinctCJKBigrams(queryCompact) {
			if _, ok := seenBigrams[bigram]; ok {
				continue
			}
			seenBigrams[bigram] = struct{}{}
			if strings.Contains(windowCompact, bigram) {
				score++
			}
		}
	}

	return score
}

func compactLowerString(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func containsCJKString(value string) bool {
	for _, r := range value {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

func buildDistinctCJKBigrams(value string) []string {
	runes := []rune(value)
	if len(runes) < 2 {
		return nil
	}
	result := make([]string, 0, len(runes)-1)
	seen := make(map[string]struct{})
	for i := 0; i < len(runes)-1; i++ {
		bigram := string(runes[i : i+2])
		if _, ok := seen[bigram]; ok {
			continue
		}
		seen[bigram] = struct{}{}
		result = append(result, bigram)
	}
	return result
}
