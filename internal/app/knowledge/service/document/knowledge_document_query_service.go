package document

import (
	"context"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/paging"
)

const (
	defaultKnowledgePageSize = 10
	maxKnowledgePageSize     = 100
)

func (s *KnowledgeDocumentService) Get(ctx context.Context, input GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s == nil || s.documentRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document id is required", nil)
	}
	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document not found", nil)
	}
	return document, nil
}

func (s *KnowledgeDocumentService) Page(ctx context.Context, input PageKnowledgeDocumentInput) (KnowledgeDocumentPageResult, error) {
	if s == nil || s.documentRepo == nil {
		return KnowledgeDocumentPageResult{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	baseFilter := port.KnowledgeDocumentListFilter{
		KnowledgeBaseID: strings.TrimSpace(input.KnowledgeBaseID),
		Status:          strings.TrimSpace(input.Status),
		Query:           strings.TrimSpace(input.Query),
	}
	total, err := s.countKnowledgeDocuments(ctx, baseFilter)
	if err != nil {
		return KnowledgeDocumentPageResult{}, err
	}

	items, err := s.documentRepo.List(ctx, port.KnowledgeDocumentListFilter{
		KnowledgeBaseID: strings.TrimSpace(input.KnowledgeBaseID),
		Status:          strings.TrimSpace(input.Status),
		Query:           strings.TrimSpace(input.Query),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeDocumentPageResult{}, exception.NewServiceException("failed to page knowledge documents", err)
	}

	return KnowledgeDocumentPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) Search(ctx context.Context, input SearchKnowledgeDocumentsInput) ([]KnowledgeDocumentSearchItem, error) {
	if s == nil || s.documentRepo == nil {
		return nil, exception.NewServiceException("knowledge document repository is required", nil)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	items, err := s.documentRepo.List(ctx, port.KnowledgeDocumentListFilter{
		Query: strings.TrimSpace(input.Query),
		ListOptions: port.ListOptions{
			Limit: limit,
		},
	})
	if err != nil {
		return nil, exception.NewServiceException("failed to search knowledge documents", err)
	}

	result := make([]KnowledgeDocumentSearchItem, 0, len(items))
	for _, item := range items {
		result = append(result, KnowledgeDocumentSearchItem{
			ID:              item.ID,
			KnowledgeBaseID: item.KnowledgeBaseID,
			Name:            item.Name,
		})
	}
	return result, nil
}

func (s *KnowledgeDocumentService) PageChunkLogs(ctx context.Context, input KnowledgeDocumentChunkLogPageInput) (KnowledgeDocumentChunkLogPageResult, error) {
	if s == nil || s.chunkLogRepo == nil {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewServiceException("knowledge document chunk log repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)
	total, err := s.countKnowledgeDocumentChunkLogs(ctx, documentID)
	if err != nil {
		return KnowledgeDocumentChunkLogPageResult{}, err
	}
	items, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{
		Offset: (page - 1) * pageSize,
		Limit:  pageSize,
	})
	if err != nil {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewServiceException("failed to page knowledge document chunk logs", err)
	}
	mapped := make([]KnowledgeDocumentChunkLogItem, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, KnowledgeDocumentChunkLogItem{Log: item})
	}
	return KnowledgeDocumentChunkLogPageResult{
		Items:    mapped,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) PageScheduleExecs(ctx context.Context, input PageKnowledgeDocumentScheduleExecInput) (KnowledgeDocumentScheduleExecPageResult, error) {
	if s == nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("knowledge document service is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	if s.scheduleService == nil || s.scheduleService.scheduleExecRepo == nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}

	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	filter := port.KnowledgeDocumentScheduleExecListFilter{
		DocumentID: documentID,
		Status:     strings.TrimSpace(input.Status),
	}
	total, err := s.countKnowledgeDocumentScheduleExecs(ctx, filter)
	if err != nil {
		return KnowledgeDocumentScheduleExecPageResult{}, err
	}

	items, err := s.scheduleService.scheduleExecRepo.List(ctx, port.KnowledgeDocumentScheduleExecListFilter{
		DocumentID: documentID,
		Status:     strings.TrimSpace(input.Status),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("failed to page knowledge document schedule execs", err)
	}

	return KnowledgeDocumentScheduleExecPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) countKnowledgeDocuments(ctx context.Context, filter port.KnowledgeDocumentListFilter) (int, error) {
	if counter, ok := s.documentRepo.(knowledgeDocumentCounter); ok {
		total, err := counter.Count(ctx, filter)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge documents", err)
		}
		return total, nil
	}
	allItems, err := s.documentRepo.List(ctx, filter)
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge documents", err)
	}
	return len(allItems), nil
}

func (s *KnowledgeDocumentService) countKnowledgeDocumentChunkLogs(ctx context.Context, documentID string) (int, error) {
	if counter, ok := s.chunkLogRepo.(knowledgeDocumentChunkLogCounter); ok {
		total, err := counter.CountByDocumentID(ctx, documentID)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge document chunk logs", err)
		}
		return total, nil
	}
	allItems, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{})
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge document chunk logs", err)
	}
	return len(allItems), nil
}

func (s *KnowledgeDocumentService) countKnowledgeDocumentScheduleExecs(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) (int, error) {
	if s == nil || s.scheduleService == nil || s.scheduleService.scheduleExecRepo == nil {
		return 0, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}
	if counter, ok := s.scheduleService.scheduleExecRepo.(knowledgeDocumentScheduleExecCounter); ok {
		total, err := counter.Count(ctx, filter)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge document schedule execs", err)
		}
		return total, nil
	}
	lister, ok := s.scheduleService.scheduleExecRepo.(knowledgeDocumentScheduleExecLister)
	if !ok {
		return 0, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}
	allItems, err := lister.List(ctx, filter)
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge document schedule execs", err)
	}
	return len(allItems), nil
}
