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

type ConversationMessageRepository struct {
	db *gorm.DB
}

func NewConversationMessageRepository(db *gorm.DB) *ConversationMessageRepository {
	return &ConversationMessageRepository{db: db}
}

func (r *ConversationMessageRepository) Create(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
	model := toConversationMessageModel(message)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.ConversationMessage{}, fmt.Errorf("create conversation message: %w", err)
	}
	return toConversationMessageDomain(model), nil
}

func (r *ConversationMessageRepository) GetByID(ctx context.Context, id string) (domain.ConversationMessage, error) {
	var model models.ConversationMessageModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ConversationMessage{}, nil
	}
	if err != nil {
		return domain.ConversationMessage{}, fmt.Errorf("get conversation message by id: %w", err)
	}
	return toConversationMessageDomain(model), nil
}

func (r *ConversationMessageRepository) List(ctx context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	query := r.db.WithContext(ctx).Model(&models.ConversationMessageModel{})
	if filter.ConversationID != "" {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if len(filter.Roles) > 0 {
		roles := make([]string, 0, len(filter.Roles))
		for _, role := range filter.Roles {
			role = strings.TrimSpace(role)
			if role != "" {
				roles = append(roles, role)
			}
		}
		if len(roles) > 0 {
			query = query.Where("role IN ?", roles)
		}
	}
	if filter.AfterID != "" {
		query = query.Where("id > ?", filter.AfterID)
	}
	if filter.BeforeID != "" {
		query = query.Where("id < ?", filter.BeforeID)
	}

	switch filter.Order {
	case port.ConversationMessageOrderDesc:
		query = query.Order("create_time desc")
	default:
		query = query.Order("create_time asc")
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	} else {
		query = query.Limit(500)
	}

	var items []models.ConversationMessageModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list conversation messages: %w", err)
	}

	result := make([]domain.ConversationMessage, 0, len(items))
	for _, item := range items {
		result = append(result, toConversationMessageDomain(item))
	}
	return result, nil
}

func (r *ConversationMessageRepository) CountByConversationIDAndUserIDAndRole(ctx context.Context, conversationID string, userID string, role string) (int64, error) {
	query := r.db.WithContext(ctx).Model(&models.ConversationMessageModel{})
	if conversationID != "" {
		query = query.Where("conversation_id = ?", conversationID)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if role != "" {
		query = query.Where("role = ?", role)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count conversation messages: %w", err)
	}
	return count, nil
}

func (r *ConversationMessageRepository) FindMaxIDAtOrBefore(ctx context.Context, conversationID string, userID string, at time.Time) (string, error) {
	var model models.ConversationMessageModel
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ? AND create_time <= ?", conversationID, userID, at).
		Order("id desc").
		Limit(1).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find max conversation message id at or before time: %w", err)
	}
	return model.ID, nil
}

func (r *ConversationMessageRepository) DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error {
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Delete(&models.ConversationMessageModel{}).Error; err != nil {
		return fmt.Errorf("delete conversation messages by conversation id and user id: %w", err)
	}
	return nil
}
