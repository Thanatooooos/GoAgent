package recall

import (
	"context"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type memoryRecallProjection struct {
	item           domain.MemoryItem
	summary        string
	detail         string
	searchableText string
	keywordMatched bool
	vectorMatched  bool
	keywordScore   int
	vectorScore    float32
	finalScore     int
}

type recallService struct {
	repo          port.MemoryItemRepository
	embeddingRepo port.MemoryItemEmbeddingRepository
	embedding     aiembedding.EmbeddingService
	options       memorytypes.MemoryServiceOptions
	cache         port.MemoryRecallCache
	cacheOptions  memorytypes.RecallCacheOptions
	cacheMetrics  *cachemetrics.Service
	now           func() time.Time
}

func NewService(repo port.MemoryItemRepository, options memorytypes.MemoryServiceOptions) memorytypes.RecallService {
	return NewVectorAwareService(repo, nil, nil, options)
}

func NewVectorAwareService(
	repo port.MemoryItemRepository,
	embeddingRepo port.MemoryItemEmbeddingRepository,
	embedding aiembedding.EmbeddingService,
	options memorytypes.MemoryServiceOptions,
) memorytypes.RecallService {
	if options.MaxRecallItems <= 0 {
		options.MaxRecallItems = memorytypes.DefaultMemoryRecallItems
	}
	if options.MaxRecallChars <= 0 {
		options.MaxRecallChars = memorytypes.DefaultMemoryRecallMaxChars
	}
	if options.MaxCandidatesPerScope <= 0 {
		options.MaxCandidatesPerScope = options.MaxRecallItems * 4
	}
	return &recallService{
		repo:          repo,
		embeddingRepo: embeddingRepo,
		embedding:     embedding,
		options:       options,
		now:           time.Now,
	}
}

func (r *recallService) RecallMemories(ctx context.Context, input memorytypes.RecallMemoriesInput) (memorytypes.RecallMemoriesResult, error) {
	if r == nil || r.repo == nil {
		return memorytypes.RecallMemoriesResult{}, nil
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return memorytypes.RecallMemoriesResult{}, exception.NewClientException("user id is required", nil)
	}

	query := strings.TrimSpace(input.Query)
	knowledgeBaseIDs := trimMemoryValues(input.KnowledgeBaseIDs)
	scopeTypes := trimMemoryValues(input.ScopeTypes)
	memoryTypes := trimMemoryValues(input.MemoryTypes)
	statuses := normalizeRecallStatuses(input.Statuses)
	longtermmemoryobs.LogRecallStarted(ctx, userID, query, scopeTypes, memoryTypes, statuses, len(knowledgeBaseIDs))

	includeRules := len(memoryTypes) == 0 || containsMemoryValue(memoryTypes, domain.MemoryTypePreference)
	includeFacts := len(memoryTypes) == 0 || containsMemoryValue(memoryTypes, domain.MemoryTypeKnowledge)

	var (
		ruleCandidates      []memoryRecallProjection
		scopeVersions       port.ScopeVersions
		ruleCacheLayer      string
		ruleReason          string
		rankedFacts         []memoryRecallProjection
		factScopeVersions   port.ScopeVersions
		factCacheLayer      string
		embeddingCacheLayer string
		factReason          string
		err                 error
	)
	if includeRules {
		ruleCandidates, scopeVersions, ruleCacheLayer, ruleReason, err = r.loadRuleMemoryProjections(ctx, userID, query, knowledgeBaseIDs, scopeTypes, statuses)
		if err != nil {
			longtermmemoryobs.LogRecallFailed(ctx, userID, "load_rule_memory_projections", err)
			return memorytypes.RecallMemoriesResult{}, err
		}
	}

	if includeFacts {
		rankedFacts, _, factScopeVersions, factCacheLayer, embeddingCacheLayer, factReason, err = r.loadFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, r.options.MaxCandidatesPerScope)
		if err != nil {
			longtermmemoryobs.LogRecallFailed(ctx, userID, "load_fact_ranking_projections", err)
			return memorytypes.RecallMemoriesResult{}, err
		}
	}
	if scopeVersions.GlobalVersion == 0 && len(scopeVersions.KBVersions) == 0 {
		scopeVersions = factScopeVersions
	}

	selectedRules, selectedFacts, contextText, truncated := buildMemoryRecallContext(ruleCandidates, rankedFacts, r.options.MaxRecallItems, r.options.MaxRecallChars)
	selected := append(append([]memoryRecallProjection(nil), selectedRules...), selectedFacts...)
	if len(selectedRules) > 0 {
		longtermmemoryobs.Record(r.cacheMetrics, longtermmemoryobs.LayerRecall, longtermmemoryobs.OutcomePreferenceRecalled)
	}
	r.touchLastUsed(ctx, userID, selected)
	recomputeReason := strings.TrimSpace(strings.Join([]string{ruleReason, factReason}, ";"))
	recomputeReason = strings.Trim(recomputeReason, "; ")
	longtermmemoryobs.LogRecallCompleted(
		ctx,
		userID,
		len(ruleCandidates)+len(rankedFacts),
		len(selected),
		len(selectedRules),
		len(selectedFacts),
		ruleCacheLayer,
		factCacheLayer,
		embeddingCacheLayer,
		truncated,
	)

	return memorytypes.RecallMemoriesResult{
		Used:                len(selected) > 0,
		Context:             contextText,
		Items:               projectedMemoryItems(selected),
		SelectedEntries:     projectedMemoryEntries(selected),
		CandidateCount:      len(ruleCandidates) + len(rankedFacts),
		SelectedCount:       len(selected),
		RuleCount:           len(selectedRules),
		FactCandidateCount:  len(rankedFacts),
		FactSelectedCount:   len(selectedFacts),
		Truncated:           truncated,
		ScopeCounts:         projectedScopeCounts(selected),
		SourceCounts:        projectedSourceCounts(selected),
		ContributionCounts:  projectedContributionCounts(selected),
		TypeCounts:          projectedTypeCounts(selected),
		SelectedMemoryIDs:   projectedMemoryIDs(selected),
		RuleMemoryIDs:       projectedMemoryIDs(selectedRules),
		FactMemoryIDs:       projectedMemoryIDs(selectedFacts),
		CacheEnabled:        r.canUseRecallCache() || r.cacheOptions.RequestScopeEnabled,
		RuleCacheLayer:      ruleCacheLayer,
		FactCacheLayer:      factCacheLayer,
		EmbeddingCacheLayer: embeddingCacheLayer,
		ScopeVersions:       scopeVersions,
		RecomputeReason:     recomputeReason,
	}, nil
}

func (r *recallService) loadRuleMemories(ctx context.Context, userID string, knowledgeBaseIDs []string, scopeTypes []string, statuses []string) ([]domain.MemoryItem, error) {
	scopeTypes = trimMemoryValues(scopeTypes)
	statuses = normalizeRecallStatuses(statuses)
	var kbItems []domain.MemoryItem
	var err error
	if len(knowledgeBaseIDs) > 0 && (len(scopeTypes) == 0 || containsMemoryValue(scopeTypes, domain.MemoryScopeKB)) {
		kbItems, err = r.repo.List(ctx, port.MemoryItemListFilter{
			UserID:      userID,
			ScopeTypes:  []string{domain.MemoryScopeKB},
			ScopeIDs:    knowledgeBaseIDs,
			MemoryTypes: []string{domain.MemoryTypePreference},
			Statuses:    statuses,
			ListOptions: port.ListOptions{
				Limit: r.options.MaxCandidatesPerScope,
			},
		})
		if err != nil {
			return nil, exception.NewServiceException("failed to list kb rule memory items", err)
		}
	}
	var globalItems []domain.MemoryItem
	if len(scopeTypes) == 0 || containsMemoryValue(scopeTypes, domain.MemoryScopeGlobal) {
		globalItems, err = r.repo.List(ctx, port.MemoryItemListFilter{
			UserID:      userID,
			ScopeTypes:  []string{domain.MemoryScopeGlobal},
			MemoryTypes: []string{domain.MemoryTypePreference},
			Statuses:    statuses,
			ListOptions: port.ListOptions{
				Limit: r.options.MaxCandidatesPerScope,
			},
		})
		if err != nil {
			return nil, exception.NewServiceException("failed to list global rule memory items", err)
		}
	}
	items := append(append([]domain.MemoryItem(nil), kbItems...), globalItems...)
	sortRuleMemoryItems(items)
	return items, nil
}

func (r *recallService) loadFactMemoryCandidates(ctx context.Context, userID string, query string, knowledgeBaseIDs []string) ([]domain.MemoryItem, map[string]float32, string, error) {
	return r.loadFactMemoryCandidatesWithLimit(ctx, userID, query, knowledgeBaseIDs, r.options.MaxCandidatesPerScope)
}

func (r *recallService) touchLastUsed(ctx context.Context, userID string, selected []memoryRecallProjection) {
	if r == nil || r.repo == nil || len(selected) == 0 {
		return
	}
	ids := projectedMemoryIDs(selected)
	if len(ids) == 0 {
		return
	}
	at := time.Now()
	if r.now != nil {
		at = r.now()
	}
	if err := r.repo.TouchLastUsed(ctx, userID, ids, at); err != nil {
		if r.cacheMetrics != nil {
			r.cacheMetrics.RecordTouchLastUsedFailure()
		}
		log.Warnf("long-term memory touch last_used_at failed: userID=%s ids=%v err=%v", userID, ids, err)
	}
}
