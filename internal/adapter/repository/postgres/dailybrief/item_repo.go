package dailybrief

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type ItemRepository struct {
	db *gorm.DB
}

func NewItemRepository(db *gorm.DB) *ItemRepository {
	return &ItemRepository{db: db}
}

func (r *ItemRepository) Create(ctx context.Context, item domain.Item) (domain.Item, error) {
	model := toItemModel(item)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Item{}, fmt.Errorf("create daily brief item: %w", err)
	}
	return toItemDomain(model), nil
}

func (r *ItemRepository) ReplaceIssueItems(ctx context.Context, issueID string, items []domain.Item) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("issue_id = ?", issueID).Delete(&models.ItemModel{}).Error; err != nil {
			return fmt.Errorf("delete daily brief items for issue: %w", err)
		}
		if len(items) == 0 {
			return nil
		}
		modelsBatch := make([]models.ItemModel, 0, len(items))
		for _, item := range items {
			modelsBatch = append(modelsBatch, toItemModel(item))
		}
		if err := tx.Create(&modelsBatch).Error; err != nil {
			return fmt.Errorf("create daily brief replacement items: %w", err)
		}
		return nil
	})
}

func (r *ItemRepository) List(ctx context.Context, filter port.ItemListFilter) ([]domain.Item, error) {
	query := r.db.WithContext(ctx).Model(&models.ItemModel{}).Order("section_key asc, rank asc")
	if filter.IssueID != "" {
		query = query.Where("issue_id = ?", filter.IssueID)
	}
	if filter.SectionKey != "" {
		query = query.Where("section_key = ?", filter.SectionKey)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.ItemModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list daily brief items: %w", err)
	}
	result := make([]domain.Item, 0, len(items))
	for _, item := range items {
		result = append(result, toItemDomain(item))
	}
	return result, nil
}

var _ port.ItemRepository = (*ItemRepository)(nil)
