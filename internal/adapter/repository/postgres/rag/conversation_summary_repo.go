package rag

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
)

type ConversationSummaryRepository struct {
	db *gorm.DB
}

func NewConversationSummaryRepository(db *gorm.DB) *ConversationSummaryRepository {
	return &ConversationSummaryRepository{db: db}
}

func (r *ConversationSummaryRepository) Create(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	model := toConversationSummaryModel(summary)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.ConversationSummary{}, fmt.Errorf("create conversation summary: %w", err)
	}
	return toConversationSummaryDomain(model), nil
}

func (r *ConversationSummaryRepository) FindLatestByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error) {
	var model models.ConversationSummaryModel
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Order("id desc").
		Limit(1).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ConversationSummary{}, nil
	}
	if err != nil {
		return domain.ConversationSummary{}, fmt.Errorf("find latest conversation summary: %w", err)
	}
	return toConversationSummaryDomain(model), nil
}

func (r *ConversationSummaryRepository) DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error {
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Delete(&models.ConversationSummaryModel{}).Error; err != nil {
		return fmt.Errorf("delete conversation summaries by conversation id and user id: %w", err)
	}
	return nil
}
