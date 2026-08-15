package chunk

import (
	"context"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
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
	})
	if err != nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("failed to page knowledge chunks", err)
	}
	groups := groupKnowledgeChunks(items)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(groups) {
		start = len(groups)
	}
	if end > len(groups) {
		end = len(groups)
	}
	pageGroups := groups[start:end]
	pageItems := flattenKnowledgeChunkGroups(pageGroups)
	return KnowledgeChunkPageResult{
		Items:       pageItems,
		Groups:      pageGroups,
		Total:       len(groups),
		RecordTotal: total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func groupKnowledgeChunks(items []domain.KnowledgeChunk) []KnowledgeChunkGroup {
	groups := make([]KnowledgeChunkGroup, 0, len(items))
	parentGroupIndex := make(map[string]int)

	for _, item := range items {
		if item.RecordType != "parent" {
			continue
		}
		parentGroupIndex[item.ID] = len(groups)
		groups = append(groups, KnowledgeChunkGroup{Parent: item})
	}

	for _, item := range items {
		if item.RecordType == "parent" {
			continue
		}
		if index, ok := parentGroupIndex[item.ParentChunkID]; ok {
			groups[index].Children = append(groups[index].Children, item)
			continue
		}
		groups = append(groups, KnowledgeChunkGroup{Parent: item})
	}
	return groups
}

func flattenKnowledgeChunkGroups(groups []KnowledgeChunkGroup) []domain.KnowledgeChunk {
	items := make([]domain.KnowledgeChunk, 0)
	for _, group := range groups {
		items = append(items, group.Parent)
		items = append(items, group.Children...)
	}
	return items
}

func (s *KnowledgeChunkService) GetByID(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error) {
	if s == nil || s.chunkRepo == nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return domain.KnowledgeChunk{}, exception.NewClientException("knowledge chunk id is required", nil)
	}
	chunk, err := s.chunkRepo.GetByID(ctx, chunkID)
	if err != nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("failed to get knowledge chunk", err)
	}
	if chunk.ID == "" {
		return domain.KnowledgeChunk{}, exception.NewClientException("knowledge chunk not found", nil)
	}
	return chunk, nil
}
