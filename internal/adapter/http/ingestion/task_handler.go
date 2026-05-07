package ingestion

import (
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	"local/rag-project/internal/framework/exception"
)

type taskCreatePayload struct {
	PipelineID string            `json:"pipelineId"`
	Source     taskSourcePayload `json:"source"`
	Metadata   map[string]any    `json:"metadata"`
}

type taskSourcePayload struct {
	Type        string            `json:"type"`
	Location    string            `json:"location"`
	FileName    string            `json:"fileName"`
	Credentials map[string]string `json:"credentials"`
}

type taskVO struct {
	ID             string         `json:"id"`
	PipelineID     string         `json:"pipelineId"`
	SourceType     string         `json:"sourceType,omitempty"`
	SourceLocation string         `json:"sourceLocation,omitempty"`
	SourceFileName string         `json:"sourceFileName,omitempty"`
	Status         string         `json:"status,omitempty"`
	ChunkCount     *int           `json:"chunkCount,omitempty"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	CreatedBy      string         `json:"createdBy,omitempty"`
	CreateTime     *time.Time     `json:"createTime,omitempty"`
	UpdateTime     *time.Time     `json:"updateTime,omitempty"`
}

type taskNodeVO struct {
	ID           string         `json:"id"`
	TaskID       string         `json:"taskId"`
	PipelineID   string         `json:"pipelineId"`
	NodeID       string         `json:"nodeId"`
	NodeType     string         `json:"nodeType"`
	NodeOrder    *int           `json:"nodeOrder,omitempty"`
	Status       string         `json:"status,omitempty"`
	DurationMs   *int64         `json:"durationMs,omitempty"`
	Message      string         `json:"message,omitempty"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	CreateTime   *time.Time     `json:"createTime,omitempty"`
	UpdateTime   *time.Time     `json:"updateTime,omitempty"`
}

type taskPageVO struct {
	Records []taskVO `json:"records"`
	Total   int      `json:"total"`
	Size    int      `json:"size"`
	Current int      `json:"current"`
	Pages   int      `json:"pages"`
}

type taskCreateResultVO struct {
	TaskID     string `json:"taskId"`
	PipelineID string `json:"pipelineId"`
	Status     string `json:"status,omitempty"`
	ChunkCount *int   `json:"chunkCount,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RegisterTaskRoutes 注册 ingestion task 路由。
func RegisterTaskRoutes(r gin.IRouter, service *ingestionservice.TaskService) {
	handler := &TaskHandler{service: service}
	r.GET("/ingestion/tasks", handler.Page)
	r.GET("/ingestion/tasks/:id", handler.Get)
	r.GET("/ingestion/tasks/:id/nodes", handler.ListNodes)
	r.POST("/ingestion/tasks", handler.Create)
	r.POST("/ingestion/tasks/upload", handler.Upload)
}

// TaskHandler 负责 task HTTP 接口。
type TaskHandler struct {
	service *ingestionservice.TaskService
}

// Page 分页查询 task 列表。
func (h *TaskHandler) Page(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion task service is required", nil))
		return
	}
	pageResult, err := h.service.Page(c.Request.Context(), ingestionservice.PageTasksInput{
		Page:       parsePage(c.Query("pageNo")),
		PageSize:   parsePageSize(c.Query("pageSize")),
		PipelineID: c.Query("pipelineId"),
		Status:     c.Query("status"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, taskPageVO{
		Records: toTaskVOs(pageResult.Items),
		Total:   pageResult.Total,
		Size:    pageResult.PageSize,
		Current: pageResult.Page,
		Pages:   calcPages(pageResult.Total, pageResult.PageSize),
	})
}

// Get 查询单个 task。
func (h *TaskHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion task service is required", nil))
		return
	}
	item, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toTaskVO(item))
}

// ListNodes 查询单个 task 下的节点日志。
func (h *TaskHandler) ListNodes(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion task service is required", nil))
		return
	}
	items, err := h.service.ListNodes(c.Request.Context(), c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toTaskNodeVOs(items))
}

// Create 创建一条 task。
func (h *TaskHandler) Create(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req taskCreatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	item, err := h.service.Create(c.Request.Context(), ingestionservice.CreateTaskInput{
		PipelineID:     req.PipelineID,
		SourceType:     req.Source.Type,
		SourceLocation: req.Source.Location,
		SourceFileName: req.Source.FileName,
		Metadata:       mergeTaskMetadata(req.Metadata, req.Source.Credentials),
		CreatedBy:      user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toTaskCreateResultVO(item))
}

// Upload 以文件上传方式创建 task。
func (h *TaskHandler) Upload(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	pipelineID := c.Query("pipelineId")
	file, err := c.FormFile("file")
	if err != nil {
		_ = c.Error(err)
		return
	}
	sourceLocation, metadata, err := persistUploadedSource(file)
	if err != nil {
		_ = c.Error(exception.NewServiceException("failed to persist uploaded source file", err))
		return
	}
	item, err := h.service.Create(c.Request.Context(), ingestionservice.CreateTaskInput{
		PipelineID:     pipelineID,
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: sourceLocation,
		SourceFileName: file.Filename,
		Metadata:       metadata,
		CreatedBy:      user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toTaskCreateResultVO(item))
}

// buildUploadedSourceLocation 为上传文件构造一个临时占位来源。
func buildUploadedSourceLocation(file *multipart.FileHeader) string {
	if file == nil {
		return ""
	}
	return file.Filename
}

func mergeTaskMetadata(metadata map[string]any, credentials map[string]string) map[string]any {
	if len(metadata) == 0 && len(credentials) == 0 {
		return nil
	}
	result := make(map[string]any, len(metadata)+2)
	for key, value := range metadata {
		result[key] = value
	}
	if len(credentials) > 0 {
		result["sourceCredentials"] = credentials
		result["headers"] = credentials
	}
	return result
}

func persistUploadedSource(file *multipart.FileHeader) (string, map[string]any, error) {
	if file == nil {
		return "", nil, nil
	}
	pattern := "goagent-ingestion-*"
	if ext := filepath.Ext(file.Filename); ext != "" {
		pattern += ext
	}
	tempFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	path := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(path)
		return "", nil, closeErr
	}
	if err := saveMultipartFile(file, path); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	return path, map[string]any{
		"cleanupSourceLocation": true,
	}, nil
}

func saveMultipartFile(file *multipart.FileHeader, targetPath string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = dst.ReadFrom(src)
	return err
}

// toTaskVOs 批量转换 task 出参。
func toTaskVOs(items []domain.Task) []taskVO {
	if len(items) == 0 {
		return nil
	}
	result := make([]taskVO, 0, len(items))
	for _, item := range items {
		result = append(result, toTaskVO(item))
	}
	return result
}

// toTaskVO 转换单个 task 出参。
func toTaskVO(item domain.Task) taskVO {
	return taskVO{
		ID:             item.ID,
		PipelineID:     item.PipelineID,
		SourceType:     item.SourceType,
		SourceLocation: item.SourceLocation,
		SourceFileName: item.SourceFileName,
		Status:         item.Status,
		ChunkCount:     intPointer(item.ChunkCount),
		ErrorMessage:   item.ErrorMessage,
		Metadata:       item.Metadata,
		StartedAt:      item.StartedAt,
		CompletedAt:    item.CompletedAt,
		CreatedBy:      item.CreatedBy,
		CreateTime:     timePointer(item.CreatedAt),
		UpdateTime:     timePointer(item.UpdatedAt),
	}
}

// toTaskNodeVOs 批量转换 task node 出参。
func toTaskNodeVOs(items []domain.TaskNode) []taskNodeVO {
	if len(items) == 0 {
		return nil
	}
	result := make([]taskNodeVO, 0, len(items))
	for _, item := range items {
		result = append(result, toTaskNodeVO(item))
	}
	return result
}

// toTaskNodeVO 转换单个 task node 出参。
func toTaskNodeVO(item domain.TaskNode) taskNodeVO {
	return taskNodeVO{
		ID:           item.ID,
		TaskID:       item.TaskID,
		PipelineID:   item.PipelineID,
		NodeID:       item.NodeID,
		NodeType:     item.NodeType,
		NodeOrder:    intValuePointer(item.NodeOrder),
		Status:       item.Status,
		DurationMs:   int64ValuePointer(item.DurationMs),
		Message:      item.Message,
		ErrorMessage: item.ErrorMessage,
		Output:       item.Output,
		CreateTime:   timePointer(item.CreatedAt),
		UpdateTime:   timePointer(item.UpdatedAt),
	}
}

// toTaskCreateResultVO 转换 task 创建结果。
func toTaskCreateResultVO(item domain.Task) taskCreateResultVO {
	return taskCreateResultVO{
		TaskID:     item.ID,
		PipelineID: item.PipelineID,
		Status:     item.Status,
		ChunkCount: intPointer(item.ChunkCount),
	}
}

// intPointer 把数值包装成指针。
func intPointer(value int) *int {
	return &value
}

// intValuePointer 把节点序号包装成指针。
func intValuePointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

// int64ValuePointer 把时长包装成指针。
func int64ValuePointer(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
