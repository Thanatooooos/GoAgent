package knowledge

import (
	"context"
	"math"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/exception"
)

type KnowledgeChunkService interface {
	Page(ctx context.Context, input service.PageKnowledgeChunkInput) (service.KnowledgeChunkPageResult, error)
	Create(ctx context.Context, input service.CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error)
	Update(ctx context.Context, input service.UpdateKnowledgeChunkInput) error
	Delete(ctx context.Context, input service.DeleteKnowledgeChunkInput) error
	Enable(ctx context.Context, input service.EnableKnowledgeChunkInput) error
	BatchToggleEnabled(ctx context.Context, input service.BatchToggleKnowledgeChunksInput) error
}

type KnowledgeChunkHandler struct {
	service KnowledgeChunkService
}

type createKnowledgeChunkRequest struct {
	Content string `json:"content"`
	Index   *int   `json:"index"`
	ChunkID string `json:"chunkId"`
}

type updateKnowledgeChunkRequest struct {
	Content string `json:"content"`
}

type batchToggleKnowledgeChunkRequest struct {
	ChunkIDs []string `json:"chunkIds"`
}

type knowledgeChunkVO struct {
	ID              string     `json:"id"`
	KnowledgeBaseID string     `json:"kbId"`
	DocumentID      string     `json:"docId"`
	ChunkIndex      int        `json:"chunkIndex"`
	Content         string     `json:"content"`
	ContentHash     string     `json:"contentHash"`
	CharCount       int        `json:"charCount"`
	TokenCount      int        `json:"tokenCount"`
	Enabled         bool       `json:"enabled"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	UpdateTime      *time.Time `json:"updateTime,omitempty"`
}

func NewKnowledgeChunkHandler(service KnowledgeChunkService) *KnowledgeChunkHandler {
	return &KnowledgeChunkHandler{service: service}
}

func RegisterKnowledgeChunkRoutes(r gin.IRoutes, service KnowledgeChunkService) {
	handler := NewKnowledgeChunkHandler(service)
	r.GET("/knowledge-base/docs/:docId/chunks", handler.Page)
	r.POST("/knowledge-base/docs/:docId/chunks", handler.Create)
	r.PUT("/knowledge-base/docs/:docId/chunks/:chunkId", handler.Update)
	r.DELETE("/knowledge-base/docs/:docId/chunks/:chunkId", handler.Delete)
	r.PATCH("/knowledge-base/docs/:docId/chunks/:chunkId/enable", handler.Enable)
	r.PATCH("/knowledge-base/docs/:docId/chunks/batch-enable", handler.BatchEnable)
}

func (h *KnowledgeChunkHandler) Page(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge chunk service is required", nil))
		return
	}
	var enabled *bool
	if c.Query("enabled") != "" {
		value := parseBool(c.Query("enabled"))
		enabled = &value
	}
	result, err := h.service.Page(c.Request.Context(), service.PageKnowledgeChunkInput{
		DocumentID: c.Param("docId"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 10),
		Enabled:    enabled,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]knowledgeChunkVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeChunkVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeChunkVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *KnowledgeChunkHandler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge chunk service is required", nil))
		return
	}
	var req createKnowledgeChunkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	created, err := h.service.Create(c.Request.Context(), service.CreateKnowledgeChunkInput{
		DocumentID: c.Param("docId"),
		ChunkID:    req.ChunkID,
		Index:      req.Index,
		Content:    req.Content,
		OperatorID: operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toKnowledgeChunkVO(created))
}

func (h *KnowledgeChunkHandler) Update(c *gin.Context) {
	var req updateKnowledgeChunkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.service.Update(c.Request.Context(), service.UpdateKnowledgeChunkInput{
		DocumentID: c.Param("docId"),
		ChunkID:    c.Param("chunkId"),
		Content:    req.Content,
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeChunkHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), service.DeleteKnowledgeChunkInput{
		DocumentID: c.Param("docId"),
		ChunkID:    c.Param("chunkId"),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeChunkHandler) Enable(c *gin.Context) {
	if err := h.service.Enable(c.Request.Context(), service.EnableKnowledgeChunkInput{
		DocumentID: c.Param("docId"),
		ChunkID:    c.Param("chunkId"),
		Enabled:    parseBool(c.Query("value")),
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeChunkHandler) BatchEnable(c *gin.Context) {
	var req batchToggleKnowledgeChunkRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			_ = c.Error(err)
			return
		}
	}
	if err := h.service.BatchToggleEnabled(c.Request.Context(), service.BatchToggleKnowledgeChunksInput{
		DocumentID: c.Param("docId"),
		ChunkIDs:   req.ChunkIDs,
		Enabled:    parseBool(c.Query("value")),
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func toKnowledgeChunkVO(item domain.KnowledgeChunk) knowledgeChunkVO {
	return knowledgeChunkVO{
		ID:              item.ID,
		KnowledgeBaseID: item.KnowledgeBaseID,
		DocumentID:      item.DocumentID,
		ChunkIndex:      item.ChunkIndex,
		Content:         item.Content,
		ContentHash:     item.ContentHash,
		CharCount:       item.CharCount,
		TokenCount:      item.TokenCount,
		Enabled:         item.Enabled,
		CreateTime:      timePointer(item.CreatedAt),
		UpdateTime:      timePointer(item.UpdatedAt),
	}
}
