package knowledge

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/framework/exception"
)

// WikiPageService 是 handler 依赖的 wiki 查询接口（服务端由 *wiki.WikiPageService 满足）。
type WikiPageService interface {
	GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error)
	ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]domain.WikiPage, int, error)
}

type WikiPageHandler struct {
	service WikiPageService
}

type wikiPageVO struct {
	ID         string     `json:"id"`
	KbId       string     `json:"kbId"`
	Slug       string     `json:"slug"`
	Title      string     `json:"title"`
	PageType   string     `json:"pageType"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary"`
	Content    string     `json:"content,omitempty"`
	CreatedBy  string     `json:"createdBy,omitempty"`
	CreateTime *time.Time `json:"createTime,omitempty"`
}

func NewWikiPageHandler(service WikiPageService) *WikiPageHandler {
	return &WikiPageHandler{service: service}
}

func RegisterWikiPageRoutes(r gin.IRoutes, service WikiPageService) {
	handler := NewWikiPageHandler(service)
	r.GET("/knowledge-base/:kb-id/wiki/pages", handler.List)
	r.GET("/knowledge-base/:kb-id/wiki/pages/*slug", handler.Get)
}

func (h *WikiPageHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("wiki page service is required", nil))
		return
	}
	slug := strings.TrimPrefix(c.Param("slug"), "/")
	page, err := h.service.GetBySlug(c.Request.Context(), c.Param("kb-id"), slug)
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toWikiPageVO(page))
}

func (h *WikiPageHandler) List(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("wiki page service is required", nil))
		return
	}
	page := parsePositiveInt(c.Query("current"), 1)
	size := parsePositiveInt(c.Query("size"), 10)
	pages, total, err := h.service.ListByKB(c.Request.Context(), c.Param("kb-id"), page, size)
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]wikiPageVO, 0, len(pages))
	for _, item := range pages {
		records = append(records, toWikiPageVO(item))
	}
	writeSuccess(c, pageResult[wikiPageVO]{
		Records: records,
		Total:   total,
		Size:    size,
		Current: page,
	})
}

func toWikiPageVO(item domain.WikiPage) wikiPageVO {
	return wikiPageVO{
		ID:         item.ID,
		KbId:       item.KnowledgeBaseID,
		Slug:       item.Slug,
		Title:      item.Title,
		PageType:   item.PageType,
		Status:     item.Status,
		Summary:    item.Summary,
		Content:    item.Content,
		CreatedBy:  item.CreatedBy,
		CreateTime: timePointer(item.CreatedAt),
	}
}
