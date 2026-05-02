package rag

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type MessageFeedbackRepository struct {
	db *gorm.DB
}

func NewMessageFeedbackRepository(db *gorm.DB) *MessageFeedbackRepository {
	return &MessageFeedbackRepository{db: db}
}

func (r *MessageFeedbackRepository) Create(ctx context.Context, feedback domain.MessageFeedback) (domain.MessageFeedback, error) {
	model := toMessageFeedbackModel(feedback)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.MessageFeedback{}, fmt.Errorf("create message feedback: %w", err)
	}
	return toMessageFeedbackDomain(model), nil
}

func (r *MessageFeedbackRepository) Update(ctx context.Context, feedback domain.MessageFeedback) (domain.MessageFeedback, error) {
	rows, err := r.UpdateWhere(ctx, port.MessageFeedbackConditions{ID: feedback.ID}, port.MessageFeedbackPatch{
		MessageID:      port.ValueOf(feedback.MessageID),
		ConversationID: port.ValueOf(feedback.ConversationID),
		UserID:         port.ValueOf(feedback.UserID),
		Vote:           port.ValueOf(feedback.Vote),
		Reason:         port.ValueOf(feedback.Reason),
		Comment:        port.ValueOf(feedback.Comment),
		UpdateTime:     port.ValueOf(feedback.UpdateTime),
	})
	if err != nil {
		return domain.MessageFeedback{}, fmt.Errorf("update message feedback: %w", err)
	}
	if rows == 0 {
		return domain.MessageFeedback{}, fmt.Errorf("update message feedback: no rows affected")
	}
	return feedback, nil
}

func (r *MessageFeedbackRepository) UpdateWhere(ctx context.Context, cond port.MessageFeedbackConditions, patch port.MessageFeedbackPatch) (int64, error) {
	updates := buildMessageFeedbackUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}
	if !hasMessageFeedbackConditions(cond) {
		return 0, conditionalUpdateRequiresConditions("message feedback")
	}

	query := applyMessageFeedbackConditions(r.db.WithContext(ctx).Model(&models.MessageFeedbackModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update message feedback where: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *MessageFeedbackRepository) GetByMessageIDAndUserID(ctx context.Context, messageID string, userID string) (domain.MessageFeedback, error) {
	var model models.MessageFeedbackModel
	err := r.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ?", messageID, userID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.MessageFeedback{}, nil
	}
	if err != nil {
		return domain.MessageFeedback{}, fmt.Errorf("get message feedback by message id and user id: %w", err)
	}
	return toMessageFeedbackDomain(model), nil
}

func (r *MessageFeedbackRepository) ListByUserIDAndMessageIDs(ctx context.Context, userID string, messageIDs []string) ([]domain.MessageFeedback, error) {
	if len(messageIDs) == 0 {
		return []domain.MessageFeedback{}, nil
	}

	var items []models.MessageFeedbackModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND message_id IN ?", userID, messageIDs).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list message feedback by user id and message ids: %w", err)
	}

	result := make([]domain.MessageFeedback, 0, len(items))
	for _, item := range items {
		result = append(result, toMessageFeedbackDomain(item))
	}
	return result, nil
}

func applyMessageFeedbackConditions(query *gorm.DB, cond port.MessageFeedbackConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.MessageID != "" {
		query = query.Where("message_id = ?", cond.MessageID)
	}
	if cond.ConversationID != "" {
		query = query.Where("conversation_id = ?", cond.ConversationID)
	}
	if cond.UserID != "" {
		query = query.Where("user_id = ?", cond.UserID)
	}
	return query
}

func hasMessageFeedbackConditions(cond port.MessageFeedbackConditions) bool {
	return cond.ID != "" || cond.MessageID != "" || cond.ConversationID != "" || cond.UserID != ""
}

func buildMessageFeedbackUpdates(patch port.MessageFeedbackPatch) map[string]any {
	updates := map[string]any{}
	if patch.MessageID.Set {
		updates["message_id"] = patch.MessageID.Value
	}
	if patch.ConversationID.Set {
		updates["conversation_id"] = patch.ConversationID.Value
	}
	if patch.UserID.Set {
		updates["user_id"] = patch.UserID.Value
	}
	if patch.Vote.Set {
		updates["vote"] = int16(patch.Vote.Value)
	}
	if patch.Reason.Set {
		updates["reason"] = patch.Reason.Value
	}
	if patch.Comment.Set {
		updates["comment"] = patch.Comment.Value
	}
	if patch.UpdateTime.Set {
		updates["update_time"] = patch.UpdateTime.Value
	}
	return updates
}
