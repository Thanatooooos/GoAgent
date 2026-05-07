package ingestion

import (
	"github.com/gin-gonic/gin"

	ingestionservice "local/rag-project/internal/app/ingestion/service"
	"local/rag-project/internal/framework/exception"
)

// RegisterMetricsRoutes 注册 ingestion metrics 路由。
func RegisterMetricsRoutes(r gin.IRouter, service *ingestionservice.MetricsService) {
	handler := &MetricsHandler{service: service}
	r.GET("/ingestion/metrics", handler.Get)
}

// MetricsHandler 负责 ingestion metrics HTTP 接口。
type MetricsHandler struct {
	service *ingestionservice.MetricsService
}

// Get 返回 ingestion 指标快照。
func (h *MetricsHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion metrics service is required", nil))
		return
	}
	writeSuccess(c, h.service.Snapshot())
}
