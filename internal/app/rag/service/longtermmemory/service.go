package longtermmemory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"local/rag-project/internal/app/rag/cachemetrics"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory/governance"
	"local/rag-project/internal/app/rag/service/longtermmemory/recall"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

// MemoryService owns long-term memory CRUD and exposes the recall capability used by chat.
type MemoryService struct {
	repo         port.MemoryItemRepository
	now          func() time.Time
	options      MemoryServiceOptions
	recall       RecallService
	recallCache  RecallCache
	cacheOptions RecallCacheOptions
	cacheMetrics *cachemetrics.Service
	mutationTx   MemoryMutationTransaction
	embedding    aiembedding.EmbeddingService
	vectorRepo   port.MemoryItemEmbeddingRepository
}

func NewMemoryService(repo port.MemoryItemRepository, options MemoryServiceOptions) *MemoryService {
	if options.MaxRecallItems <= 0 {
		options.MaxRecallItems = memorytypes.DefaultMemoryRecallItems
	}
	if options.MaxRecallChars <= 0 {
		options.MaxRecallChars = memorytypes.DefaultMemoryRecallMaxChars
	}
	if options.MaxCandidatesPerScope <= 0 {
		options.MaxCandidatesPerScope = options.MaxRecallItems * 4
	}
	if strings.TrimSpace(options.DefaultListStatus) == "" {
		options.DefaultListStatus = domain.MemoryStatusActive
	}
	return &MemoryService{
		repo:    repo,
		now:     time.Now,
		options: options,
		recall:  recall.NewService(repo, options),
	}
}

func (s *MemoryService) SaveExplicitMemory(ctx context.Context, input SaveExplicitMemoryInput) (domain.MemoryItem, error) {
	if s == nil || s.repo == nil {
		return domain.MemoryItem{}, exception.NewServiceException("memory item repository is required", nil)
	}

	var saved domain.MemoryItem
	err := s.runMemoryMutation(ctx, func(ctx context.Context, repo port.MemoryItemRepository) error {
		item, err := governance.SaveExplicitMemoryWithRepo(ctx, repo, input, s.now)
		if err != nil {
			return err
		}
		saved = item
		return nil
	})
	if err != nil {
		if resolved, ok, resolveErr := s.resolveSingleValueUniqueConflict(ctx, input, err); resolveErr != nil {
			return domain.MemoryItem{}, resolveErr
		} else if ok {
			saved = resolved
		} else {
			return domain.MemoryItem{}, err
		}
	}
	s.persistMemoryEmbedding(ctx, saved)
	s.bumpRecallCacheVersion(ctx, saved)
	return saved, nil
}

func (s *MemoryService) ListMemories(ctx context.Context, input ListMemoriesInput) ([]domain.MemoryItem, error) {
	if s == nil || s.repo == nil {
		return nil, exception.NewServiceException("memory item repository is required", nil)
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil, exception.NewClientException("user id is required", nil)
	}

	page := input.Page
	if page <= 0 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = memorytypes.DefaultMemoryListPageSize
	}
	if pageSize > memorytypes.MaxMemoryListPageSize {
		pageSize = memorytypes.MaxMemoryListPageSize
	}

	filter := port.MemoryItemListFilter{
		UserID: userID,
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}
	if scopeType := normalizeMemoryScopeType(input.ScopeType); scopeType != "" {
		filter.ScopeTypes = []string{scopeType}
	}
	if scopeID := strings.TrimSpace(input.ScopeID); scopeID != "" {
		filter.ScopeIDs = []string{scopeID}
	}
	if namespace := strings.TrimSpace(input.Namespace); namespace != "" {
		filter.Namespaces = []string{namespace}
	}
	if memoryType := normalizeMemoryType(input.MemoryType); memoryType != "" {
		filter.MemoryTypes = []string{memoryType}
	}
	if category := strings.TrimSpace(input.Category); category != "" {
		filter.Categories = []string{strings.ToLower(category)}
	}
	if canonicalKey := normalizeCanonicalKey(input.CanonicalKey); canonicalKey != "" {
		filter.CanonicalKeys = []string{canonicalKey}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = s.options.DefaultListStatus
	}
	if status != "" {
		filter.Statuses = []string{status}
	}

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, exception.NewServiceException("failed to list memory items", err)
	}
	return items, nil
}

func (s *MemoryService) ExpireMemory(ctx context.Context, userID string, id string) (domain.MemoryItem, error) {
	if s == nil || s.repo == nil {
		return domain.MemoryItem{}, exception.NewServiceException("memory item repository is required", nil)
	}
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" {
		return domain.MemoryItem{}, exception.NewClientException("user id is required", nil)
	}
	if id == "" {
		return domain.MemoryItem{}, exception.NewClientException("memory id is required", nil)
	}

	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to load memory item", err)
	}
	if item.ID == "" || strings.TrimSpace(item.UserID) != userID {
		return domain.MemoryItem{}, exception.NewClientException("memory item not found", nil)
	}

	now := s.now()
	item = governance.MarkMemoryExpired(item, userID, now)
	updated, err := s.repo.Update(ctx, item)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to expire memory item", err)
	}
	s.bumpRecallCacheVersion(ctx, updated)
	return updated, nil
}

func (s *MemoryService) RecallMemories(ctx context.Context, input RecallMemoriesInput) (RecallMemoriesResult, error) {
	if s == nil || s.recall == nil {
		return RecallMemoriesResult{}, nil
	}
	return s.recall.RecallMemories(ctx, input)
}

func (s *MemoryService) RecallService() RecallService {
	if s == nil {
		return nil
	}
	return s.recall
}

func (s *MemoryService) FactRetriever() ragretrieve.FactMemoryRetriever {
	if s == nil {
		return nil
	}
	retriever, ok := s.recall.(ragretrieve.FactMemoryRetriever)
	if !ok && s.recall != nil {
		log.Warnf("long-term memory recall service does not implement fact retriever: recallType=%T", s.recall)
		return nil
	}
	return retriever
}

func (s *MemoryService) SetMutationTransaction(tx MemoryMutationTransaction) {
	if s == nil {
		return
	}
	s.mutationTx = tx
}

func (s *MemoryService) SetEmbeddingSupport(embedding aiembedding.EmbeddingService, repo port.MemoryItemEmbeddingRepository) {
	if s == nil {
		return
	}
	s.embedding = embedding
	s.vectorRepo = repo
	if repo == nil && embedding == nil {
		return
	}
	s.recall = recall.NewVectorAwareService(s.repo, repo, embedding, s.options)
	if aware, ok := s.recall.(interface {
		SetRecallCache(cache RecallCache, options RecallCacheOptions)
	}); ok {
		aware.SetRecallCache(s.recallCache, s.cacheOptions)
	}
	if aware, ok := s.recall.(interface {
		SetCacheMetrics(metrics *cachemetrics.Service)
	}); ok {
		aware.SetCacheMetrics(s.cacheMetrics)
	}
}

func (s *MemoryService) SetRecallCache(cache RecallCache, options RecallCacheOptions) {
	if s == nil {
		return
	}
	s.recallCache = cache
	s.cacheOptions = normalizeRecallCacheOptions(options)
	if aware, ok := s.recall.(interface {
		SetRecallCache(cache RecallCache, options RecallCacheOptions)
	}); ok {
		aware.SetRecallCache(cache, s.cacheOptions)
	}
}

func (s *MemoryService) SetCacheMetrics(metrics *cachemetrics.Service) {
	if s == nil {
		return
	}
	s.cacheMetrics = metrics
	if aware, ok := s.recall.(interface {
		SetCacheMetrics(metrics *cachemetrics.Service)
	}); ok {
		aware.SetCacheMetrics(metrics)
	}
}

func (s *MemoryService) persistMemoryEmbedding(ctx context.Context, item domain.MemoryItem) {
	if s == nil || s.embedding == nil || s.vectorRepo == nil {
		return
	}
	text := buildMemoryEmbeddingText(item)
	if strings.TrimSpace(text) == "" {
		return
	}
	vector, err := s.embedding.Embed(text)
	if err != nil {
		log.Warnf("long-term memory embedding generation failed: memoryID=%s err=%v", strings.TrimSpace(item.ID), err)
		return
	}
	if len(vector) == 0 {
		log.Warnf("long-term memory embedding generation returned empty vector: memoryID=%s", strings.TrimSpace(item.ID))
		return
	}
	if err := s.vectorRepo.UpsertBatch(ctx, []domain.MemoryItemEmbedding{{
		MemoryItemID: strings.TrimSpace(item.ID),
		Embedding:    vector,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}}); err != nil {
		log.Warnf("long-term memory embedding persist failed: memoryID=%s err=%v", strings.TrimSpace(item.ID), err)
	}
}

func buildMemoryEmbeddingText(item domain.MemoryItem) string {
	parts := []string{
		"scope: " + renderMemoryScopeLabel(item),
		"type: " + strings.TrimSpace(item.MemoryType),
		"category: " + strings.TrimSpace(item.Category),
	}
	if key := strings.TrimSpace(item.CanonicalKey); key != "" {
		parts = append(parts, "key: "+key)
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "summary: "+summary)
	}
	if displayValue := strings.TrimSpace(item.DisplayValue); displayValue != "" {
		parts = append(parts, "display: "+displayValue)
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		parts = append(parts, "content: "+content)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (s *MemoryService) bumpRecallCacheVersion(ctx context.Context, item domain.MemoryItem) {
	if s == nil || s.recallCache == nil || !s.cacheOptions.Enabled {
		return
	}
	switch strings.TrimSpace(item.MemoryType) {
	case domain.MemoryTypePreference, domain.MemoryTypeKnowledge:
	default:
		return
	}
	userID := strings.TrimSpace(item.UserID)
	if userID == "" {
		return
	}
	var err error
	switch strings.TrimSpace(item.ScopeType) {
	case domain.MemoryScopeKB:
		scopeID := strings.TrimSpace(item.ScopeID)
		if scopeID == "" {
			return
		}
		err = s.recallCache.IncrKBVersion(ctx, userID, scopeID)
	default:
		err = s.recallCache.IncrGlobalVersion(ctx, userID)
	}
	if err != nil {
		log.Warnf("long-term memory cache version bump failed: userID=%s scopeType=%s scopeID=%s err=%v",
			userID,
			strings.TrimSpace(item.ScopeType),
			strings.TrimSpace(item.ScopeID),
			err,
		)
		return
	}
	if s.cacheMetrics != nil {
		s.cacheMetrics.RecordVersionInvalidation()
	}
}

func (s *MemoryService) runMemoryMutation(
	ctx context.Context,
	fn func(ctx context.Context, repo port.MemoryItemRepository) error,
) error {
	if s == nil || s.repo == nil {
		return exception.NewServiceException("memory item repository is required", nil)
	}
	if s.mutationTx != nil {
		return s.mutationTx(ctx, fn)
	}
	return fn(ctx, s.repo)
}

func (s *MemoryService) resolveSingleValueUniqueConflict(ctx context.Context, input SaveExplicitMemoryInput, mutationErr error) (domain.MemoryItem, bool, error) {
	if !isSingleValueActiveUniqueViolation(mutationErr) {
		return domain.MemoryItem{}, false, nil
	}

	normalized := governance.NormalizeSaveExplicitMemoryInput(input)
	decision, err := governance.EvaluateExplicitMemoryGate(normalized)
	if err != nil {
		return domain.MemoryItem{}, false, err
	}
	if decision.Spec == nil || decision.Spec.Cardinality != governance.MemoryCardinalitySingle || strings.TrimSpace(decision.Input.CanonicalKey) == "" {
		return domain.MemoryItem{}, false, nil
	}

	active, err := s.repo.ListActiveByCanonicalKey(
		ctx,
		normalized.UserID,
		normalized.ScopeType,
		normalized.ScopeID,
		normalized.CanonicalKey,
	)
	if err != nil {
		return domain.MemoryItem{}, true, exception.NewServiceException("failed to reload active memory items after unique conflict", err)
	}
	if len(active) > 1 {
		return domain.MemoryItem{}, true, exception.NewServiceException(
			"multiple active memory items detected for single-valued canonical key",
			nil,
		)
	}
	if len(active) == 0 {
		return domain.MemoryItem{}, true, exception.NewServiceException("single-valued memory unique conflict occurred but no active memory item was found after retry", mutationErr)
	}
	return active[0], true, nil
}

func isSingleValueActiveUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.EqualFold(strings.TrimSpace(pgErr.ConstraintName), "uk_memory_item_single_active")
	}
	return false
}

func normalizeMemoryScopeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryScopeGlobal
	}
	return value
}

func normalizeMemoryType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryTypeKnowledge
	}
	return value
}

func normalizeCanonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func renderMemoryScopeLabel(item domain.MemoryItem) string {
	scope := strings.TrimSpace(item.ScopeType)
	scopeID := strings.TrimSpace(item.ScopeID)
	if scope != "" && scopeID != "" {
		return scope + ":" + scopeID
	}
	if scope != "" {
		return scope
	}
	if scopeID != "" {
		return scopeID
	}
	return "unknown"
}
