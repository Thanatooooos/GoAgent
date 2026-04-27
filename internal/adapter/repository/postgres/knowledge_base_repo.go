package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeBaseRepository struct {
	db *gorm.DB
}

func NewKnowledgeBaseRepository(db *gorm.DB) *KnowledgeBaseRepository {
	return &KnowledgeBaseRepository{db: db}
}

func (r *KnowledgeBaseRepository) Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	model := toKnowledgeBaseModel(knowledgeBase)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeBase{}, fmt.Errorf("create knowledge base: %w", err)
	}
	return toKnowledgeBaseDomain(model), nil
}

func (r *KnowledgeBaseRepository) Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	rows, err := r.UpdateWhere(ctx, port.KnowledgeBaseConditions{ID: knowledgeBase.ID}, port.KnowledgeBasePatch{
		Name:           port.ValueOf(knowledgeBase.Name),
		EmbeddingModel: port.ValueOf(knowledgeBase.EmbeddingModel),
		CollectionName: port.ValueOf(knowledgeBase.CollectionName),
		UpdatedBy:      port.ValueOf(knowledgeBase.UpdatedBy),
		UpdatedAt:      port.ValueOf(knowledgeBase.UpdatedAt),
	})
	if err != nil {
		return domain.KnowledgeBase{}, err
	}
	if rows == 0 {
		return domain.KnowledgeBase{}, fmt.Errorf("update knowledge base: no rows affected")
	}
	return knowledgeBase, nil
}

func (r *KnowledgeBaseRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error) {
	updates := buildKnowledgeBaseUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}

	query := r.applyKnowledgeBaseConditions(r.db.WithContext(ctx).Model(&models.KnowledgeBaseModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge base with conditions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeBaseRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeBaseModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete knowledge base: %w", err)
	}
	return nil
}

func (r *KnowledgeBaseRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error) {
	var model models.KnowledgeBaseModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeBase{}, nil
	}
	if err != nil {
		return domain.KnowledgeBase{}, fmt.Errorf("get knowledge base by id: %w", err)
	}
	return toKnowledgeBaseDomain(model), nil
}

func (r *KnowledgeBaseRepository) GetByName(ctx context.Context, name string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.KnowledgeBaseModel{}).
		Where("name = ?", name).
		Count(&count).
		Error
	if err != nil {
		return 0, fmt.Errorf("count knowledge base by name: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeBaseRepository) Count(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error) {
	query := r.applyKnowledgeBaseListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeBaseModel{}), filter)

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count knowledge bases: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeBaseRepository) List(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error) {
	query := r.applyKnowledgeBaseListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeBaseModel{}), filter).
		Order("create_time desc")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.KnowledgeBaseModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge bases: %w", err)
	}

	result := make([]domain.KnowledgeBase, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeBaseDomain(item))
	}
	return result, nil
}

func (r *KnowledgeBaseRepository) applyKnowledgeBaseListFilter(query *gorm.DB, filter port.KnowledgeBaseListFilter) *gorm.DB {
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		query = query.Where("name ILIKE ?", like)
	}
	return query
}

func (r *KnowledgeBaseRepository) applyKnowledgeBaseConditions(query *gorm.DB, cond port.KnowledgeBaseConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.NameEQ != "" {
		query = query.Where("name = ?", cond.NameEQ)
	}
	if cond.NameNE != "" {
		query = query.Where("name <> ?", cond.NameNE)
	}
	if cond.CollectionEQ != "" {
		query = query.Where("collection_name = ?", cond.CollectionEQ)
	}
	if cond.Deleted != nil {
		query = query.Where("deleted = ?", boolToDeletedFlag(*cond.Deleted))
	}
	return query
}

func buildKnowledgeBaseUpdates(patch port.KnowledgeBasePatch) map[string]any {
	updates := map[string]any{}
	if patch.Name.Set {
		updates["name"] = patch.Name.Value
	}
	if patch.EmbeddingModel.Set {
		updates["embedding_model"] = patch.EmbeddingModel.Value
	}
	if patch.CollectionName.Set {
		updates["collection_name"] = patch.CollectionName.Value
	}
	if patch.UpdatedBy.Set {
		updates["updated_by"] = patch.UpdatedBy.Value
	}
	if patch.UpdatedAt.Set {
		updates["update_time"] = patch.UpdatedAt.Value
	}
	return updates
}
