package ingestion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	ingestionservice "local/rag-project/internal/app/ingestion/service"
	"local/rag-project/internal/middleware"
)

func TestMetricsHandlerReturnsSnapshot(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())

	metrics := ingestionservice.NewMetricsService(8)
	RegisterMetricsRoutes(router, metrics)

	req := httptest.NewRequest(http.MethodGet, "/ingestion/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Code string                           `json:"code"`
		Data ingestionservice.MetricsSnapshot `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.Code != "0" {
		t.Fatalf("unexpected code: %+v", body)
	}
	if body.Data.MaxConcurrent != 8 {
		t.Fatalf("MaxConcurrent = %d, want 8", body.Data.MaxConcurrent)
	}
	if body.Data.RunningTasks != 0 || body.Data.Totals.Submitted != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", body.Data)
	}
}
