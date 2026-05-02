package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"local/rag-project/internal/app/user/domain"
	"local/rag-project/internal/app/user/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/paging"
)

const (
	defaultUserPageSize = 20
	maxUserPageSize     = 100
	defaultUserRole     = "user"
	adminRole           = "admin"
	builtinAdminName    = "admin"
	minPasswordLength   = 6
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

type CreateUserInput struct {
	Username   string
	Password   string
	Role       string
	Avatar     string
	OperatorID string
}

type UpdateUserInput struct {
	ID         string
	Username   string
	Password   string
	Role       string
	Avatar     string
	OperatorID string
}

type DeleteUserInput struct {
	ID string
}

type PageUsersInput struct {
	Page     int
	PageSize int
	Keyword  string
}

type UserPageResult struct {
	Items    []domain.User
	Total    int
	Page     int
	PageSize int
}

type UserService struct {
	userRepo    port.UserRepository
	sessionRepo port.UserSessionRepository
}

func NewUserService(userRepo port.UserRepository, sessionRepo port.UserSessionRepository) *UserService {
	return &UserService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
	}
}

func (s *UserService) Create(ctx context.Context, input CreateUserInput) (domain.User, error) {
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return domain.User{}, err
	}
	if err := validatePassword(input.Password); err != nil {
		return domain.User{}, err
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.User{}, exception.NewClientException("operator id is required", nil)
	}

	role, err := normalizeRole(input.Role)
	if err != nil {
		return domain.User{}, err
	}

	existing, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return domain.User{}, exception.NewServiceException("failed to check username", err)
	}
	if existing.ID != "" {
		return domain.User{}, exception.NewClientException("username already exists", nil)
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}

	id, err := distributedid.NextID()
	if err != nil {
		return domain.User{}, exception.NewServiceException("failed to generate user id", err)
	}

	user := domain.NewUser(
		fmt.Sprintf("%d", id),
		username,
		passwordHash,
		role,
		strings.TrimSpace(input.Avatar),
		operatorID,
	)
	return s.userRepo.Create(ctx, user)
}

func (s *UserService) Update(ctx context.Context, input UpdateUserInput) (domain.User, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domain.User{}, exception.NewClientException("user id is required", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.User{}, exception.NewClientException("operator id is required", nil)
	}

	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, exception.NewServiceException("failed to get user", err)
	}
	if user.ID == "" {
		return domain.User{}, exception.NewClientException("user not found", nil)
	}
	if isBuiltinAdmin(user.Username) {
		return domain.User{}, exception.NewClientException("builtin admin cannot be modified", nil)
	}

	username, err := normalizeUsername(input.Username)
	if err != nil {
		return domain.User{}, err
	}
	if !strings.EqualFold(username, user.Username) {
		existing, err := s.userRepo.GetByUsername(ctx, username)
		if err != nil {
			return domain.User{}, exception.NewServiceException("failed to check username", err)
		}
		if existing.ID != "" && existing.ID != user.ID {
			return domain.User{}, exception.NewClientException("username already exists", nil)
		}
		user.Username = username
	}

	role, err := normalizeRole(input.Role)
	if err != nil {
		return domain.User{}, err
	}
	user.Role = role
	user.Avatar = strings.TrimSpace(input.Avatar)
	user.UpdatedBy = operatorID
	user.UpdatedAt = time.Now()

	if password := strings.TrimSpace(input.Password); password != "" {
		if err := validatePassword(password); err != nil {
			return domain.User{}, err
		}
		passwordHash, err := hashPassword(password)
		if err != nil {
			return domain.User{}, err
		}
		user.PasswordHash = passwordHash
	}

	return s.userRepo.Update(ctx, user)
}

func (s *UserService) Delete(ctx context.Context, input DeleteUserInput) error {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return exception.NewClientException("user id is required", nil)
	}

	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return exception.NewServiceException("failed to get user", err)
	}
	if user.ID == "" {
		return exception.NewClientException("user not found", nil)
	}
	if isBuiltinAdmin(user.Username) {
		return exception.NewClientException("builtin admin cannot be deleted", nil)
	}

	if err := s.userRepo.Delete(ctx, id); err != nil {
		return err
	}
	if s.sessionRepo != nil {
		if err := s.sessionRepo.DeleteByUserID(ctx, id); err != nil {
			return exception.NewServiceException("failed to clear user sessions", err)
		}
	}
	return nil
}

func (s *UserService) Page(ctx context.Context, input PageUsersInput) (UserPageResult, error) {
	page := input.Page
	page, pageSize := paging.Normalize(page, input.PageSize, defaultUserPageSize, maxUserPageSize)

	filter := port.UserListFilter{
		Keyword: strings.TrimSpace(input.Keyword),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	}

	total, err := s.userRepo.Count(ctx, filter)
	if err != nil {
		return UserPageResult{}, exception.NewServiceException("failed to count users", err)
	}
	items, err := s.userRepo.List(ctx, filter)
	if err != nil {
		return UserPageResult{}, exception.NewServiceException("failed to list users", err)
	}

	return UserPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func normalizeUsername(value string) (string, error) {
	username := strings.TrimSpace(value)
	if username == "" {
		return "", exception.NewClientException("username is required", nil)
	}
	if !usernamePattern.MatchString(username) {
		return "", exception.NewClientException("username must be 3-32 characters of letters, numbers, dot, underscore or hyphen", nil)
	}
	return username, nil
}

func normalizeRole(value string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(value))
	if role == "" {
		role = defaultUserRole
	}
	if role != defaultUserRole && role != adminRole {
		return "", exception.NewClientException("role must be admin or user", nil)
	}
	return role, nil
}

func validatePassword(value string) error {
	if len(strings.TrimSpace(value)) < minPasswordLength {
		return exception.NewClientException(fmt.Sprintf("password must be at least %d characters", minPasswordLength), nil)
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", exception.NewServiceException("failed to hash password", err)
	}
	return string(hash), nil
}

func isBuiltinAdmin(username string) bool {
	return strings.EqualFold(strings.TrimSpace(username), builtinAdminName)
}
