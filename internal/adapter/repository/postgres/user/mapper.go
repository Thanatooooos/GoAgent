package user

import (
	"local/rag-project/internal/adapter/repository/postgres/user/models"
	"local/rag-project/internal/app/user/domain"
)

func toUserModel(item domain.User) models.UserModel {
	return models.UserModel{
		ID:           item.ID,
		Username:     item.Username,
		PasswordHash: item.PasswordHash,
		Role:         item.Role,
		Avatar:       item.Avatar,
		CreatedBy:    item.CreatedBy,
		UpdatedBy:    item.UpdatedBy,
		CreateTime:   item.CreatedAt,
		UpdateTime:   item.UpdatedAt,
	}
}

func toUserDomain(item models.UserModel) domain.User {
	return domain.User{
		ID:           item.ID,
		Username:     item.Username,
		PasswordHash: item.PasswordHash,
		Role:         item.Role,
		Avatar:       item.Avatar,
		CreatedBy:    item.CreatedBy,
		UpdatedBy:    item.UpdatedBy,
		CreatedAt:    item.CreateTime,
		UpdatedAt:    item.UpdateTime,
	}
}

func toUserSessionModel(item domain.UserSession) models.UserSessionModel {
	return models.UserSessionModel{
		Token:      item.Token,
		UserID:     item.UserID,
		ExpireTime: item.ExpiresAt,
		CreateTime: item.CreatedAt,
		UpdateTime: item.UpdatedAt,
	}
}

func toUserSessionDomain(item models.UserSessionModel) domain.UserSession {
	return domain.UserSession{
		Token:     item.Token,
		UserID:    item.UserID,
		ExpiresAt: item.ExpireTime,
		CreatedAt: item.CreateTime,
		UpdatedAt: item.UpdateTime,
	}
}
