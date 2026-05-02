package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	userhttp "local/rag-project/internal/adapter/http/user"
	"local/rag-project/internal/app/user/domain"
	"local/rag-project/internal/app/user/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type authServiceStub struct {
	loginFn          func(ctx context.Context, input service.LoginInput) (service.LoginResult, error)
	logoutFn         func(ctx context.Context, token string) error
	getCurrentUserFn func(ctx context.Context, token string) (domain.User, error)
	changePasswordFn func(ctx context.Context, input service.ChangePasswordInput) error
}

func (s authServiceStub) Login(ctx context.Context, input service.LoginInput) (service.LoginResult, error) {
	if s.loginFn != nil {
		return s.loginFn(ctx, input)
	}
	return service.LoginResult{}, nil
}

func (s authServiceStub) Logout(ctx context.Context, token string) error {
	if s.logoutFn != nil {
		return s.logoutFn(ctx, token)
	}
	return nil
}

func (s authServiceStub) GetCurrentUser(ctx context.Context, token string) (domain.User, error) {
	if s.getCurrentUserFn != nil {
		return s.getCurrentUserFn(ctx, token)
	}
	return domain.User{}, nil
}

func (s authServiceStub) ChangePassword(ctx context.Context, input service.ChangePasswordInput) error {
	if s.changePasswordFn != nil {
		return s.changePasswordFn(ctx, input)
	}
	return nil
}

type userServiceStub struct {
	createFn func(ctx context.Context, input service.CreateUserInput) (domain.User, error)
	updateFn func(ctx context.Context, input service.UpdateUserInput) (domain.User, error)
	deleteFn func(ctx context.Context, input service.DeleteUserInput) error
	pageFn   func(ctx context.Context, input service.PageUsersInput) (service.UserPageResult, error)
}

func (s userServiceStub) Create(ctx context.Context, input service.CreateUserInput) (domain.User, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return domain.User{}, nil
}

func (s userServiceStub) Update(ctx context.Context, input service.UpdateUserInput) (domain.User, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, input)
	}
	return domain.User{}, nil
}

func (s userServiceStub) Delete(ctx context.Context, input service.DeleteUserInput) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, input)
	}
	return nil
}

func (s userServiceStub) Page(ctx context.Context, input service.PageUsersInput) (service.UserPageResult, error) {
	if s.pageFn != nil {
		return s.pageFn(ctx, input)
	}
	return service.UserPageResult{}, nil
}

func TestAuthHandlerLoginMatchesContract(t *testing.T) {
	router := newUserRouter(authServiceStub{
		loginFn: func(ctx context.Context, input service.LoginInput) (service.LoginResult, error) {
			if input.Username != "admin" || input.Password != "admin123" {
				t.Fatalf("unexpected login input: %+v", input)
			}
			return service.LoginResult{
				User: domain.User{
					ID:       "1",
					Username: "admin",
					Role:     "admin",
					Avatar:   "",
				},
				Token: "token-1",
			}, nil
		},
	}, userServiceStub{}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			UserID   string `json:"userId"`
			Username string `json:"username"`
			Role     string `json:"role"`
			Token    string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.UserID != "1" || result.Data.Username != "admin" || result.Data.Role != "admin" || result.Data.Token != "token-1" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestUserHandlerPageMatchesIPageShape(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	router := newUserRouter(authServiceStub{}, userServiceStub{
		pageFn: func(ctx context.Context, input service.PageUsersInput) (service.UserPageResult, error) {
			if input.Page != 2 || input.PageSize != 5 || input.Keyword != "adm" {
				t.Fatalf("unexpected page input: %+v", input)
			}
			return service.UserPageResult{
				Items: []domain.User{{
					ID:        "1",
					Username:  "admin",
					Role:      "admin",
					CreatedAt: now,
					UpdatedAt: now,
				}},
				Total:    7,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	}, true)

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/users?current=2&size=5&keyword=adm", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Records []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"records"`
			Total   int `json:"total"`
			Size    int `json:"size"`
			Current int `json:"current"`
			Pages   int `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.Total != 7 || result.Data.Size != 5 || result.Data.Current != 2 || result.Data.Pages != 2 {
		t.Fatalf("unexpected page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "1" || result.Data.Records[0].Username != "admin" || result.Data.Records[0].Role != "admin" {
		t.Fatalf("unexpected records: %+v", result.Data.Records)
	}
}

func newUserRouter(authService userhttp.AuthService, userService userhttp.UserService, asAdmin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	if asAdmin {
		router.Use(func(c *gin.Context) {
			contextx.Set(c, &contextx.LoginUser{
				UserID:   "1",
				Username: "admin",
				Role:     "admin",
			})
			c.Next()
		})
	}
	group := router.Group("/api/ragent")
	userhttp.RegisterUserRoutes(group, authService, userService)
	return router
}
