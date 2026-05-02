package user

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/user/models"
	"local/rag-project/internal/app/user/domain"
	"local/rag-project/internal/app/user/port"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	model := toUserModel(user)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return toUserDomain(model), nil
}

func (r *UserRepository) Update(ctx context.Context, user domain.User) (domain.User, error) {
	model := toUserModel(user)
	result := r.db.WithContext(ctx).
		Model(&models.UserModel{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"username":      model.Username,
			"password_hash": model.PasswordHash,
			"role":          model.Role,
			"avatar":        model.Avatar,
			"updated_by":    model.UpdatedBy,
			"update_time":   model.UpdateTime,
		})
	if result.Error != nil {
		return domain.User{}, fmt.Errorf("update user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.User{}, fmt.Errorf("update user: no rows affected")
	}
	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.UserModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	var model models.UserModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, nil
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return toUserDomain(model), nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	var model models.UserModel
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, nil
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by username: %w", err)
	}
	return toUserDomain(model), nil
}

func (r *UserRepository) Count(ctx context.Context, filter port.UserListFilter) (int, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.UserModel{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return int(count), nil
}

func (r *UserRepository) List(ctx context.Context, filter port.UserListFilter) ([]domain.User, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.UserModel{}), filter).
		Order("create_time desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.UserModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	result := make([]domain.User, 0, len(items))
	for _, item := range items {
		result = append(result, toUserDomain(item))
	}
	return result, nil
}

func (r *UserRepository) applyFilter(query *gorm.DB, filter port.UserListFilter) *gorm.DB {
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		query = query.Where("username ILIKE ? OR role ILIKE ?", like, like)
	}
	return query
}
