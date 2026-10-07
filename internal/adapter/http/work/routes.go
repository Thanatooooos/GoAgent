package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"local/rag-project/internal/app/work/domain"
	"local/rag-project/internal/app/work/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/middleware"
)

type Handler struct{ Service *service.Service }

func RegisterRoutes(r gin.IRouter, s *service.Service) {
	h := &Handler{Service: s}
	g := r.Group("/work")
	g.Use(middleware.RequireLogin())
	g.POST("/topics", h.createTopic)
	g.GET("/topics", h.listTopics)
	g.GET("/topics/:topicId", h.topic)
	g.PUT("/topics/:topicId", h.updateTopic)
	g.POST("/topics/:topicId/items", h.createItem)
	g.GET("/topics/:topicId/items", h.items)
	g.POST("/topics/:topicId/conversations", h.createConversation)
	g.GET("/topics/:topicId/conversations", h.conversations)
	g.GET("/topics/:topicId/state", h.state)
	g.PUT("/topics/:topicId/state", h.saveState)
	g.POST("/topics/:topicId/artifacts", h.createArtifact)
	g.GET("/topics/:topicId/artifacts", h.artifacts)
	g.GET("/topics/:topicId/artifacts/:artifactId", h.artifact)
	g.PUT("/topics/:topicId/artifacts/:artifactId", h.saveArtifact)
	g.GET("/topics/:topicId/artifacts/:artifactId/versions", h.versions)
	g.GET("/topics/:topicId/artifacts/:artifactId/versions/:revision", h.version)
	g.POST("/topics/:topicId/artifacts/:artifactId/restore", h.restore)
}
func uid(c *gin.Context) string {
	u := contextx.Get(c)
	if u == nil {
		return ""
	}
	return u.UserID
}
func reply(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, convention.Result[any]{Code: "0", Data: data, RequestID: middleware.RequestID(c)})
		return
	}
	status, code, message := http.StatusBadRequest, "WORK_INVALID", err.Error()
	var details any
	var conflict *domain.Conflict
	var databaseError *pgconn.PgError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = "WORK_NOT_FOUND"
	case errors.As(err, &conflict):
		status = http.StatusConflict
		code = "WORK_CONFLICT"
		details = gin.H{"currentRevision": conflict.CurrentRevision}
	case errors.Is(err, domain.ErrArchived):
		status = http.StatusConflict
		code = "WORK_ARCHIVED"
	case errors.Is(err, domain.ErrRequestReused):
		status = http.StatusConflict
		code = "WORK_REQUEST_REUSED"
	case errors.As(err, &databaseError):
		status = http.StatusInternalServerError
		code = "WORK_STORAGE_ERROR"
		message = "work storage operation failed"
	}
	c.JSON(status, convention.Result[any]{Code: code, Message: message, Data: details, RequestID: middleware.RequestID(c)})
}
func bind[T any](c *gin.Context) (T, bool) {
	var input T
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 2_000_000))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		reply(c, nil, fmt.Errorf("invalid request: %w", err))
		return input, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		reply(c, nil, fmt.Errorf("request must contain one JSON object"))
		return input, false
	}
	return input, true
}
func page(c *gin.Context) (domain.Page, bool) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if err != nil {
		reply(c, nil, fmt.Errorf("invalid limit"))
		return domain.Page{}, false
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil {
		reply(c, nil, fmt.Errorf("invalid offset"))
		return domain.Page{}, false
	}
	p := domain.Page{Limit: limit, Offset: offset}
	if err := p.Validate(); err != nil {
		reply(c, nil, err)
		return p, false
	}
	return p, true
}
func (h *Handler) createTopic(c *gin.Context) {
	in, ok := bind[domain.CreateTopic](c)
	if !ok {
		return
	}
	v, e := h.Service.CreateTopic(c.Request.Context(), uid(c), in)
	reply(c, v, e)
}
func (h *Handler) listTopics(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.Service.ListTopics(c.Request.Context(), uid(c), c.Query("status"), p)
	reply(c, v, e)
}
func (h *Handler) topic(c *gin.Context) {
	v, e := h.Service.GetTopic(c.Request.Context(), uid(c), c.Param("topicId"))
	reply(c, v, e)
}
func (h *Handler) updateTopic(c *gin.Context) {
	in, ok := bind[domain.UpdateTopic](c)
	if !ok {
		return
	}
	v, e := h.Service.UpdateTopic(c.Request.Context(), uid(c), c.Param("topicId"), in)
	reply(c, v, e)
}
func (h *Handler) createItem(c *gin.Context) {
	in, ok := bind[domain.CreateItem](c)
	if !ok {
		return
	}
	v, e := h.Service.CreateItem(c.Request.Context(), uid(c), c.Param("topicId"), in)
	reply(c, v, e)
}
func (h *Handler) items(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.Service.ListItems(c.Request.Context(), uid(c), c.Param("topicId"), p)
	reply(c, v, e)
}
func (h *Handler) createConversation(c *gin.Context) {
	in, ok := bind[domain.CreateConversation](c)
	if !ok {
		return
	}
	v, e := h.Service.CreateConversation(c.Request.Context(), uid(c), c.Param("topicId"), in)
	reply(c, v, e)
}
func (h *Handler) conversations(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.Service.ListConversations(c.Request.Context(), uid(c), c.Param("topicId"), p)
	reply(c, v, e)
}
func (h *Handler) state(c *gin.Context) {
	v, e := h.Service.GetState(c.Request.Context(), uid(c), c.Param("topicId"))
	reply(c, v, e)
}
func (h *Handler) saveState(c *gin.Context) {
	in, ok := bind[domain.SaveState](c)
	if !ok {
		return
	}
	v, e := h.Service.SaveState(c.Request.Context(), uid(c), c.Param("topicId"), in)
	reply(c, v, e)
}
func (h *Handler) createArtifact(c *gin.Context) {
	in, ok := bind[domain.SaveArtifact](c)
	if !ok {
		return
	}
	v, e := h.Service.CreateArtifact(c.Request.Context(), uid(c), c.Param("topicId"), in)
	reply(c, v, e)
}
func (h *Handler) artifacts(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.Service.ListArtifacts(c.Request.Context(), uid(c), c.Param("topicId"), p)
	reply(c, v, e)
}
func (h *Handler) artifact(c *gin.Context) {
	v, e := h.Service.GetArtifact(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("artifactId"))
	reply(c, v, e)
}
func (h *Handler) saveArtifact(c *gin.Context) {
	in, ok := bind[domain.SaveArtifact](c)
	if !ok {
		return
	}
	v, e := h.Service.SaveArtifact(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("artifactId"), in)
	reply(c, v, e)
}
func (h *Handler) versions(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.Service.ListVersions(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("artifactId"), p)
	reply(c, v, e)
}
func (h *Handler) version(c *gin.Context) {
	n, e := strconv.Atoi(c.Param("revision"))
	if e != nil || n < 1 {
		reply(c, nil, fmt.Errorf("invalid revision"))
		return
	}
	v, e := h.Service.GetVersion(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("artifactId"), n)
	reply(c, v, e)
}
func (h *Handler) restore(c *gin.Context) {
	in, ok := bind[domain.RestoreArtifact](c)
	if !ok {
		return
	}
	v, e := h.Service.RestoreArtifact(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("artifactId"), in)
	reply(c, v, e)
}
