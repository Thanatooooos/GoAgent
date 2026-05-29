package knowledge

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/exception"
)

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
