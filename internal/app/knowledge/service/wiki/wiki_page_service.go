package wiki

import (
	"context"
	"strings"

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

// UpsertPagesFromDocument 按 slug 逐页 upsert，并把链接的 from/to slug 解析为
// 真实页面 ID 后持久化（本批页面 ID 来自 upsert 返回；批外 slug 走 ListBySlugs）。
func (s *WikiPageService) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error {
	slugToID := make(map[string]string, len(pages))
	for _, page := range pages {
		page.KnowledgeBaseID = kbID
		created, err := s.pageRepo.Upsert(ctx, page)
		if err != nil {
			return err
		}
		slugToID[created.Slug] = created.ID
	}
	if len(links) == 0 {
		return nil
	}
	resolved, err := s.resolveLinkIDs(ctx, kbID, links, slugToID)
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		return nil
	}
	byFrom := map[string][]domain.WikiLink{}
	for _, link := range resolved {
		byFrom[link.FromPageID] = append(byFrom[link.FromPageID], link)
	}
	for fromPageID, group := range byFrom {
		if err := s.linkRepo.ReplaceByKBAndFrom(ctx, kbID, fromPageID, group); err != nil {
			return err
		}
	}
	return nil
}

// resolveLinkIDs 把 links 的 from/to（slug）解析为页面 ID；无法解析的丢弃。
// 先收集批内未解析的 slug，用 ListBySlugs 一次补全，再统一解析。
func (s *WikiPageService) resolveLinkIDs(ctx context.Context, kbID string, links []domain.WikiLink, slugToID map[string]string) ([]domain.WikiLink, error) {
	var pending []string
	for _, link := range links {
		from := strings.TrimSpace(link.FromPageID)
		to := strings.TrimSpace(link.ToPageID)
		if from != "" && slugToID[from] == "" {
			pending = append(pending, from)
		}
		if to != "" && slugToID[to] == "" {
			pending = append(pending, to)
		}
	}
	if len(pending) > 0 {
		existing, err := s.pageRepo.ListBySlugs(ctx, kbID, uniqueStrings(pending))
		if err != nil {
			return nil, err
		}
		for _, page := range existing {
			slugToID[page.Slug] = page.ID
		}
	}
	result := make([]domain.WikiLink, 0, len(links))
	for _, link := range links {
		from := strings.TrimSpace(link.FromPageID)
		to := strings.TrimSpace(link.ToPageID)
		fromID := slugToID[from]
		toID := slugToID[to]
		if fromID == "" || toID == "" {
			continue // 无法解析 → 丢弃
		}
		link.KnowledgeBaseID = kbID
		link.FromPageID = fromID
		link.ToPageID = toID
		result = append(result, link)
	}
	return result, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
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

// LinkifyAndPersist 对 batch 页面做交叉链接注入，并把 extraLinks（generator 的 LLM
// 语义链接，from/to 为 slug）并入每页出链；随后无条件 ReplaceByKBAndFrom（空组清除
// 旧链接保证收敛）。返回实际持久化的链接数。
func (s *WikiPageService) LinkifyAndPersist(ctx context.Context, kbID string, pages []domain.WikiPage, extraLinks []domain.WikiLink) (int, error) {
	allPages, _, err := s.pageRepo.ListByKB(ctx, kbID, 0, 1000)
	if err != nil {
		return 0, err
	}
	builder := &WikiLinkBuilder{}
	slugToID := make(map[string]string, len(allPages))
	for _, page := range allPages {
		slugToID[page.Slug] = page.ID
	}
	extraByFrom := map[string][]domain.WikiLink{}
	for _, link := range extraLinks {
		from := strings.TrimSpace(link.FromPageID)
		if from == "" {
			continue
		}
		extraByFrom[from] = append(extraByFrom[from], link)
	}
	total := 0
	for _, page := range pages {
		page.KnowledgeBaseID = kbID
		updated, derived, linkErr := builder.LinkifyPage(ctx, kbID, page, allPages)
		if linkErr != nil {
			return 0, linkErr
		}
		merged := mergeLinks(derived, extraByFrom[page.Slug])
		persisted, err := s.pageRepo.Upsert(ctx, updated)
		if err != nil {
			return 0, err
		}
		pageID := persisted.ID
		if pageID == "" {
			continue
		}
		resolved, err := s.resolveLinkIDs(ctx, kbID, merged, slugToID)
		if err != nil {
			return 0, err
		}
		// 无条件 replace：空组清除该源旧链接（收敛）
		if err := s.linkRepo.ReplaceByKBAndFrom(ctx, kbID, pageID, resolved); err != nil {
			return 0, err
		}
		total += len(resolved)
	}
	return total, nil
}

// mergeLinks 合并派生链接与 extra 链接，按目标 slug 去重（派生优先）。
func mergeLinks(derived, extra []domain.WikiLink) []domain.WikiLink {
	seen := map[string]bool{}
	merged := make([]domain.WikiLink, 0, len(derived)+len(extra))
	for _, link := range derived {
		key := strings.TrimSpace(link.ToPageID)
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, link)
	}
	for _, link := range extra {
		key := strings.TrimSpace(link.ToPageID)
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, link)
	}
	return merged
}

// RebuildLinkCounts 从 wiki_link 重算每页 in/out 计数并写回；无链接的页面写 0。
func (s *WikiPageService) RebuildLinkCounts(ctx context.Context, kbID string) error {
	in, out, err := s.linkRepo.CountLinksByPage(ctx, kbID)
	if err != nil {
		return err
	}
	pages, _, err := s.pageRepo.ListByKB(ctx, kbID, 0, 1000)
	if err != nil {
		return err
	}
	counts := make(map[string]domain.WikiLinkCounts, len(pages))
	for _, page := range pages {
		c := counts[page.ID]
		c.In = in[page.ID]
		c.Out = out[page.ID]
		counts[page.ID] = c
	}
	return s.pageRepo.UpdateLinkCounts(ctx, kbID, counts)
}

// CleanDeadLinks 删除指向不存在页面的 wiki 链接，返回清理数。
func (s *WikiPageService) CleanDeadLinks(ctx context.Context, kbID string) (int, error) {
	pages, _, err := s.pageRepo.ListByKB(ctx, kbID, 0, 1000)
	if err != nil {
		return 0, err
	}
	validIDs := make([]string, 0, len(pages))
	for _, page := range pages {
		validIDs = append(validIDs, page.ID)
	}
	return s.linkRepo.DeleteMissingTargets(ctx, kbID, validIDs)
}
