package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type MemoryItemRepository struct {
	db *gorm.DB
}

func NewMemoryItemRepository(db *gorm.DB) *MemoryItemRepository {
	return &MemoryItemRepository{db: db}
}

func (r *MemoryItemRepository) Create(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	model := toMemoryItemModel(item)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.MemoryItem{}, fmt.Errorf("create memory item: %w", err)
	}
	return toMemoryItemDomain(model), nil
}

func (r *MemoryItemRepository) Update(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	model := toMemoryItemModel(item)
	if err := r.db.WithContext(ctx).Model(&models.MemoryItemModel{}).Where("id = ?", item.ID).Updates(&model).Error; err != nil {
		return domain.MemoryItem{}, fmt.Errorf("update memory item: %w", err)
	}
	return r.GetByID(ctx, item.ID)
}

func (r *MemoryItemRepository) GetByID(ctx context.Context, id string) (domain.MemoryItem, error) {
	var model models.MemoryItemModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.MemoryItem{}, nil
	}
	if err != nil {
		return domain.MemoryItem{}, fmt.Errorf("get memory item by id: %w", err)
	}
	return toMemoryItemDomain(model), nil
}

func (r *MemoryItemRepository) List(ctx context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
	query := r.db.WithContext(ctx).Model(&models.MemoryItemModel{})
	if userID := strings.TrimSpace(filter.UserID); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if values := trimNonEmpty(filter.ScopeTypes); len(values) > 0 {
		query = query.Where("scope_type IN ?", values)
	}
	if values := trimNonEmpty(filter.ScopeIDs); len(values) > 0 {
		query = query.Where("scope_id IN ?", values)
	}
	if values := trimNonEmpty(filter.Namespaces); len(values) > 0 {
		query = query.Where("namespace IN ?", values)
	}
	if values := trimNonEmpty(filter.MemoryTypes); len(values) > 0 {
		query = query.Where("memory_type IN ?", values)
	}
	if values := trimNonEmpty(filter.Categories); len(values) > 0 {
		query = query.Where("category IN ?", values)
	}
	if values := trimNonEmpty(filter.CanonicalKeys); len(values) > 0 {
		query = query.Where("canonical_key IN ?", values)
	}
	if values := trimNonEmpty(filter.Statuses); len(values) > 0 {
		query = query.Where("status IN ?", values)
	}
	if sourceMessageID := strings.TrimSpace(filter.SourceMessageID); sourceMessageID != "" {
		query = query.Where("source_message_id = ?", sourceMessageID)
	}
	if supersedesID := strings.TrimSpace(filter.SupersedesID); supersedesID != "" {
		query = query.Where("supersedes_id = ?", supersedesID)
	}
	query = query.Order("update_time desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.MemoryItemModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list memory items: %w", err)
	}
	result := make([]domain.MemoryItem, 0, len(items))
	for _, item := range items {
		result = append(result, toMemoryItemDomain(item))
	}
	return result, nil
}

func trimNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
