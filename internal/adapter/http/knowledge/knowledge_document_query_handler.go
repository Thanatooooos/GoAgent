package knowledge

import (
	"math"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/service"
)

func (h *KnowledgeDocumentHandler) Get(c *gin.Context) {
	document, err := h.service.Get(c.Request.Context(), service.GetKnowledgeDocumentInput{DocumentID: c.Param("docId")})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toKnowledgeDocumentVO(document))
}

func (h *KnowledgeDocumentHandler) Page(c *gin.Context) {
	result, err := h.service.Page(c.Request.Context(), service.PageKnowledgeDocumentInput{
		KnowledgeBaseID: c.Param("kb-id"),
		Page:            parsePositiveInt(c.Query("current"), 1),
		PageSize:        parsePositiveInt(c.Query("size"), 10),
		Status:          c.Query("status"),
		Query:           c.Query("keyword"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]knowledgeDocumentVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeDocumentVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeDocumentVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *KnowledgeDocumentHandler) Search(c *gin.Context) {
	items, err := h.service.Search(c.Request.Context(), service.SearchKnowledgeDocumentsInput{
		Query: c.Query("keyword"),
		Limit: parsePositiveInt(c.Query("limit"), 8),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]knowledgeDocumentSearchVO, 0, len(items))
	for _, item := range items {
		result = append(result, knowledgeDocumentSearchVO{
			ID:              item.ID,
			KnowledgeBaseID: item.KnowledgeBaseID,
			Name:            item.Name,
		})
	}
	writeSuccess(c, result)
}
