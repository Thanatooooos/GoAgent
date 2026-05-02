package user

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/user/models"
	"local/rag-project/internal/app/user/domain"
)

type UserSessionRepository struct {
	db *gorm.DB
}

func NewUserSessionRepository(db *gorm.DB) *UserSessionRepository {
	return &UserSessionRepository{db: db}
}

func (r *UserSessionRepository) Create(ctx context.Context, session domain.UserSession) (domain.UserSession, error) {
	model := toUserSessionModel(session)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.UserSession{}, fmt.Errorf("create user session: %w", err)
	}
	return toUserSessionDomain(model), nil
}

func (r *UserSessionRepository) GetByToken(ctx context.Context, token string) (domain.UserSession, error) {
	var model models.UserSessionModel
	err := r.db.WithContext(ctx).Where("token = ?", token).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.UserSession{}, nil
	}
	if err != nil {
		return domain.UserSession{}, fmt.Errorf("get user session by token: %w", err)
	}
	return toUserSessionDomain(model), nil
}

func (r *UserSessionRepository) DeleteByToken(ctx context.Context, token string) error {
	if err := r.db.WithContext(ctx).Delete(&models.UserSessionModel{}, "token = ?", token).Error; err != nil {
		return fmt.Errorf("delete user session by token: %w", err)
	}
	return nil
}

func (r *UserSessionRepository) DeleteByUserID(ctx context.Context, userID string) error {
	if err := r.db.WithContext(ctx).Delete(&models.UserSessionModel{}, "user_id = ?", userID).Error; err != nil {
		return fmt.Errorf("delete user sessions by user id: %w", err)
	}
	return nil
}
