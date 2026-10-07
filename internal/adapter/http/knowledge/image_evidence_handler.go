package knowledge

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"local/rag-project/internal/app/knowledge/service/imageevidence"
)

// RegisterImageEvidenceReadRoutes must be mounted behind RequireLogin.
func RegisterImageEvidenceReadRoutes(r gin.IRoutes, service *imageevidence.Service) {
	handler := &imageEvidenceHandler{service: service}
	r.GET("/knowledge-base/images/:evidenceId", handler.get)
	r.GET("/knowledge-base/images/:evidenceId/original", handler.original)
}

// RegisterImageEvidenceAdminRoutes must be mounted behind admin authorization.
func RegisterImageEvidenceAdminRoutes(r gin.IRoutes, service *imageevidence.Service) {
	handler := &imageEvidenceHandler{service: service}
	r.GET("/knowledge-base/docs/:docId/images", handler.list)
	r.POST("/knowledge-base/images/:evidenceId/retry", handler.retry)
	r.POST("/knowledge-base/image-occurrences/:occurrenceId/retry", handler.retryOccurrence)
	r.GET("/knowledge-base/image-occurrences/:occurrenceId/original", handler.occurrenceOriginal)
}

type imageEvidenceHandler struct{ service *imageevidence.Service }

func (h *imageEvidenceHandler) list(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	items, err := h.service.List(c.Request.Context(), c.Param("docId"))
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	writeSuccess(c, items)
}

func (h *imageEvidenceHandler) retry(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	count, err := h.service.RetryFailed(c.Request.Context(), c.Param("evidenceId"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusConflict)
		return
	}
	writeSuccess(c, gin.H{"queued": count})
}

func (h *imageEvidenceHandler) retryOccurrence(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	count, err := h.service.RetryOccurrence(c.Request.Context(), c.Param("occurrenceId"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusConflict)
		return
	}
	writeSuccess(c, gin.H{"queued": count})
}

func (h *imageEvidenceHandler) get(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	detail, err := h.service.Get(c.Request.Context(), c.Param("evidenceId"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	writeSuccess(c, detail)
}

func (h *imageEvidenceHandler) original(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	data, mimeType, err := h.service.OpenOriginal(c.Request.Context(), c.Param("evidenceId"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, data)
}

func (h *imageEvidenceHandler) occurrenceOriginal(c *gin.Context) {
	if h.service == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	data, mimeType, err := h.service.OpenOccurrenceOriginal(c.Request.Context(), c.Param("occurrenceId"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, data)
}
