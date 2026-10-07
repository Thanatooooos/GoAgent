package rag

import (
	"math"
	"strings"

	"github.com/gin-gonic/gin"

	runtimetrace "local/rag-project/internal/app/runtime/trace"
	"local/rag-project/internal/framework/exception"
)

// TraceHandler exposes the runtime execution projection. Session and journal
// are the only data source; this API never reads legacy RAG trace tables.
type TraceHandler struct{ service *runtimetrace.Service }

func NewTraceHandler(service *runtimetrace.Service) *TraceHandler {
	return &TraceHandler{service: service}
}

func RegisterTraceRoutes(r gin.IRouter, service *runtimetrace.Service) {
	if r == nil || service == nil {
		return
	}
	handler := NewTraceHandler(service)
	r.GET("/rag/traces/runs", handler.ListRuns)
	r.GET("/rag/traces/runs/:traceId", handler.GetDetail)
}

func (h *TraceHandler) ListRuns(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("runtime trace service is required", nil))
		return
	}
	result, err := h.service.Page(c.Request.Context(), runtimetrace.PageInput{
		Page:           parsePositiveInt(c.Query("current"), 1),
		PageSize:       parsePositiveInt(c.Query("size"), 10),
		TraceID:        strings.TrimSpace(c.Query("traceId")),
		ConversationID: strings.TrimSpace(c.Query("conversationId")),
		Status:         strings.TrimSpace(c.Query("status")),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[runtimetrace.Run]{Records: result.Items, Total: result.Total, Size: result.PageSize, Current: result.Page, Pages: pages})
}

func (h *TraceHandler) GetDetail(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("runtime trace service is required", nil))
		return
	}
	result, err := h.service.Detail(c.Request.Context(), strings.TrimSpace(c.Param("traceId")))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, result)
}

type pageResult[T any] struct {
	Records []T `json:"records"`
	Total   int `json:"total"`
	Size    int `json:"size"`
	Current int `json:"current"`
	Pages   int `json:"pages"`
}
