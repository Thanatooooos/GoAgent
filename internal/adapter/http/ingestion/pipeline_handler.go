package ingestion

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

type pipelinePayload struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Nodes       []pipelineNodeVOItem `json:"nodes"`
}

type pipelineNodeVOItem struct {
	NodeID     string         `json:"nodeId"`
	NodeType   string         `json:"nodeType"`
	Settings   map[string]any `json:"settings,omitempty"`
	Condition  map[string]any `json:"condition,omitempty"`
	NextNodeID string         `json:"nextNodeId,omitempty"`
}

type pipelineVO struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	CreatedBy   string               `json:"createdBy,omitempty"`
	Nodes       []pipelineNodeVOItem `json:"nodes,omitempty"`
	CreateTime  *time.Time           `json:"createTime,omitempty"`
	UpdateTime  *time.Time           `json:"updateTime,omitempty"`
}

type pipelinePageVO struct {
	Records []pipelineVO `json:"records"`
	Total   int          `json:"total"`
	Size    int          `json:"size"`
	Current int          `json:"current"`
	Pages   int          `json:"pages"`
}

// RegisterPipelineRoutes 注册 ingestion pipeline 路由。
func RegisterPipelineRoutes(r gin.IRouter, service *ingestionservice.PipelineService) {
	handler := &PipelineHandler{service: service}
	r.GET("/ingestion/pipelines", handler.Page)
	r.GET("/ingestion/pipelines/:id", handler.Get)
	r.POST("/ingestion/pipelines", handler.Create)
	r.PUT("/ingestion/pipelines/:id", handler.Update)
	r.DELETE("/ingestion/pipelines/:id", handler.Delete)
}

// PipelineHandler 负责 pipeline HTTP 接口。
type PipelineHandler struct {
	service *ingestionservice.PipelineService
}

// Page 分页查询 pipeline 列表。
func (h *PipelineHandler) Page(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion pipeline service is required", nil))
		return
	}
	pageResult, err := h.service.Page(c.Request.Context(), ingestionservice.PagePipelinesInput{
		Page:     parsePage(c.Query("pageNo")),
		PageSize: parsePageSize(c.Query("pageSize")),
		Keyword:  c.Query("keyword"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, pipelinePageVO{
		Records: toPipelineVOs(pageResult.Items),
		Total:   pageResult.Total,
		Size:    pageResult.PageSize,
		Current: pageResult.Page,
		Pages:   calcPages(pageResult.Total, pageResult.PageSize),
	})
}

// Get 查询单个 pipeline。
func (h *PipelineHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("ingestion pipeline service is required", nil))
		return
	}
	item, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toPipelineVO(item))
}

// Create 创建 pipeline。
func (h *PipelineHandler) Create(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req pipelinePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	item, err := h.service.Create(c.Request.Context(), ingestionservice.CreatePipelineInput{
		Name:        req.Name,
		Description: req.Description,
		Nodes:       toPipelineNodes(req.Nodes),
		CreatedBy:   user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toPipelineVO(item))
}

// Update 更新 pipeline。
func (h *PipelineHandler) Update(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req pipelinePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	item, err := h.service.Update(c.Request.Context(), ingestionservice.UpdatePipelineInput{
		ID:          c.Param("id"),
		Name:        req.Name,
		Description: req.Description,
		Nodes:       toPipelineNodes(req.Nodes),
		UpdatedBy:   user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toPipelineVO(item))
}

// Delete 删除 pipeline。
func (h *PipelineHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// toPipelineNodes 把请求节点转换为领域节点。
func toPipelineNodes(items []pipelineNodeVOItem) []domain.PipelineNode {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.PipelineNode, 0, len(items))
	for _, item := range items {
		result = append(result, domain.PipelineNode{
			NodeID:     item.NodeID,
			NodeType:   item.NodeType,
			Settings:   item.Settings,
			Condition:  item.Condition,
			NextNodeID: item.NextNodeID,
		})
	}
	return result
}

// toPipelineVOs 批量转换 pipeline 出参。
func toPipelineVOs(items []domain.Pipeline) []pipelineVO {
	if len(items) == 0 {
		return nil
	}
	result := make([]pipelineVO, 0, len(items))
	for _, item := range items {
		result = append(result, toPipelineVO(item))
	}
	return result
}

// toPipelineVO 转换单个 pipeline 出参。
func toPipelineVO(item domain.Pipeline) pipelineVO {
	return pipelineVO{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		CreatedBy:   item.CreatedBy,
		Nodes:       toPipelineNodeVOs(item.Nodes),
		CreateTime:  timePointer(item.CreatedAt),
		UpdateTime:  timePointer(item.UpdatedAt),
	}
}

// toPipelineNodeVOs 批量转换节点定义。
func toPipelineNodeVOs(items []domain.PipelineNode) []pipelineNodeVOItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]pipelineNodeVOItem, 0, len(items))
	for _, item := range items {
		result = append(result, pipelineNodeVOItem{
			NodeID:     item.NodeID,
			NodeType:   item.NodeType,
			Settings:   item.Settings,
			Condition:  item.Condition,
			NextNodeID: item.NextNodeID,
		})
	}
	return result
}

// requireLoginUser 提取当前登录用户。
func requireLoginUser(c *gin.Context) *contextx.LoginUser {
	user := contextx.Get(c)
	if user == nil || user.UserID == "" {
		_ = c.Error(exception.NewClientException("unauthorized", nil))
		return nil
	}
	return user
}

// parsePage 解析分页页码。
func parsePage(value string) int {
	page, _ := strconv.Atoi(value)
	return page
}

// parsePageSize 解析分页大小。
func parsePageSize(value string) int {
	pageSize, _ := strconv.Atoi(value)
	return pageSize
}

// calcPages 计算总页数。
func calcPages(total int, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return pages
}

// timePointer 把零值时间过滤为空。
func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

// writeSuccess 输出统一成功响应。
func writeSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}
