package base

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

const (
	defaultKnowledgeBasePageSize = 20
	maxKnowledgeBasePageSize     = 100
)

var nonCollectionChars = regexp.MustCompile(`[^a-z0-9_]+`)

type CreateKnowledgeBaseInput struct {
	Name           string
	EmbeddingModel string
	CollectionName string
	OperatorID     string
}

type GetKnowledgeBaseInput struct {
	ID string
}

type UpdateKnowledgeBaseInput struct {
	ID             string
	Name           string
	EmbeddingModel string
	OperatorID     string
}

type DeleteKnowledgeBaseInput struct {
	ID string
}

type PageKnowledgeBaseInput struct {
	Page     int
	PageSize int
	Query    string
}

type KnowledgeBasePageResult struct {
	Items          []domain.KnowledgeBase
	DocumentCounts map[string]int
	Total          int
	Page           int
	PageSize       int
}

type KnowledgeBaseService struct {
	baseRepo     port.KnowledgeBaseRepository
	documentRepo port.KnowledgeDocumentRepository
}

type knowledgeBaseDocumentBatchCounter interface {
	CountByKnowledgeBaseIDs(ctx context.Context, knowledgeBaseIDs []string) (map[string]int, error)
}

func NewKnowledgeBaseService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
) *KnowledgeBaseService {
	return &KnowledgeBaseService{
		baseRepo:     baseRepo,
		documentRepo: documentRepo,
	}
}

func (s *KnowledgeBaseService) Create(ctx context.Context, input CreateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base name is required", nil)
	}

	embeddingModel := strings.TrimSpace(input.EmbeddingModel)
	if embeddingModel == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("embedding model is required", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("operator id is required", nil)
	}

	count, err := s.baseRepo.GetByName(ctx, name)
	if err != nil {
		return domain.KnowledgeBase{}, exception.NewServiceException("failed to check knowledge base name", err)
	}
	if count > 0 {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base name already exists", nil)
	}

	id, err := distributedid.NextID()
	if err != nil {
		return domain.KnowledgeBase{}, exception.NewServiceException("failed to generate knowledge base id", err)
	}

	collectionName := resolveCollectionName(input.CollectionName, name, id)
	knowledgeBase := domain.NewKnowledgeBase(
		fmt.Sprintf("%d", id),
		name,
		embeddingModel,
		collectionName,
		operatorID,
	)

	created, err := s.baseRepo.Create(ctx, knowledgeBase)
	if err != nil {
		return domain.KnowledgeBase{}, err
	}

	return created, nil
}

func (s *KnowledgeBaseService) Get(ctx context.Context, input GetKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base id is required", nil)
	}

	knowledgeBase, err := s.baseRepo.GetByID(ctx, id)
	if err != nil {
		return domain.KnowledgeBase{}, exception.NewServiceException("failed to get knowledge base", err)
	}
	if knowledgeBase.ID == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base not found", nil)
	}

	return knowledgeBase, nil
}

func (s *KnowledgeBaseService) Update(ctx context.Context, input UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base id is required", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("operator id is required", nil)
	}

	knowledgeBase, err := s.baseRepo.GetByID(ctx, id)
	if err != nil {
		return domain.KnowledgeBase{}, exception.NewServiceException("failed to get knowledge base to update", err)
	}
	if knowledgeBase.ID == "" {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base not found", nil)
	}

	if name := strings.TrimSpace(input.Name); name != "" && name != knowledgeBase.Name {
		count, err := s.baseRepo.GetByName(ctx, name)
		if err != nil {
			return domain.KnowledgeBase{}, exception.NewServiceException("failed to check knowledge base name", err)
		}
		if count > 0 {
			return domain.KnowledgeBase{}, exception.NewClientException("knowledge base name already exists", nil)
		}
		knowledgeBase.Name = name
	}

	if embeddingModel := strings.TrimSpace(input.EmbeddingModel); embeddingModel != "" && embeddingModel != knowledgeBase.EmbeddingModel {
		documentCount, err := s.countChunkedKnowledgeBaseDocuments(ctx, knowledgeBase.ID)
		if err != nil {
			return domain.KnowledgeBase{}, err
		}
		if documentCount > 0 {
			return domain.KnowledgeBase{}, exception.NewClientException("embedding model cannot be modified after chunked documents exist", nil)
		}
		knowledgeBase.EmbeddingModel = embeddingModel
	}

	now := time.Now()
	rows, err := s.baseRepo.UpdateWhere(ctx, port.KnowledgeBaseConditions{
		ID:      knowledgeBase.ID,
		Deleted: boolPointer(false),
	}, port.KnowledgeBasePatch{
		Name:           port.ValueOf(knowledgeBase.Name),
		EmbeddingModel: port.ValueOf(knowledgeBase.EmbeddingModel),
		CollectionName: port.ValueOf(knowledgeBase.CollectionName),
		UpdatedBy:      port.ValueOf(operatorID),
		UpdatedAt:      port.ValueOf(now),
	})
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if rows == 0 {
		return domain.KnowledgeBase{}, exception.NewClientException("knowledge base not found", nil)
	}

	knowledgeBase.UpdatedBy = operatorID
	knowledgeBase.UpdatedAt = now
	return knowledgeBase, nil
}

func (s *KnowledgeBaseService) Delete(ctx context.Context, input DeleteKnowledgeBaseInput) error {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return exception.NewClientException("knowledge base id is required", nil)
	}

	knowledgeBase, err := s.baseRepo.GetByID(ctx, id)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge base to delete", err)
	}
	if knowledgeBase.ID == "" {
		return exception.NewClientException("knowledge base not found", nil)
	}

	documentCount, err := s.countKnowledgeBaseDocuments(ctx, id)
	if err != nil {
		return err
	}
	if documentCount > 0 {
		return exception.NewClientException("knowledge base cannot be deleted while documents exist", nil)
	}

	return s.baseRepo.Delete(ctx, id)
}

func (s *KnowledgeBaseService) Page(ctx context.Context, input PageKnowledgeBaseInput) (KnowledgeBasePageResult, error) {
	page := input.Page
	if page <= 0 {
		page = 1
	}

	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = defaultKnowledgeBasePageSize
	}
	if pageSize > maxKnowledgeBasePageSize {
		pageSize = maxKnowledgeBasePageSize
	}

	filter := port.KnowledgeBaseListFilter{
		Query: strings.TrimSpace(input.Query),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}

	total, err := s.baseRepo.Count(ctx, filter)
	if err != nil {
		return KnowledgeBasePageResult{}, exception.NewServiceException("failed to count knowledge bases", err)
	}

	items, err := s.baseRepo.List(ctx, filter)
	if err != nil {
		return KnowledgeBasePageResult{}, exception.NewServiceException("failed to list knowledge bases", err)
	}

	return KnowledgeBasePageResult{
		Items:          items,
		DocumentCounts: s.countDocumentsByKnowledgeBaseIDs(ctx, items),
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

func (s *KnowledgeBaseService) countDocumentsByKnowledgeBaseIDs(ctx context.Context, items []domain.KnowledgeBase) map[string]int {
	counts := make(map[string]int, len(items))
	if s.documentRepo == nil {
		return counts
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	if batchCounter, ok := s.documentRepo.(knowledgeBaseDocumentBatchCounter); ok {
		batchCounts, err := batchCounter.CountByKnowledgeBaseIDs(ctx, ids)
		if err == nil {
			for id, count := range batchCounts {
				counts[id] = count
			}
			return counts
		}
	}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		count, err := s.documentRepo.CountByKnowledgeBaseID(ctx, item.ID)
		if err == nil {
			counts[item.ID] = count
		}
	}
	return counts
}

func (s *KnowledgeBaseService) countKnowledgeBaseDocuments(ctx context.Context, knowledgeBaseID string) (int, error) {
	if s.documentRepo == nil {
		return 0, exception.NewServiceException("knowledge document repository is required", nil)
	}

	count, err := s.documentRepo.CountByKnowledgeBaseID(ctx, knowledgeBaseID)
	if err != nil {
		return 0, exception.NewServiceException("failed to count knowledge base documents", err)
	}

	return count, nil
}

func (s *KnowledgeBaseService) countChunkedKnowledgeBaseDocuments(ctx context.Context, knowledgeBaseID string) (int, error) {
	if s.documentRepo == nil {
		return 0, exception.NewServiceException("knowledge document repository is required", nil)
	}

	count, err := s.documentRepo.CountChunkedByKnowledgeBaseID(ctx, knowledgeBaseID)
	if err != nil {
		return 0, exception.NewServiceException("failed to count chunked knowledge base documents", err)
	}

	return count, nil
}

func buildCollectionName(name string, id int64) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = nonCollectionChars.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		normalized = "kb"
	}
	return fmt.Sprintf("%s_%d", normalized, id)
}

func resolveCollectionName(collectionName, name string, id int64) string {
	collectionName = strings.TrimSpace(collectionName)
	if collectionName != "" {
		return collectionName
	}
	return buildCollectionName(name, id)
}

func boolPointer(value bool) *bool {
	return &value
}
