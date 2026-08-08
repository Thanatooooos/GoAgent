package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"local/rag-project/internal/framework/contextx"
	fwlog "local/rag-project/internal/framework/log"
	umw "local/rag-project/internal/middleware"
)

func TestRegisterCoreMiddlewareLogsAuthorizationRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	registerCoreMiddleware(router, func(string) (*contextx.LoginUser, error) {
		return nil, nil
	}, umw.DefaultLoginIDExtractor)
	router.GET("/private", umw.RequireLogin(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req = req.WithContext(fwlog.BindLogger(req.Context(), zap.New(core).Sugar()))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	entries := observed.All()
	if len(entries) != 1 || entries[0].Level != zap.WarnLevel || entries[0].ContextMap()["status_code"] != int64(http.StatusUnauthorized) {
		t.Fatalf("unexpected access entries: %+v", entries)
	}
}

func TestRegisterCoreMiddlewareLogsHandledServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	registerCoreMiddleware(router, nil, umw.DefaultLoginIDExtractor)
	router.GET("/error", func(c *gin.Context) {
		_ = c.Error(errors.New("boom"))
		c.Abort()
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	req = req.WithContext(fwlog.BindLogger(req.Context(), zap.New(core).Sugar()))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	entries := observed.All()
	if len(entries) != 1 || entries[0].Level != zap.ErrorLevel || entries[0].ContextMap()["status_code"] != int64(http.StatusInternalServerError) {
		t.Fatalf("unexpected access entries: %+v", entries)
	}
}
