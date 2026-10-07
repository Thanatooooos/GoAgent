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
)

type UserMemoryProfileRepository struct{ db *gorm.DB }

func NewUserMemoryProfileRepository(db *gorm.DB) *UserMemoryProfileRepository {
	return &UserMemoryProfileRepository{db: db}
}

func (r *UserMemoryProfileRepository) Get(ctx context.Context, userID string) (domain.UserMemoryProfile, error) {
	var model models.UserMemoryProfileModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.UserMemoryProfile{UserID: userID}, nil
	}
	if err != nil {
		return domain.UserMemoryProfile{}, fmt.Errorf("get user memory profile: %w", err)
	}
	return domain.UserMemoryProfile{UserID: model.UserID, ContentMarkdown: model.ContentMarkdown, Version: model.Version, LastObservedAt: model.LastObservedAt, CreateTime: model.CreateTime, UpdateTime: model.UpdateTime}, nil
}

func (r *UserMemoryProfileRepository) Save(ctx context.Context, profile domain.UserMemoryProfile) (domain.UserMemoryProfile, error) {
	profile.UserID = strings.TrimSpace(profile.UserID)
	if profile.UserID == "" {
		return domain.UserMemoryProfile{}, fmt.Errorf("user id is required")
	}
	now := time.Now()
	model := models.UserMemoryProfileModel{UserID: profile.UserID, ContentMarkdown: strings.TrimSpace(profile.ContentMarkdown), Version: profile.Version, LastObservedAt: profile.LastObservedAt, CreateTime: now, UpdateTime: now}
	if model.Version <= 0 {
		model.Version = 1
	}
	if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
		return domain.UserMemoryProfile{}, fmt.Errorf("save user memory profile: %w", err)
	}
	return r.Get(ctx, profile.UserID)
}
