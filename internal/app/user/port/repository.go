package port

import (
	"context"

	"local/rag-project/internal/app/user/domain"
)

type ListOptions struct {
	Offset int
	Limit  int
}

type UserListFilter struct {
	Keyword string
	ListOptions
}

type UserRepository interface {
	Create(ctx context.Context, user domain.User) (domain.User, error)
	Update(ctx context.Context, user domain.User) (domain.User, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.User, error)
	GetByUsername(ctx context.Context, username string) (domain.User, error)
	Count(ctx context.Context, filter UserListFilter) (int, error)
	List(ctx context.Context, filter UserListFilter) ([]domain.User, error)
}

type UserSessionRepository interface {
	Create(ctx context.Context, session domain.UserSession) (domain.UserSession, error)
	GetByToken(ctx context.Context, token string) (domain.UserSession, error)
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID string) error
}
