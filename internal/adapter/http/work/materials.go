package work

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/middleware"
	"mime"
	"net/http"
	"net/url"
)

func RegisterMaterialRoutes(r gin.IRouter, runtime *workbootstrap.Runtime) {
	h := &materialHandler{runtime}
	g := r.Group("/work/topics/:topicId")
	g.Use(middleware.RequireLogin())
	g.GET("/sources", h.list)
	g.POST("/sources/upload", h.upload)
	g.POST("/sources/link", h.link)
	g.POST("/sources/shared", h.shared)
	g.POST("/sources/:sourceId/promote", h.promote)
	g.POST("/sources/:sourceId/remove", h.remove)
	g.POST("/sources/:sourceId/retry", h.retry)
	g.GET("/sources/:sourceId/text", h.text)
	g.GET("/sources/:sourceId/original", h.original)
	g.GET("/citations/:chunkId", func(c *gin.Context) {
		v, e := runtime.Store.ReadCitation(c.Request.Context(), uid(c), c.Param("topicId"), c.Query("conversationId"), c.Param("chunkId"))
		reply(c, v, e)
	})
	g.GET("/available-knowledge-bases", func(c *gin.Context) {
		if _, e := runtime.Store.GetTopic(c.Request.Context(), uid(c), c.Param("topicId")); e != nil {
			reply(c, nil, e)
			return
		}
		v, e := runtime.Knowledge.BaseService.Page(c.Request.Context(), knowledgeservice.PageKnowledgeBaseInput{Page: 1, PageSize: 100})
		reply(c, v, e)
	})
	g.GET("/citations/:chunkId/original", func(c *gin.Context) {
		citation, err := runtime.Store.ReadCitation(c.Request.Context(), uid(c), c.Param("topicId"), c.Query("conversationId"), c.Param("chunkId"))
		if err != nil {
			reply(c, nil, err)
			return
		}
		if !citation.Image {
			reply(c, nil, domain.ErrNotFound)
			return
		}
		data, mimeType, err := runtime.Knowledge.ImageEvidenceService.OpenOriginal(c.Request.Context(), citation.ID)
		if err != nil {
			reply(c, nil, domain.ErrNotFound)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, mimeType, data)
	})
}

type materialHandler struct{ r *workbootstrap.Runtime }

func (h *materialHandler) list(c *gin.Context) {
	p, ok := page(c)
	if !ok {
		return
	}
	v, e := h.r.Store.ListSources(c.Request.Context(), uid(c), c.Param("topicId"), c.Query("conversationId"), p)
	reply(c, v, e)
}
func (h *materialHandler) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, err := c.FormFile("file")
	if err != nil {
		reply(c, nil, fmt.Errorf("请选择不超过 20 MB 的文件"))
		return
	}
	if file.Size > 20<<20 {
		reply(c, nil, fmt.Errorf("文件超过 20 MB"))
		return
	}
	body, err := file.Open()
	if err != nil {
		reply(c, nil, err)
		return
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, (20<<20)+1))
	if err != nil || len(raw) > 20<<20 {
		reply(c, nil, fmt.Errorf("文件读取失败或过大"))
		return
	}
	hash := sha256.Sum256(raw)
	reserve := domain.ReserveSource{Mutation: domain.Mutation{RequestID: c.PostForm("requestId")}, Name: file.Filename, SourceType: "file", ConversationID: c.PostForm("conversationId"), Attachment: c.PostForm("attachment") == "true", ContentHash: hex.EncodeToString(hash[:])}
	v, e := h.r.UploadSource(c.Request.Context(), uid(c), c.Param("topicId"), reserve, knowledgeservice.UploadKnowledgeDocumentInput{SourceType: "file", FileName: file.Filename, ContentType: file.Header.Get("Content-Type"), Size: int64(len(raw)), Body: bytes.NewReader(raw)})
	reply(c, v, e)
}
func (h *materialHandler) link(c *gin.Context) {
	in, ok := bind[domain.ReserveSource](c)
	if !ok {
		return
	}
	u, err := url.Parse(in.SourceLocation)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		reply(c, nil, fmt.Errorf("请输入有效的网页链接"))
		return
	}
	in.SourceType = "url"
	v, e := h.r.UploadSource(c.Request.Context(), uid(c), c.Param("topicId"), in, knowledgeservice.UploadKnowledgeDocumentInput{SourceType: "url", FileName: in.Name, SourceLocation: in.SourceLocation})
	reply(c, v, e)
}
func (h *materialHandler) shared(c *gin.Context) {
	in, ok := bind[domain.ReserveSource](c)
	if !ok {
		return
	}
	in.SourceType = "shared"
	v, e := h.r.Store.ReserveSource(c.Request.Context(), uid(c), c.Param("topicId"), h.r.EmbeddingModel, in)
	reply(c, v, e)
}
func (h *materialHandler) promote(c *gin.Context) { h.change(c, true) }
func (h *materialHandler) remove(c *gin.Context)  { h.change(c, false) }
func (h *materialHandler) change(c *gin.Context, promote bool) {
	in, ok := bind[domain.Mutation](c)
	if !ok {
		return
	}
	v, e := h.r.Store.ChangeSource(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("sourceId"), promote, in)
	reply(c, v, e)
}
func (h *materialHandler) retry(c *gin.Context) {
	e := h.r.RetrySource(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("sourceId"), c.Query("conversationId"))
	reply(c, gin.H{"queued": e == nil}, e)
}
func (h *materialHandler) text(c *gin.Context) {
	v, e := h.r.Store.SourceText(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("sourceId"), c.Query("conversationId"))
	reply(c, gin.H{"text": v}, e)
}
func (h *materialHandler) original(c *gin.Context) {
	src, err := h.r.Store.GetSource(c.Request.Context(), uid(c), c.Param("topicId"), c.Param("sourceId"), c.Query("conversationId"))
	if err != nil {
		reply(c, nil, err)
		return
	}
	if src.SourceType != "file" || src.DocumentID == "" {
		reply(c, nil, domain.ErrNotFound)
		return
	}
	d, err := h.r.Knowledge.DocumentService.Get(c.Request.Context(), knowledgeservice.GetKnowledgeDocumentInput{DocumentID: src.DocumentID})
	if err != nil {
		reply(c, nil, domain.ErrNotFound)
		return
	}
	body, err := h.r.Knowledge.Storage.Open(c.Request.Context(), d.FileURL)
	if err != nil {
		reply(c, nil, fmt.Errorf("原文件暂不可用"))
		return
	}
	defer body.Close()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": src.Name}))
	c.Header("Content-Type", "application/octet-stream")
	c.Status(200)
	_, _ = io.Copy(c.Writer, body)
}
