package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	searchText := strings.TrimSpace(filter.SearchText)
	searchTokens := trimNonEmpty(filter.SearchTokens)
	if searchClause, searchArgs := buildMemorySearchClause(searchText, searchTokens); searchClause != "" {
		query = query.Where(searchClause, searchArgs...)
	}
	if sourceMessageID := strings.TrimSpace(filter.SourceMessageID); sourceMessageID != "" {
		query = query.Where("source_message_id = ?", sourceMessageID)
	}
	if supersedesID := strings.TrimSpace(filter.SupersedesID); supersedesID != "" {
		query = query.Where("supersedes_id = ?", supersedesID)
	}
	if filter.ExpiresBefore != nil && !filter.ExpiresBefore.IsZero() {
		query = query.Where("expires_at IS NOT NULL AND expires_at <= ?", *filter.ExpiresBefore)
	}
	if filter.UpdatedBefore != nil && !filter.UpdatedBefore.IsZero() {
		query = query.Where("update_time < ?", *filter.UpdatedBefore)
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

func (r *MemoryItemRepository) ListActiveByCanonicalKey(ctx context.Context, userID string, scopeType string, scopeID string, canonicalKey string) ([]domain.MemoryItem, error) {
	filter := port.MemoryItemListFilter{
		UserID:        strings.TrimSpace(userID),
		ScopeTypes:    []string{strings.TrimSpace(scopeType)},
		CanonicalKeys: []string{strings.TrimSpace(canonicalKey)},
		Statuses:      []string{domain.MemoryStatusActive},
	}
	if scopeType = strings.TrimSpace(scopeType); scopeType == domain.MemoryScopeKB {
		filter.ScopeIDs = []string{strings.TrimSpace(scopeID)}
	}
	return r.List(ctx, filter)
}

func (r *MemoryItemRepository) ListActiveSingleValueConflicts(ctx context.Context, canonicalKeys []string) ([]port.ActiveMemoryConflict, error) {
	keys := trimNonEmpty(canonicalKeys)
	if len(keys) == 0 {
		return nil, nil
	}

	type conflictRow struct {
		UserID       string `gorm:"column:user_id"`
		ScopeType    string `gorm:"column:scope_type"`
		ScopeID      string `gorm:"column:scope_id"`
		CanonicalKey string `gorm:"column:canonical_key"`
		ActiveCount  int    `gorm:"column:active_count"`
	}
	var rows []conflictRow
	if err := r.db.WithContext(ctx).
		Table("t_memory_item").
		Select("user_id, scope_type, COALESCE(scope_id, '') AS scope_id, canonical_key, COUNT(*) AS active_count").
		Where("deleted = 0").
		Where("status = ?", domain.MemoryStatusActive).
		Where("canonical_key IN ?", keys).
		Group("user_id, scope_type, COALESCE(scope_id, ''), canonical_key").
		Having("COUNT(*) > 1").
		Order("user_id, scope_type, COALESCE(scope_id, ''), canonical_key").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list active single-value memory conflicts: %w", err)
	}

	conflicts := make([]port.ActiveMemoryConflict, 0, len(rows))
	for _, row := range rows {
		conflicts = append(conflicts, port.ActiveMemoryConflict{
			UserID:       row.UserID,
			ScopeType:    row.ScopeType,
			ScopeID:      row.ScopeID,
			CanonicalKey: row.CanonicalKey,
			ActiveCount:  row.ActiveCount,
		})
	}
	return conflicts, nil
}

func (r *MemoryItemRepository) TouchLastUsed(ctx context.Context, userID string, ids []string, at time.Time) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || at.IsZero() {
		return nil
	}
	ids = trimNonEmpty(ids)
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Model(&models.MemoryItemModel{}).
		Where("user_id = ?", userID).
		Where("id IN ?", ids).
		Update("last_used_at", at).
		Error; err != nil {
		return fmt.Errorf("touch memory item last_used_at: %w", err)
	}
	return nil
}

func (r *MemoryItemRepository) ExpireByIDs(ctx context.Context, ids []string, updatedBy string, at time.Time) (int64, error) {
	ids = trimNonEmpty(ids)
	if len(ids) == 0 || at.IsZero() {
		return 0, nil
	}
	updatedBy = strings.TrimSpace(updatedBy)
	if updatedBy == "" {
		updatedBy = "memory-maintenance"
	}
	result := r.db.WithContext(ctx).
		Model(&models.MemoryItemModel{}).
		Where("id IN ?", ids).
		Where("status <> ?", domain.MemoryStatusExpired).
		Updates(map[string]any{
			"status":      domain.MemoryStatusExpired,
			"updated_by":  updatedBy,
			"update_time": at,
			"expires_at":  at,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("expire memory items by ids: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *MemoryItemRepository) DeleteByStatusesUpdatedBefore(ctx context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error) {
	statuses = trimNonEmpty(statuses)
	if len(statuses) == 0 || updatedBefore.IsZero() {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	candidateIDs := r.db.WithContext(ctx).
		Model(&models.MemoryItemModel{}).
		Select("id").
		Where("status IN ?", statuses).
		Where("update_time < ?", updatedBefore).
		Order("update_time asc").
		Limit(limit)
	result := r.db.WithContext(ctx).
		Where("id IN (?)", candidateIDs).
		Delete(&models.MemoryItemModel{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete memory items by statuses and updated_before: %w", result.Error)
	}
	return result.RowsAffected, nil
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

func buildMemorySearchClause(searchText string, searchTokens []string) (string, []any) {
	clauses := make([]string, 0, len(searchTokens)+1)
	args := make([]any, 0, (len(searchTokens)+1)*4)
	appendClause := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		pattern := "%" + term + "%"
		clauses = append(clauses, "(summary ILIKE ? OR content ILIKE ? OR display_value ILIKE ? OR canonical_key ILIKE ?)")
		args = append(args, pattern, pattern, pattern, pattern)
	}

	appendClause(searchText)
	for _, token := range searchTokens {
		appendClause(token)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}
