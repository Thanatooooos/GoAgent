package wiki

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

// WikiPageService 负责 wiki 页面的持久化与查询。
type WikiPageService struct {
	pageRepo port.WikiPageRepository
	linkRepo port.WikiLinkRepository
}

func NewWikiPageService(pageRepo port.WikiPageRepository, linkRepo port.WikiLinkRepository) *WikiPageService {
	return &WikiPageService{pageRepo: pageRepo, linkRepo: linkRepo}
}

// UpsertPagesFromDocument 按 slug 逐页 upsert，并按源页替换链接。
// P0 中链接的 from/to 直接使用页面 slug 持久化（slug 到 page id 的映射在 P1 补充）。
func (s *WikiPageService) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error {
	for _, page := range pages {
		if _, err := s.pageRepo.Upsert(ctx, page); err != nil {
			return err
		}
	}
	if len(links) == 0 {
		return nil
	}
	byFrom := map[string][]domain.WikiLink{}
	for _, link := range links {
		byFrom[link.FromPageID] = append(byFrom[link.FromPageID], link)
	}
	for fromPageID, group := range byFrom {
		if err := s.linkRepo.ReplaceByKBAndFrom(ctx, kbID, fromPageID, group); err != nil {
			return err
		}
	}
	return nil
}

func (s *WikiPageService) GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error) {
	return s.pageRepo.GetBySlug(ctx, kbID, slug)
}

func (s *WikiPageService) ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]domain.WikiPage, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	return s.pageRepo.ListByKB(ctx, kbID, offset, pageSize)
}
