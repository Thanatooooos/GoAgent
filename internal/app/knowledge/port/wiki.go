package port

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
)

type WikiPageRepository interface {
	Upsert(ctx context.Context, page domain.WikiPage) (domain.WikiPage, error)
	GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error)
	ListByKB(ctx context.Context, kbID string, offset, limit int) ([]domain.WikiPage, int, error)
	DeleteByKB(ctx context.Context, kbID string) error
}

type WikiLinkRepository interface {
	CreateBatch(ctx context.Context, links []domain.WikiLink) error
	ReplaceByKBAndFrom(ctx context.Context, kbID, fromPageID string, links []domain.WikiLink) error
	ListByKB(ctx context.Context, kbID string) ([]domain.WikiLink, error)
}
