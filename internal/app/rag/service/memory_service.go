package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

const (
	defaultMemoryListPageSize   = 20
	maxMemoryListPageSize       = 100
	defaultMemoryRecallItems    = 6
	defaultMemoryRecallMaxChars = 1600
	defaultMemorySummaryRunes   = 120
	defaultMemoryDetailRunes    = 220
)

type SaveExplicitMemoryInput struct {
	UserID          string
	ScopeType       string
	ScopeID         string
	MemoryType      string
	SourceMessageID string
	Content         string
	Summary         string
	ExpiresAt       *time.Time
}

type ListMemoriesInput struct {
	UserID     string
	ScopeType  string
	ScopeID    string
	MemoryType string
	Status     string
	Page       int
	PageSize   int
}

type RecallMemoriesInput struct {
	UserID           string
	Query            string
	KnowledgeBaseIDs []string
}

type RecallMemoriesResult struct {
	Used               bool
	Context            string
	Items              []domain.MemoryItem
	SelectedEntries    []RecallMemoryEntry
	CandidateCount     int
	SelectedCount      int
	Truncated          bool
	ScopeCounts        map[string]int
	SourceCounts       map[string]int
	ContributionCounts map[string]int
	TypeCounts         map[string]int
	SelectedMemoryIDs  []string
}

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

type RecallMemoryEntry struct {
	ID           string
	ScopeType    string
	ScopeID      string
	MemoryType   string
	Summary      string
	Detail       string
	HitSources   []string
	KeywordScore int
	VectorScore  float32
	FinalScore   int
}

type MemoryServiceOptions struct {
	MaxRecallItems        int
	MaxRecallChars        int
	MaxCandidatesPerScope int
	DefaultListStatus     string
}

type MemoryService struct {
	repo       port.MemoryItemRepository
	now        func() time.Time
	options    MemoryServiceOptions
	retriever  ExplicitMemoryRecallService
	embedding  aiembedding.EmbeddingService
	vectorRepo port.MemoryItemEmbeddingRepository
}

func NewMemoryService(repo port.MemoryItemRepository, options MemoryServiceOptions) *MemoryService {
	if options.MaxRecallItems <= 0 {
		options.MaxRecallItems = defaultMemoryRecallItems
	}
	if options.MaxRecallChars <= 0 {
		options.MaxRecallChars = defaultMemoryRecallMaxChars
	}
	if options.MaxCandidatesPerScope <= 0 {
		options.MaxCandidatesPerScope = options.MaxRecallItems * 4
	}
	if strings.TrimSpace(options.DefaultListStatus) == "" {
		options.DefaultListStatus = domain.MemoryStatusActive
	}
	return &MemoryService{
		repo:      repo,
		now:       time.Now,
		options:   options,
		retriever: newMemoryRecallRetriever(repo, options),
	}
}

func (s *MemoryService) SaveExplicitMemory(ctx context.Context, input SaveExplicitMemoryInput) (domain.MemoryItem, error) {
	if s == nil || s.repo == nil {
		return domain.MemoryItem{}, exception.NewServiceException("memory item repository is required", nil)
	}

	userID := strings.TrimSpace(input.UserID)
	content := strings.TrimSpace(input.Content)
	scopeType := normalizeMemoryScopeType(input.ScopeType)
	memoryType := normalizeMemoryType(input.MemoryType)
	if userID == "" {
		return domain.MemoryItem{}, exception.NewClientException("user id is required", nil)
	}
	if content == "" {
		return domain.MemoryItem{}, exception.NewClientException("memory content is required", nil)
	}
	if !isSupportedMemoryScopeType(scopeType) {
		return domain.MemoryItem{}, exception.NewClientException("memory scope type must be global or kb", nil)
	}
	scopeID := strings.TrimSpace(input.ScopeID)
	if scopeType == domain.MemoryScopeKB && scopeID == "" {
		return domain.MemoryItem{}, exception.NewClientException("scope id is required for kb-scoped memory", nil)
	}
	if !isSupportedMemoryType(memoryType) {
		return domain.MemoryItem{}, exception.NewClientException("memory type must be preference, knowledge, or feedback", nil)
	}

	id, err := nextMemoryItemID()
	if err != nil {
		return domain.MemoryItem{}, err
	}
	now := s.now()
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = summarizeMemoryText(content, defaultMemorySummaryRunes)
	}

	item := domain.MemoryItem{
		ID:              id,
		UserID:          userID,
		ScopeType:       scopeType,
		ScopeID:         scopeID,
		MemoryType:      memoryType,
		SourceMessageID: strings.TrimSpace(input.SourceMessageID),
		Content:         content,
		Summary:         summary,
		Confidence:      1,
		Status:          domain.MemoryStatusActive,
		LastConfirmedAt: &now,
		ExpiresAt:       input.ExpiresAt,
		CreatedBy:       userID,
		UpdatedBy:       userID,
		CreateTime:      now,
		UpdateTime:      now,
	}
	created, err := s.repo.Create(ctx, item)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to create memory item", err)
	}
	s.persistMemoryEmbedding(ctx, created)
	return created, nil
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
		pageSize = defaultMemoryListPageSize
	}
	if pageSize > maxMemoryListPageSize {
		pageSize = maxMemoryListPageSize
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
	if memoryType := normalizeMemoryType(input.MemoryType); memoryType != "" {
		filter.MemoryTypes = []string{memoryType}
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
	item.Status = domain.MemoryStatusExpired
	item.ExpiresAt = &now
	item.UpdatedBy = userID
	item.UpdateTime = now
	updated, err := s.repo.Update(ctx, item)
	if err != nil {
		return domain.MemoryItem{}, exception.NewServiceException("failed to expire memory item", err)
	}
	return updated, nil
}

func (s *MemoryService) RecallMemories(ctx context.Context, input RecallMemoriesInput) (RecallMemoriesResult, error) {
	if s == nil || s.retriever == nil {
		return RecallMemoriesResult{}, nil
	}
	return s.retriever.RecallMemories(ctx, input)
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
	s.retriever = NewVectorAwareMemoryRecallRetriever(s.repo, repo, embedding, s.options)
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

func isSupportedMemoryScopeType(value string) bool {
	return value == domain.MemoryScopeGlobal || value == domain.MemoryScopeKB
}

func isSupportedMemoryType(value string) bool {
	return value == domain.MemoryTypePreference || value == domain.MemoryTypeKnowledge || value == domain.MemoryTypeFeedback
}

func memoryScopePriority(scopeType string) int {
	if scopeType == domain.MemoryScopeKB {
		return 1000
	}
	return 500
}

func memoryTypePriority(memoryType string) int {
	switch memoryType {
	case domain.MemoryTypePreference:
		return 300
	case domain.MemoryTypeFeedback:
		return 250
	case domain.MemoryTypeKnowledge:
		return 200
	default:
		return 0
	}
}

func nextMemoryItemID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate memory item id", err)
	}
	return fmt.Sprintf("%d", id), nil
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
	if err != nil || len(vector) == 0 {
		return
	}
	_ = s.vectorRepo.UpsertBatch(ctx, []domain.MemoryItemEmbedding{{
		MemoryItemID: strings.TrimSpace(item.ID),
		Embedding:    vector,
		CreateTime:   item.CreateTime,
		UpdateTime:   item.UpdateTime,
	}})
}

func buildMemoryEmbeddingText(item domain.MemoryItem) string {
	parts := []string{
		"scope: " + renderMemoryScopeLabel(item),
		"type: " + strings.TrimSpace(item.MemoryType),
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		parts = append(parts, "summary: "+summary)
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		parts = append(parts, "content: "+content)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func trimMemoryValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func minMemoryInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
