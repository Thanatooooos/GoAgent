package knowledge

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type WikiLinkRepository struct {
	db *gorm.DB
}

func NewWikiLinkRepository(db *gorm.DB) port.WikiLinkRepository {
	return &WikiLinkRepository{db: db}
}

func (r *WikiLinkRepository) CreateBatch(ctx context.Context, links []domain.WikiLink) error {
	if len(links) == 0 {
		return nil
	}
	rows := make([]models.WikiLinkModel, 0, len(links))
	for _, link := range links {
		rows = append(rows, toWikiLinkModel(link))
	}
	if err := r.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("create wiki links: %w", err)
	}
	return nil
}

func (r *WikiLinkRepository) ReplaceByKBAndFrom(ctx context.Context, kbID, fromPageID string, links []domain.WikiLink) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("kb_id = ? AND from_page_id = ?", kbID, fromPageID).Delete(&models.WikiLinkModel{}).Error; err != nil {
			return fmt.Errorf("delete old wiki links: %w", err)
		}
		if len(links) == 0 {
			return nil
		}
		rows := make([]models.WikiLinkModel, 0, len(links))
		for _, link := range links {
			rows = append(rows, toWikiLinkModel(link))
		}
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("insert new wiki links: %w", err)
		}
		return nil
	})
}

func (r *WikiLinkRepository) ListByKB(ctx context.Context, kbID string) ([]domain.WikiLink, error) {
	var rows []models.WikiLinkModel
	if err := r.db.WithContext(ctx).Where("kb_id = ?", kbID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list wiki links: %w", err)
	}
	links := make([]domain.WikiLink, 0, len(rows))
	for _, row := range rows {
		links = append(links, toWikiLinkDomain(row))
	}
	return links, nil
}

func toWikiLinkModel(link domain.WikiLink) models.WikiLinkModel {
	return models.WikiLinkModel{
		ID:              link.ID,
		KnowledgeBaseID: link.KnowledgeBaseID,
		FromPageID:      link.FromPageID,
		ToPageID:        link.ToPageID,
		TargetType:      link.TargetType,
		Anchor:          link.Anchor,
		CreateTime:      link.CreatedAt,
	}
}

func toWikiLinkDomain(model models.WikiLinkModel) domain.WikiLink {
	return domain.WikiLink{
		ID:              model.ID,
		KnowledgeBaseID: model.KnowledgeBaseID,
		FromPageID:      model.FromPageID,
		ToPageID:        model.ToPageID,
		TargetType:      model.TargetType,
		Anchor:          model.Anchor,
		CreatedAt:       model.CreateTime,
	}
}
