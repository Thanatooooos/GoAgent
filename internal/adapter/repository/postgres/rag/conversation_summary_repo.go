package rag

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

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

func (r *ConversationSummaryRepository) CreateIfCoverageAdvances(ctx context.Context, summary domain.ConversationSummary) (bool, error) {
	accepted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockKey := strings.TrimSpace(summary.ConversationID) + ":" + strings.TrimSpace(summary.UserID)
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey).Error; err != nil {
			return fmt.Errorf("lock conversation summary coverage: %w", err)
		}

		var latest models.ConversationSummaryModel
		err := tx.Where("conversation_id = ? AND user_id = ?", summary.ConversationID, summary.UserID).
			Order("id desc").
			Limit(1).
			First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load latest conversation summary coverage: %w", err)
		}
		if latest.CoveredToMessageID != "" &&
			compareDistributedIDs(latest.CoveredToMessageID, summary.CoveredToMessageID) >= 0 {
			return nil
		}

		model := toConversationSummaryModel(summary)
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("create advancing conversation summary: %w", err)
		}
		accepted = true
		return nil
	})
	return accepted, err
}

func compareDistributedIDs(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	leftInt, leftOK := new(big.Int).SetString(left, 10)
	rightInt, rightOK := new(big.Int).SetString(right, 10)
	if leftOK && rightOK {
		return leftInt.Cmp(rightInt)
	}
	return strings.Compare(left, right)
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
