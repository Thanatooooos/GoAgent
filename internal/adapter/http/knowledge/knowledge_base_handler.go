package knowledge

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

type KnowledgeBaseService interface {
	Create(ctx context.Context, input service.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	Update(ctx context.Context, input service.UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	Delete(ctx context.Context, input service.DeleteKnowledgeBaseInput) error
	Get(ctx context.Context, input service.GetKnowledgeBaseInput) (domain.KnowledgeBase, error)
	Page(ctx context.Context, input service.PageKnowledgeBaseInput) (service.KnowledgeBasePageResult, error)
}

type KnowledgeBaseHandler struct {
	service KnowledgeBaseService
}

func NewKnowledgeBaseHandler(service KnowledgeBaseService) *KnowledgeBaseHandler {
	return &KnowledgeBaseHandler{service: service}
}

func RegisterKnowledgeBaseRoutes(r gin.IRoutes, service KnowledgeBaseService) {
	handler := NewKnowledgeBaseHandler(service)
	r.POST("/knowledge-base", handler.Create)
	r.GET("/knowledge-base", handler.Page)
	r.GET("/knowledge-base/chunk-strategies", handler.ChunkStrategies)
	r.GET("/knowledge-base/:kb-id", handler.Get)
	r.PUT("/knowledge-base/:kb-id", handler.Update)
	r.DELETE("/knowledge-base/:kb-id", handler.Delete)
}

type createKnowledgeBaseRequest struct {
	Name           string `json:"name"`
	EmbeddingModel string `json:"embeddingModel"`
	CollectionName string `json:"collectionName"`
}

type updateKnowledgeBaseRequest struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	EmbeddingModel string `json:"embeddingModel"`
}

type knowledgeBaseVO struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	EmbeddingModel string     `json:"embeddingModel"`
	CollectionName string     `json:"collectionName"`
	DocumentCount  int        `json:"documentCount"`
	CreatedBy      string     `json:"createdBy,omitempty"`
	CreateTime     *time.Time `json:"createTime,omitempty"`
	UpdateTime     *time.Time `json:"updateTime,omitempty"`
}

type pageResult[T any] struct {
	Records []T `json:"records"`
	Total   int `json:"total"`
	Size    int `json:"size"`
	Current int `json:"current"`
	Pages   int `json:"pages"`
}

type chunkStrategyVO struct {
	Value         string         `json:"value"`
	Label         string         `json:"label"`
	DefaultConfig map[string]int `json:"defaultConfig"`
}

func (h *KnowledgeBaseHandler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge base service is required", nil))
		return
	}

	var req createKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	created, err := h.service.Create(c.Request.Context(), service.CreateKnowledgeBaseInput{
		Name:           req.Name,
		EmbeddingModel: req.EmbeddingModel,
		CollectionName: req.CollectionName,
		OperatorID:     operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	writeSuccess(c, created.ID)
}

func (h *KnowledgeBaseHandler) Update(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge base service is required", nil))
		return
	}

	var req updateKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}

	id := strings.TrimSpace(c.Param("kb-id"))
	if id == "" {
		id = strings.TrimSpace(req.ID)
	}
	_, err := h.service.Update(c.Request.Context(), service.UpdateKnowledgeBaseInput{
		ID:             id,
		Name:           req.Name,
		EmbeddingModel: req.EmbeddingModel,
		OperatorID:     operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	writeSuccess[any](c, nil)
}

func (h *KnowledgeBaseHandler) Delete(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge base service is required", nil))
		return
	}

	if err := h.service.Delete(c.Request.Context(), service.DeleteKnowledgeBaseInput{ID: c.Param("kb-id")}); err != nil {
		_ = c.Error(err)
		return
	}

	writeSuccess[any](c, nil)
}

func (h *KnowledgeBaseHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge base service is required", nil))
		return
	}

	knowledgeBase, err := h.service.Get(c.Request.Context(), service.GetKnowledgeBaseInput{ID: c.Param("kb-id")})
	if err != nil {
		_ = c.Error(err)
		return
	}

	writeSuccess(c, toKnowledgeBaseVO(knowledgeBase, 0))
}

func (h *KnowledgeBaseHandler) Page(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge base service is required", nil))
		return
	}

	result, err := h.service.Page(c.Request.Context(), service.PageKnowledgeBaseInput{
		Page:     parsePositiveInt(c.Query("current"), 1),
		PageSize: parsePositiveInt(c.Query("size"), 20),
		Query:    c.Query("name"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	records := make([]knowledgeBaseVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeBaseVO(item, result.DocumentCounts[item.ID]))
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	writeSuccess(c, pageResult[knowledgeBaseVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *KnowledgeBaseHandler) ChunkStrategies(c *gin.Context) {
	writeSuccess(c, []chunkStrategyVO{
		{
			Value: "fixed_size",
			Label: "\u56fa\u5b9a\u5927\u5c0f",
			DefaultConfig: map[string]int{
				"chunkSize":   512,
				"overlapSize": 128,
			},
		},
		{
			Value: "structure_aware",
			Label: "\u8bed\u4e49\u611f\u77e5\uff08Markdown\u53cb\u597d\uff09",
			DefaultConfig: map[string]int{
				"targetChars":  1400,
				"overlapChars": 0,
				"maxChars":     1800,
				"minChars":     600,
			},
		},
	})
}

func toKnowledgeBaseVO(item domain.KnowledgeBase, documentCount int) knowledgeBaseVO {
	return knowledgeBaseVO{
		ID:             item.ID,
		Name:           item.Name,
		EmbeddingModel: item.EmbeddingModel,
		CollectionName: item.CollectionName,
		DocumentCount:  documentCount,
		CreatedBy:      item.CreatedBy,
		CreateTime:     timePointer(item.CreatedAt),
		UpdateTime:     timePointer(item.UpdatedAt),
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
	if value := strings.TrimSpace(c.GetHeader("X-User-ID")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Login-Id")); value != "" {
		return value
	}
	return "system"
}

func writeSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}
