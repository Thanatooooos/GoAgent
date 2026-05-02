package knowledge

import (
	"context"
	"encoding/json"
	"math"
	"mime/multipart"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/exception"
)

type KnowledgeDocumentService interface {
	Upload(ctx context.Context, input service.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	StartChunk(ctx context.Context, input service.StartChunkKnowledgeDocumentInput) error
	Get(ctx context.Context, input service.GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	Update(ctx context.Context, input service.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	Page(ctx context.Context, input service.PageKnowledgeDocumentInput) (service.KnowledgeDocumentPageResult, error)
	Search(ctx context.Context, input service.SearchKnowledgeDocumentsInput) ([]service.KnowledgeDocumentSearchItem, error)
	Enable(ctx context.Context, input service.EnableKnowledgeDocumentInput) error
	Delete(ctx context.Context, input service.DeleteKnowledgeDocumentInput) error
	PageChunkLogs(ctx context.Context, input service.KnowledgeDocumentChunkLogPageInput) (service.KnowledgeDocumentChunkLogPageResult, error)
	PageScheduleExecs(ctx context.Context, input service.PageKnowledgeDocumentScheduleExecInput) (service.KnowledgeDocumentScheduleExecPageResult, error)
}

type KnowledgeDocumentHandler struct {
	service KnowledgeDocumentService
}

func NewKnowledgeDocumentHandler(service KnowledgeDocumentService) *KnowledgeDocumentHandler {
	return &KnowledgeDocumentHandler{service: service}
}

func RegisterKnowledgeDocumentRoutes(r gin.IRoutes, service KnowledgeDocumentService) {
	handler := NewKnowledgeDocumentHandler(service)
	r.POST("/knowledge-base/:kb-id/docs/upload", handler.Upload)
	r.POST("/knowledge-base/docs/:docId/chunk", handler.StartChunk)
	r.DELETE("/knowledge-base/docs/:docId", handler.Delete)
	r.GET("/knowledge-base/docs/:docId", handler.Get)
	r.PUT("/knowledge-base/docs/:docId", handler.Update)
	r.GET("/knowledge-base/:kb-id/docs", handler.Page)
	r.GET("/knowledge-base/docs/search", handler.Search)
	r.PATCH("/knowledge-base/docs/:docId/enable", handler.Enable)
	r.GET("/knowledge-base/docs/:docId/chunk-logs", handler.ChunkLogs)
	r.GET("/knowledge-base/docs/:docId/schedule-execs", handler.ScheduleExecs)
}

type updateKnowledgeDocumentRequest struct {
	Name            string `json:"docName"`
	ProcessMode     string `json:"processMode"`
	ChunkStrategy   string `json:"chunkStrategy"`
	ChunkConfig     string `json:"chunkConfig"`
	PipelineID      string `json:"pipelineId"`
	SourceLocation  string `json:"sourceLocation"`
	ScheduleEnabled *bool  `json:"scheduleEnabled"`
	ScheduleCron    string `json:"scheduleCron"`
}

type knowledgeDocumentVO struct {
	ID              string     `json:"id"`
	KnowledgeBaseID string     `json:"kbId"`
	Name            string     `json:"docName"`
	SourceType      string     `json:"sourceType,omitempty"`
	SourceLocation  string     `json:"sourceLocation,omitempty"`
	ScheduleEnabled int        `json:"scheduleEnabled"`
	ScheduleCron    string     `json:"scheduleCron,omitempty"`
	Enabled         bool       `json:"enabled"`
	ChunkCount      int        `json:"chunkCount"`
	FileURL         string     `json:"fileUrl,omitempty"`
	FileType        string     `json:"fileType,omitempty"`
	FileSize        int64      `json:"fileSize"`
	ProcessMode     string     `json:"processMode,omitempty"`
	ChunkStrategy   string     `json:"chunkStrategy,omitempty"`
	ChunkConfig     string     `json:"chunkConfig,omitempty"`
	PipelineID      string     `json:"pipelineId,omitempty"`
	Status          string     `json:"status,omitempty"`
	CreatedBy       string     `json:"createdBy,omitempty"`
	UpdatedBy       string     `json:"updatedBy,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	UpdateTime      *time.Time `json:"updateTime,omitempty"`
}

type knowledgeDocumentSearchVO struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"kbId"`
	Name            string `json:"docName"`
}

type knowledgeDocumentChunkLogVO struct {
	ID              string     `json:"id"`
	DocumentID      string     `json:"docId"`
	Status          string     `json:"status"`
	ProcessMode     string     `json:"processMode,omitempty"`
	ChunkStrategy   string     `json:"chunkStrategy,omitempty"`
	PipelineID      string     `json:"pipelineId,omitempty"`
	ExtractDuration int64      `json:"extractDuration"`
	ChunkDuration   int64      `json:"chunkDuration"`
	EmbedDuration   int64      `json:"embedDuration"`
	PersistDuration int64      `json:"persistDuration"`
	TotalDuration   int64      `json:"totalDuration"`
	ChunkCount      int        `json:"chunkCount"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	EndTime         *time.Time `json:"endTime,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
}

type knowledgeDocumentScheduleExecVO struct {
	ID              string     `json:"id"`
	ScheduleID      string     `json:"scheduleId"`
	DocumentID      string     `json:"docId"`
	KnowledgeBaseID string     `json:"kbId"`
	Status          string     `json:"status"`
	Message         string     `json:"message,omitempty"`
	FileName        string     `json:"fileName,omitempty"`
	FileSize        *int64     `json:"fileSize,omitempty"`
	ContentHash     string     `json:"contentHash,omitempty"`
	ETag            string     `json:"etag,omitempty"`
	LastModified    string     `json:"lastModified,omitempty"`
	StartTime       *time.Time `json:"startTime,omitempty"`
	EndTime         *time.Time `json:"endTime,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	UpdateTime      *time.Time `json:"updateTime,omitempty"`
}

func (h *KnowledgeDocumentHandler) Upload(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge document service is required", nil))
		return
	}

	file, err := c.FormFile("file")
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		_ = c.Error(err)
		return
	}
	body, closeFn, err := openMultipartFile(file)
	if err != nil {
		_ = c.Error(err)
		return
	}
	defer closeFn()

	created, err := h.service.Upload(c.Request.Context(), service.UploadKnowledgeDocumentInput{
		KnowledgeBaseID: c.Param("kb-id"),
		SourceType:      c.PostForm("sourceType"),
		FileName:        firstNonEmpty(c.PostForm("fileName"), fileNameFromHeader(file)),
		ContentType:     contentTypeFromHeader(file),
		Size:            multipartFileSize(file),
		Body:            body,
		SourceLocation:  c.PostForm("sourceLocation"),
		ScheduleEnabled: parseBool(c.PostForm("scheduleEnabled")),
		ScheduleCron:    c.PostForm("scheduleCron"),
		ProcessMode:     c.PostForm("processMode"),
		ChunkStrategy:   c.PostForm("chunkStrategy"),
		ChunkConfig:     c.PostForm("chunkConfig"),
		PipelineID:      c.PostForm("pipelineId"),
		OperatorID:      operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toKnowledgeDocumentVO(created))
}

func (h *KnowledgeDocumentHandler) StartChunk(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge document service is required", nil))
		return
	}
	if err := h.service.StartChunk(c.Request.Context(), service.StartChunkKnowledgeDocumentInput{
		DocumentID: c.Param("docId"),
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeDocumentHandler) Delete(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge document service is required", nil))
		return
	}
	if err := h.service.Delete(c.Request.Context(), service.DeleteKnowledgeDocumentInput{
		DocumentID: c.Param("docId"),
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeDocumentHandler) Get(c *gin.Context) {
	document, err := h.service.Get(c.Request.Context(), service.GetKnowledgeDocumentInput{DocumentID: c.Param("docId")})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toKnowledgeDocumentVO(document))
}

func (h *KnowledgeDocumentHandler) Update(c *gin.Context) {
	var req updateKnowledgeDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	_, err := h.service.Update(c.Request.Context(), service.UpdateKnowledgeDocumentInput{
		DocumentID:      c.Param("docId"),
		Name:            req.Name,
		ProcessMode:     req.ProcessMode,
		ChunkStrategy:   req.ChunkStrategy,
		ChunkConfig:     req.ChunkConfig,
		PipelineID:      req.PipelineID,
		SourceLocation:  req.SourceLocation,
		ScheduleEnabled: req.ScheduleEnabled,
		ScheduleCron:    req.ScheduleCron,
		OperatorID:      operatorID(c),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
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

func (h *KnowledgeDocumentHandler) Enable(c *gin.Context) {
	if err := h.service.Enable(c.Request.Context(), service.EnableKnowledgeDocumentInput{
		DocumentID: c.Param("docId"),
		Enabled:    parseBool(c.Query("value")),
		OperatorID: operatorID(c),
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

func (h *KnowledgeDocumentHandler) ChunkLogs(c *gin.Context) {
	result, err := h.service.PageChunkLogs(c.Request.Context(), service.KnowledgeDocumentChunkLogPageInput{
		DocumentID: c.Param("docId"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 10),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	records := make([]knowledgeDocumentChunkLogVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeDocumentChunkLogVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeDocumentChunkLogVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func (h *KnowledgeDocumentHandler) ScheduleExecs(c *gin.Context) {
	result, err := h.service.PageScheduleExecs(c.Request.Context(), service.PageKnowledgeDocumentScheduleExecInput{
		DocumentID: c.Param("docId"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 10),
		Status:     c.Query("status"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	records := make([]knowledgeDocumentScheduleExecVO, 0, len(result.Items))
	for _, item := range result.Items {
		records = append(records, toKnowledgeDocumentScheduleExecVO(item))
	}
	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	writeSuccess(c, pageResult[knowledgeDocumentScheduleExecVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

func toKnowledgeDocumentVO(item domain.KnowledgeDocument) knowledgeDocumentVO {
	var chunkConfig string
	if len(item.ChunkConfig) > 0 && json.Valid(item.ChunkConfig) {
		chunkConfig = string(item.ChunkConfig)
	}
	return knowledgeDocumentVO{
		ID:              item.ID,
		KnowledgeBaseID: item.KnowledgeBaseID,
		Name:            item.Name,
		SourceType:      item.SourceType,
		SourceLocation:  item.SourceLocation,
		ScheduleEnabled: boolToInt(item.ScheduleEnabled),
		ScheduleCron:    item.ScheduleCron,
		Enabled:         item.Enabled,
		ChunkCount:      item.ChunkCount,
		FileURL:         item.FileURL,
		FileType:        item.FileType,
		FileSize:        item.FileSize,
		ProcessMode:     item.ProcessMode,
		ChunkStrategy:   item.ChunkStrategy,
		ChunkConfig:     chunkConfig,
		PipelineID:      item.PipelineID,
		Status:          item.Status,
		CreatedBy:       item.CreatedBy,
		UpdatedBy:       item.UpdatedBy,
		CreateTime:      timePointer(item.CreatedAt),
		UpdateTime:      timePointer(item.UpdatedAt),
	}
}

func toKnowledgeDocumentChunkLogVO(item domain.KnowledgeDocumentChunkLog) knowledgeDocumentChunkLogVO {
	return knowledgeDocumentChunkLogVO{
		ID:              item.ID,
		DocumentID:      item.DocumentID,
		Status:          item.Status,
		ProcessMode:     item.ProcessMode,
		ChunkStrategy:   item.ChunkStrategy,
		PipelineID:      item.PipelineID,
		ExtractDuration: item.ExtractDuration,
		ChunkDuration:   item.ChunkDuration,
		EmbedDuration:   item.EmbedDuration,
		PersistDuration: item.PersistDuration,
		TotalDuration:   item.TotalDuration,
		ChunkCount:      item.ChunkCount,
		ErrorMessage:    item.ErrorMessage,
		StartTime:       item.StartTime,
		EndTime:         item.EndTime,
		CreateTime:      timePointer(item.CreatedAt),
	}
}

func toKnowledgeDocumentScheduleExecVO(item domain.KnowledgeDocumentScheduleExec) knowledgeDocumentScheduleExecVO {
	return knowledgeDocumentScheduleExecVO{
		ID:              item.ID,
		ScheduleID:      item.ScheduleID,
		DocumentID:      item.DocumentID,
		KnowledgeBaseID: item.KnowledgeBaseID,
		Status:          item.Status,
		Message:         item.Message,
		FileName:        item.FileName,
		FileSize:        item.FileSize,
		ContentHash:     item.ContentHash,
		ETag:            item.ETag,
		LastModified:    item.LastModified,
		StartTime:       item.StartTime,
		EndTime:         item.EndTime,
		CreateTime:      timePointer(item.CreatedAt),
		UpdateTime:      timePointer(item.UpdatedAt),
	}
}

func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func multipartFileSize(file *multipart.FileHeader) int64 {
	if file == nil {
		return 0
	}
	return file.Size
}

func contentTypeFromHeader(file *multipart.FileHeader) string {
	if file == nil {
		return ""
	}
	return file.Header.Get("Content-Type")
}

func fileNameFromHeader(file *multipart.FileHeader) string {
	if file == nil {
		return ""
	}
	return file.Filename
}

func openMultipartFile(file *multipart.FileHeader) (multipart.File, func(), error) {
	if file == nil {
		return nil, func() {}, nil
	}
	opened, err := file.Open()
	if err != nil {
		return nil, func() {}, err
	}
	return opened, func() { _ = opened.Close() }, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
