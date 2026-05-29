package rag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
)

func TestMemoryCacheMetricsHandlerReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := ragcachemetrics.NewService()
	metrics.Record("session_recall", "conversation", "hit")
	metrics.RecordMaintenanceRun(2, 3)
	metrics.RecordTouchLastUsedFailure()

	router := gin.New()
	RegisterMemoryCacheMetricsRoutes(router, metrics)

	req := httptest.NewRequest(http.MethodGet, "/rag/memory/metrics", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Data ragcachemetrics.MetricsSnapshot `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload.Data.Events) != 1 || payload.Data.Events[0].CacheKind != "session_recall" {
		t.Fatalf("unexpected metrics payload: %+v", payload.Data)
	}
	if payload.Data.MaintenanceRuns != 1 || payload.Data.MaintenanceExpiredCount != 2 || payload.Data.MaintenanceDeletedCount != 3 {
		t.Fatalf("unexpected maintenance counters in payload: %+v", payload.Data)
	}
	if payload.Data.TouchLastUsedFailures != 1 {
		t.Fatalf("unexpected fail-open counters in payload: %+v", payload.Data)
	}
}
