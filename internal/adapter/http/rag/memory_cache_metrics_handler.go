package rag

import (
	"github.com/gin-gonic/gin"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/framework/exception"
)

func RegisterMemoryCacheMetricsRoutes(r gin.IRouter, service *ragcachemetrics.Service) {
	handler := &MemoryCacheMetricsHandler{service: service}
	r.GET("/rag/memory/metrics", handler.Get)
}

type MemoryCacheMetricsHandler struct {
	service *ragcachemetrics.Service
}

func (h *MemoryCacheMetricsHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("memory cache metrics service is required", nil))
		return
	}
	writeSuccess(c, h.service.Snapshot())
}
