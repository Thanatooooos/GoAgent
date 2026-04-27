package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
)

func TestRequestIDMiddlewareUsesIncomingID(t *testing.T) {
	router := newTestRouter()
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"requestId": RequestID(c)})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(requestIDHeader, "rid-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != "rid-1" {
		t.Fatalf("expected response request id rid-1, got %q", got)
	}
}

func TestErrorHandlerRecoversPanic(t *testing.T) {
	router := newTestRouter()
	router.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.Code == "" || result.Message == "" || result.RequestID == "" {
		t.Fatalf("expected code, message and request id, got %+v", result)
	}
}

func TestErrorHandlerWritesContextError(t *testing.T) {
	router := newTestRouter()
	router.GET("/err", func(c *gin.Context) {
		_ = c.Error(errors.New("bad"))
		c.Abort()
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestErrorHandlerClassifiesAppError(t *testing.T) {
	router := newTestRouter()
	router.GET("/client-err", func(c *gin.Context) {
		_ = c.Error(exception.NewClientException("bad request", nil))
		c.Abort()
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/client-err", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	result := decodeResult(t, rec)
	if result.Message != "bad request" {
		t.Fatalf("expected custom message, got %q", result.Message)
	}
}

func TestUserContextMiddlewareLoadsUser(t *testing.T) {
	router := newTestRouter()
	router.Use(UserContextMiddleware(func(loginID string) (*contextx.LoginUser, error) {
		return &contextx.LoginUser{UserID: loginID, Username: "alice", Role: "admin"}, nil
	}, nil))
	router.GET("/me", func(c *gin.Context) {
		user := contextx.Get(c)
		c.JSON(http.StatusOK, gin.H{"userId": user.UserID})
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("X-Login-Id", "1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestRequireLoginRejectsAnonymous(t *testing.T) {
	router := newTestRouter()
	router.GET("/private", RequireLogin(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestRequireRoleRejectsWrongRole(t *testing.T) {
	router := newTestRouter()
	router.Use(func(c *gin.Context) {
		contextx.Set(c, &contextx.LoginUser{UserID: "1", Role: "user"})
		c.Next()
	})
	router.GET("/admin", RequireRole("admin"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(ErrorHandlerMiddleware())
	return router
}

func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) convention.Result[any] {
	t.Helper()
	var result convention.Result[any]
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}
