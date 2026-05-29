package rag

import (
	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

type rememberRequest struct {
	ScopeType       string `json:"scopeType"`
	ScopeID         string `json:"scopeId"`
	MemoryType      string `json:"memoryType"`
	Category        string `json:"category"`
	CanonicalKey    string `json:"canonicalKey"`
	ValueType       string `json:"valueType"`
	ValueJSON       string `json:"valueJson"`
	DisplayValue    string `json:"displayValue"`
	Importance      int    `json:"importance"`
	SourceMessageID string `json:"sourceMessageId"`
	Content         string `json:"content"`
	Summary         string `json:"summary"`
}

func (h *Handler) ListMemories(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.memoryService.ListMemories(c.Request.Context(), longtermmemory.ListMemoriesInput{
		UserID:       user.UserID,
		ScopeType:    c.Query("scopeType"),
		ScopeID:      c.Query("scopeId"),
		Namespace:    c.Query("namespace"),
		MemoryType:   c.Query("memoryType"),
		Category:     c.Query("category"),
		CanonicalKey: c.Query("canonicalKey"),
		Status:       c.Query("status"),
		Page:         parsePositiveInt(c.Query("current"), 1),
		PageSize:     parsePositiveInt(c.Query("size"), 20),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]memoryItemVO, 0, len(items))
	for _, item := range items {
		result = append(result, toMemoryItemVO(item))
	}
	writeSuccess(c, result)
}

func (h *Handler) Remember(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req rememberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	item, err := h.memoryService.SaveExplicitMemory(c.Request.Context(), longtermmemory.SaveExplicitMemoryInput{
		UserID:          user.UserID,
		ScopeType:       req.ScopeType,
		ScopeID:         req.ScopeID,
		MemoryType:      req.MemoryType,
		Category:        req.Category,
		CanonicalKey:    req.CanonicalKey,
		ValueType:       req.ValueType,
		ValueJSON:       req.ValueJSON,
		DisplayValue:    req.DisplayValue,
		Importance:      req.Importance,
		SourceMessageID: req.SourceMessageID,
		Content:         req.Content,
		Summary:         req.Summary,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toMemoryItemVO(item))
}

func (h *Handler) ExpireMemory(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	item, err := h.memoryService.ExpireMemory(c.Request.Context(), user.UserID, c.Param("memoryId"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toMemoryItemVO(item))
}

func toMemoryItemVO(item domain.MemoryItem) memoryItemVO {
	return memoryItemVO{
		ID:               item.ID,
		UserID:           item.UserID,
		ScopeType:        item.ScopeType,
		ScopeID:          item.ScopeID,
		Namespace:        item.Namespace,
		MemoryType:       item.MemoryType,
		Category:         item.Category,
		CanonicalKey:     item.CanonicalKey,
		ValueType:        item.ValueType,
		ValueJSON:        item.ValueJSON,
		DisplayValue:     item.DisplayValue,
		SourceMessageID:  item.SourceMessageID,
		Content:          item.Content,
		Summary:          item.Summary,
		Confidence:       item.Confidence,
		Importance:       item.Importance,
		Status:           item.Status,
		LastConfirmedAt:  item.LastConfirmedAt,
		LastUsedAt:       item.LastUsedAt,
		ExpiresAt:        item.ExpiresAt,
		SupersedesID:     item.SupersedesID,
		ExtractionMethod: item.ExtractionMethod,
		CreateTime:       timePointer(item.CreateTime),
		UpdateTime:       timePointer(item.UpdateTime),
	}
}
