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

type ConversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(db *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) Create(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	model := toConversationModel(conversation)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return toConversationDomain(model), nil
}

func (r *ConversationRepository) Update(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	rows, err := r.UpdateWhere(ctx, port.ConversationConditions{ID: conversation.ID}, port.ConversationPatch{
		ConversationID: port.ValueOf(conversation.ConversationID),
		UserID:         port.ValueOf(conversation.UserID),
		Title:          port.ValueOf(conversation.Title),
		LastTime:       port.ValueOf(conversation.LastTime),
		UpdateTime:     port.ValueOf(conversation.UpdateTime),
	})
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("update conversation: %w", err)
	}
	if rows == 0 {
		return domain.Conversation{}, fmt.Errorf("update conversation: no rows affected")
	}
	return conversation, nil
}

func (r *ConversationRepository) UpdateWhere(ctx context.Context, cond port.ConversationConditions, patch port.ConversationPatch) (int64, error) {
	updates := buildConversationUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}
	if !hasConversationConditions(cond) {
		return 0, conditionalUpdateRequiresConditions("conversation")
	}

	query := applyConversationConditions(r.db.WithContext(ctx).Model(&models.ConversationModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update conversation where: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *ConversationRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.ConversationModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

func (r *ConversationRepository) GetByID(ctx context.Context, id string) (domain.Conversation, error) {
	var model models.ConversationModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Conversation{}, nil
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("get conversation by id: %w", err)
	}
	return toConversationDomain(model), nil
}

func (r *ConversationRepository) GetByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.Conversation, error) {
	var model models.ConversationModel
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Conversation{}, nil
	}
	if err != nil {
		return domain.Conversation{}, fmt.Errorf("get conversation by conversation id and user id: %w", err)
	}
	return toConversationDomain(model), nil
}

func (r *ConversationRepository) ListByUserID(ctx context.Context, userID string) ([]domain.Conversation, error) {
	var items []models.ConversationModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_time desc").
		Limit(1000).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list conversations by user id: %w", err)
	}

	result := make([]domain.Conversation, 0, len(items))
	for _, item := range items {
		result = append(result, toConversationDomain(item))
	}
	return result, nil
}

func applyConversationConditions(query *gorm.DB, cond port.ConversationConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.ConversationID != "" {
		query = query.Where("conversation_id = ?", cond.ConversationID)
	}
	if cond.UserID != "" {
		query = query.Where("user_id = ?", cond.UserID)
	}
	return query
}

func hasConversationConditions(cond port.ConversationConditions) bool {
	return cond.ID != "" || cond.ConversationID != "" || cond.UserID != ""
}

func buildConversationUpdates(patch port.ConversationPatch) map[string]any {
	updates := map[string]any{}
	if patch.ConversationID.Set {
		updates["conversation_id"] = patch.ConversationID.Value
	}
	if patch.UserID.Set {
		updates["user_id"] = patch.UserID.Value
	}
	if patch.Title.Set {
		updates["title"] = patch.Title.Value
	}
	if patch.LastTime.Set {
		updates["last_time"] = patch.LastTime.Value
	}
	if patch.UpdateTime.Set {
		updates["update_time"] = patch.UpdateTime.Value
	}
	return updates
}
