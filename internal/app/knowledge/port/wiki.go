package port

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
)

type WikiPageRepository interface {
	Upsert(ctx context.Context, page domain.WikiPage) (domain.WikiPage, error)
	GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error)
	ListByKB(ctx context.Context, kbID string, offset, limit int) ([]domain.WikiPage, int, error)
	ListBySlugs(ctx context.Context, kbID string, slugs []string) ([]domain.WikiPage, error)
	UpdateLinkCounts(ctx context.Context, kbID string, counts map[string]domain.WikiLinkCounts) error
	DeleteByKB(ctx context.Context, kbID string) error
	// Search 按标题/slug/摘要/content 的 ILIKE 命中打分（title>slug>summary>content），返回 top limit。
	Search(ctx context.Context, kbID, query string, limit int) ([]domain.WikiPage, error)
	// ListByIDs 批量按 ID 取页面（图邻居回填）。
	ListByIDs(ctx context.Context, kbID string, ids []string) ([]domain.WikiPage, error)
}

type WikiLinkRepository interface {
	CreateBatch(ctx context.Context, links []domain.WikiLink) error
	ReplaceByKBAndFrom(ctx context.Context, kbID, fromPageID string, links []domain.WikiLink) error
	ListByKB(ctx context.Context, kbID string) ([]domain.WikiLink, error)
	DeleteMissingTargets(ctx context.Context, kbID string, validPageIDs []string) (int, error)
	CountLinksByPage(ctx context.Context, kbID string) (in map[string]int, out map[string]int, err error)
}
