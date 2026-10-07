package rag

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
	"time"
)

type ConversationProfileStateRepository struct{ db *gorm.DB }

func NewConversationProfileStateRepository(db *gorm.DB) *ConversationProfileStateRepository {
	return &ConversationProfileStateRepository{db: db}
}

func (r *ConversationProfileStateRepository) Defer(ctx context.Context, conversationID, userID string, due time.Time) error {
	model := models.ConversationProfileStateModel{ConversationID: conversationID, UserID: userID, Status: domain.ProfileObservationPending, NextRunAt: due, CreateTime: time.Now(), UpdateTime: time.Now()}
	return r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Assign(map[string]any{"user_id": userID, "status": domain.ProfileObservationPending, "next_run_at": due, "update_time": time.Now()}).FirstOrCreate(&model).Error
}

func (r *ConversationProfileStateRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.ConversationProfileState, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []models.ConversationProfileStateModel
	if err := r.db.WithContext(ctx).Where("status IN ? AND next_run_at <= ?", []string{domain.ProfileObservationPending, domain.ProfileObservationFailed}, now).Where("NOT EXISTS (SELECT 1 FROM t_work_conversation w WHERE w.conversation_id=t_conversation_profile_state.conversation_id AND w.user_id=t_conversation_profile_state.user_id)").Order("next_run_at asc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list due profile observations: %w", err)
	}
	result := make([]domain.ConversationProfileState, 0, len(rows))
	for _, row := range rows {
		update := r.db.WithContext(ctx).Model(&models.ConversationProfileStateModel{}).Where("conversation_id = ? AND status = ?", row.ConversationID, row.Status).Updates(map[string]any{"status": domain.ProfileObservationProcessing, "attempts": row.Attempts + 1, "update_time": now})
		if update.Error != nil {
			return nil, fmt.Errorf("claim profile observation: %w", update.Error)
		}
		if update.RowsAffected == 1 {
			result = append(result, stateDomain(row))
		}
	}
	return result, nil
}

func stateDomain(row models.ConversationProfileStateModel) domain.ConversationProfileState {
	return domain.ConversationProfileState{ConversationID: row.ConversationID, UserID: row.UserID, LastObservedMessageID: row.LastObservedMessageID, ProcessingToMessageID: row.ProcessingToMessageID, LastObservedAt: row.LastObservedAt, Status: row.Status, NextRunAt: row.NextRunAt, Attempts: row.Attempts, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
}

func (r *ConversationProfileStateRepository) SetProcessingBoundary(ctx context.Context, conversationID, messageID string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&models.ConversationProfileStateModel{}).Where("conversation_id = ? AND status = ?", conversationID, domain.ProfileObservationProcessing).Updates(map[string]any{"processing_to_message_id": messageID, "update_time": at}).Error
}

func (r *ConversationProfileStateRepository) Complete(ctx context.Context, conversationID, messageID string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&models.ConversationProfileStateModel{}).Where("conversation_id = ?", conversationID).Updates(map[string]any{"status": domain.ProfileObservationCompleted, "last_observed_message_id": messageID, "processing_to_message_id": "", "last_observed_at": at, "update_time": at}).Error
}

func (r *ConversationProfileStateRepository) Fail(ctx context.Context, conversationID string, next time.Time) error {
	return r.db.WithContext(ctx).Model(&models.ConversationProfileStateModel{}).Where("conversation_id = ?", conversationID).Updates(map[string]any{"status": domain.ProfileObservationFailed, "next_run_at": next, "update_time": time.Now()}).Error
}
