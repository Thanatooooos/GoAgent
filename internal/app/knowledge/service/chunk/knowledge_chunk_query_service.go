package chunk

import (
	"context"
	"strings"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/paging"
)

func (s *KnowledgeChunkService) Page(ctx context.Context, input PageKnowledgeChunkInput) (KnowledgeChunkPageResult, error) {
	if s == nil || s.chunkRepo == nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeChunkPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	total, err := s.chunkRepo.CountByDocumentID(ctx, documentID, input.Enabled)
	if err != nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("failed to count knowledge chunks", err)
	}
	items, err := s.chunkRepo.List(ctx, port.KnowledgeChunkListFilter{
		DocumentID: documentID,
		Enabled:    input.Enabled,
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("failed to page knowledge chunks", err)
	}
	return KnowledgeChunkPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
