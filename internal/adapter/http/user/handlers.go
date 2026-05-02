package user

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/user/domain"
	"local/rag-project/internal/app/user/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

type AuthService interface {
	Login(ctx context.Context, input service.LoginInput) (service.LoginResult, error)
	Logout(ctx context.Context, token string) error
	GetCurrentUser(ctx context.Context, token string) (domain.User, error)
	ChangePassword(ctx context.Context, input service.ChangePasswordInput) error
}

type UserService interface {
	Create(ctx context.Context, input service.CreateUserInput) (domain.User, error)
	Update(ctx context.Context, input service.UpdateUserInput) (domain.User, error)
	Delete(ctx context.Context, input service.DeleteUserInput) error
	Page(ctx context.Context, input service.PageUsersInput) (service.UserPageResult, error)
}

type AuthHandler struct {
	service AuthService
}

type UserHandler struct {
	service UserService
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func NewUserHandler(service UserService) *UserHandler {
	return &UserHandler{service: service}
}

func RegisterUserRoutes(r gin.IRouter, authService AuthService, userService UserService) {
	authHandler := NewAuthHandler(authService)
	userHandler := NewUserHandler(userService)

	r.POST("/auth/login", authHandler.Login)

	protected := r.Group("/")
	protected.Use(middleware.RequireLogin())
	protected.POST("/auth/logout", authHandler.Logout)
	protected.GET("/user/me", authHandler.GetCurrentUser)
	protected.PUT("/user/password", authHandler.ChangePassword)

	admin := r.Group("/")
	admin.Use(middleware.RequireLogin(), middleware.RequireRole("admin"))
	admin.GET("/users", userHandler.Page)
	admin.POST("/users", userHandler.Create)
	admin.PUT("/users/:user-id", userHandler.Update)
	admin.DELETE("/users/:user-id", userHandler.Delete)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar"`
}

type updateUserRequest struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Avatar   string `json:"avatar"`
}

type userVO struct {
	UserID     string     `json:"userId,omitempty"`
	ID         string     `json:"id,omitempty"`
	Username   string     `json:"username"`
	Role       string     `json:"role"`
	Token      string     `json:"token,omitempty"`
	Avatar     string     `json:"avatar,omitempty"`
	CreateTime *time.Time `json:"createTime,omitempty"`
	UpdateTime *time.Time `json:"updateTime,omitempty"`
}

type pageResult[T any] struct {
	Records []T `json:"records"`
	Total   int `json:"total"`
	Size    int `json:"size"`
	Current int `json:"current"`
	Pages   int `json:"pages"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("auth service is required", nil))
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.service.Login(c.Request.Context(), service.LoginInput{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	writeSuccess(c, userVO{
		UserID:   result.User.ID,
		Username: result.User.Username,
		Role:     result.User.Role,
		Token:    result.Token,
		Avatar:   result.User.Avatar,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("auth service is required", nil))
		return
	}
	if err := h.service.Logout(c.Request.Context(), authToken(c)); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("auth service is required", nil))
		return
	}

	user, err := h.service.GetCurrentUser(c.Request.Context(), authToken(c))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, userVO{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Avatar:   user.Avatar,
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("auth service is required", nil))
		return
	}

	currentUser := contextx.Get(c)
	if currentUser == nil {
		_ = c.Error(exception.NewClientException("unauthorized", nil))
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), service.ChangePasswordInput{
		UserID:          currentUser.UserID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *UserHandler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("user service is required", nil))
		return
	}

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	created, err := h.service.Create(c.Request.Context(), service.CreateUserInput{
		Username:   req.Username,
		Password:   req.Password,
		Role:       req.Role,
		Avatar:     req.Avatar,
		OperatorID: operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, created.ID)
}

func (h *UserHandler) Update(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("user service is required", nil))
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	id := strings.TrimSpace(c.Param("user-id"))
	if id == "" {
		id = strings.TrimSpace(req.ID)
	}
	_, err := h.service.Update(c.Request.Context(), service.UpdateUserInput{
		ID:         id,
		Username:   req.Username,
		Password:   req.Password,
		Role:       req.Role,
		Avatar:     req.Avatar,
		OperatorID: operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *UserHandler) Delete(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("user service is required", nil))
		return
	}

	if err := h.service.Delete(c.Request.Context(), service.DeleteUserInput{ID: c.Param("user-id")}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *UserHandler) Page(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("user service is required", nil))
		return
	}

	result, err := h.service.Page(c.Request.Context(), service.PageUsersInput{
		Page:     parsePositiveInt(c.Query("current"), 1),
		PageSize: parsePositiveInt(c.Query("size"), 20),
		Keyword:  c.Query("keyword"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	records := make([]userVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toUserPageVO(item))
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	writeSuccess(c, pageResult[userVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func toUserPageVO(item domain.User) userVO {
	return userVO{
		ID:         item.ID,
		Username:   item.Username,
		Role:       item.Role,
		Avatar:     item.Avatar,
		CreateTime: timePointer(item.CreatedAt),
		UpdateTime: timePointer(item.UpdatedAt),
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func operatorID(c *gin.Context) string {
	if user := contextx.Get(c); user != nil {
		if strings.TrimSpace(user.Username) != "" {
			return strings.TrimSpace(user.Username)
		}
		if strings.TrimSpace(user.UserID) != "" {
			return strings.TrimSpace(user.UserID)
		}
	}
	return "system"
}

func authToken(c *gin.Context) string {
	if c == nil {
		return ""
	}
	token := strings.TrimSpace(c.GetHeader("Authorization"))
	return strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
}

func writeSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}
