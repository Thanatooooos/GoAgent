package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"local/rag-project/internal/app/user/domain"
	"local/rag-project/internal/app/user/port"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/exception"
)

type LoginInput struct {
	Username string
	Password string
}

type LoginResult struct {
	User  domain.User
	Token string
}

type ChangePasswordInput struct {
	UserID          string
	CurrentPassword string
	NewPassword     string
}

type AuthService struct {
	userRepo     port.UserRepository
	sessionRepo  port.UserSessionRepository
	sessionTTL   time.Duration
	isConcurrent bool
}

func NewAuthService(
	userRepo port.UserRepository,
	sessionRepo port.UserSessionRepository,
	sessionTTL time.Duration,
	isConcurrent bool,
) *AuthService {
	if sessionTTL <= 0 {
		sessionTTL = 30 * 24 * time.Hour
	}
	return &AuthService{
		userRepo:     userRepo,
		sessionRepo:  sessionRepo,
		sessionTTL:   sessionTTL,
		isConcurrent: isConcurrent,
	}
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)
	if username == "" || password == "" {
		return LoginResult{}, exception.NewClientException("username and password are required", nil)
	}

	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return LoginResult{}, exception.NewServiceException("failed to get user", err)
	}
	if user.ID == "" {
		return LoginResult{}, exception.NewClientException("invalid username or password", nil)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, exception.NewClientException("invalid username or password", nil)
	}

	if !s.isConcurrent {
		if err := s.sessionRepo.DeleteByUserID(ctx, user.ID); err != nil {
			return LoginResult{}, exception.NewServiceException("failed to reset user sessions", err)
		}
	}

	token := uuid.NewString()
	session := domain.NewUserSession(token, user.ID, time.Now().Add(s.sessionTTL))
	if _, err := s.sessionRepo.Create(ctx, session); err != nil {
		return LoginResult{}, exception.NewServiceException("failed to create login session", err)
	}

	return LoginResult{
		User:  user,
		Token: token,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	token = normalizeToken(token)
	if token == "" {
		return nil
	}
	if err := s.sessionRepo.DeleteByToken(ctx, token); err != nil {
		return exception.NewServiceException("failed to logout", err)
	}
	return nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, token string) (domain.User, error) {
	_, user, err := s.resolveSessionUser(ctx, token)
	if err != nil {
		return domain.User{}, err
	}
	if user.ID == "" {
		return domain.User{}, exception.NewClientException("unauthorized", nil)
	}
	return user, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, input ChangePasswordInput) error {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	if strings.TrimSpace(input.CurrentPassword) == "" {
		return exception.NewClientException("current password is required", nil)
	}
	if err := validatePassword(input.NewPassword); err != nil {
		return err
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return exception.NewServiceException("failed to get current user", err)
	}
	if user.ID == "" {
		return exception.NewClientException("user not found", nil)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.CurrentPassword)); err != nil {
		return exception.NewClientException("current password is incorrect", nil)
	}

	passwordHash, err := hashPassword(input.NewPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = passwordHash
	user.UpdatedBy = user.Username
	user.UpdatedAt = time.Now()
	if _, err := s.userRepo.Update(ctx, user); err != nil {
		return err
	}
	return nil
}

func (s *AuthService) LoadLoginUser(ctx context.Context, token string) (*contextx.LoginUser, error) {
	_, user, err := s.resolveSessionUser(ctx, token)
	if err != nil {
		var appErr interface{ ErrorCode() string }
		if errors.As(err, &appErr) && strings.HasPrefix(appErr.ErrorCode(), "A") {
			return nil, nil
		}
		return nil, err
	}
	if user.ID == "" {
		return nil, nil
	}
	return &contextx.LoginUser{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Avatar:   user.Avatar,
	}, nil
}

func (s *AuthService) resolveSessionUser(ctx context.Context, token string) (domain.UserSession, domain.User, error) {
	token = normalizeToken(token)
	if token == "" {
		return domain.UserSession{}, domain.User{}, nil
	}

	session, err := s.sessionRepo.GetByToken(ctx, token)
	if err != nil {
		return domain.UserSession{}, domain.User{}, exception.NewServiceException("failed to get session", err)
	}
	if session.Token == "" {
		return domain.UserSession{}, domain.User{}, nil
	}
	if !session.ExpiresAt.IsZero() && session.ExpiresAt.Before(time.Now()) {
		_ = s.sessionRepo.DeleteByToken(ctx, token)
		return domain.UserSession{}, domain.User{}, nil
	}

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return domain.UserSession{}, domain.User{}, exception.NewServiceException("failed to get user by session", err)
	}
	if user.ID == "" {
		_ = s.sessionRepo.DeleteByToken(ctx, token)
		return domain.UserSession{}, domain.User{}, nil
	}
	return session, user, nil
}

func normalizeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	return strings.TrimSpace(token)
}
