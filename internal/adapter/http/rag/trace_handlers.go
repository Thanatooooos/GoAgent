package rag

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

// TraceHandler 负责暴露 trace 查询接口。
type TraceHandler struct {
	service *ragservice.TraceService
}

const (
	defaultTraceName   = "rag_chat"
	defaultEntryMethod = "rag.v3.chat"
)

type ragTraceRunVO struct {
	TraceID        string     `json:"traceId"`
	TraceName      string     `json:"traceName,omitempty"`
	EntryMethod    string     `json:"entryMethod,omitempty"`
	ConversationID string     `json:"conversationId,omitempty"`
	TaskID         string     `json:"taskId,omitempty"`
	UserID         string     `json:"userId,omitempty"`
	UserName       string     `json:"userName,omitempty"`
	Username       string     `json:"username,omitempty"`
	Status         string     `json:"status,omitempty"`
	ErrorMessage   string     `json:"errorMessage,omitempty"`
	DurationMs     *int64     `json:"durationMs,omitempty"`
	StartTime      *time.Time `json:"startTime,omitempty"`
	EndTime        *time.Time `json:"endTime,omitempty"`
}

type ragTraceNodeVO struct {
	TraceID      string     `json:"traceId"`
	NodeID       string     `json:"nodeId"`
	ParentNodeID string     `json:"parentNodeId,omitempty"`
	Depth        *int       `json:"depth,omitempty"`
	NodeType     string     `json:"nodeType,omitempty"`
	NodeName     string     `json:"nodeName,omitempty"`
	ClassName    string     `json:"className,omitempty"`
	MethodName   string     `json:"methodName,omitempty"`
	Status       string     `json:"status,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	DurationMs   *int64     `json:"durationMs,omitempty"`
	StartTime    *time.Time `json:"startTime,omitempty"`
	EndTime      *time.Time `json:"endTime,omitempty"`
}

type ragTraceDetailVO struct {
	Run   ragTraceRunVO   `json:"run"`
	Nodes []ragTraceNodeVO `json:"nodes"`
}

type pageResult[T any] struct {
	Records []T `json:"records"`
	Total   int `json:"total"`
	Size    int `json:"size"`
	Current int `json:"current"`
	Pages   int `json:"pages"`
}

// NewTraceHandler 创建 trace HTTP 处理器。
func NewTraceHandler(service *ragservice.TraceService) *TraceHandler {
	return &TraceHandler{service: service}
}

// RegisterTraceRoutes 注册 trace 查询接口。
func RegisterTraceRoutes(r gin.IRouter, service *ragservice.TraceService) {
	if r == nil || service == nil {
		return
	}

	handler := NewTraceHandler(service)
	r.GET("/rag/traces/runs", handler.ListRuns)
	r.GET("/rag/traces/runs/:traceId", handler.GetDetail)
	r.GET("/rag/traces/runs/:traceId/nodes", handler.ListNodes)
}

// ListRuns 分页返回 trace run 列表。
func (h *TraceHandler) ListRuns(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("rag trace service is required", nil))
		return
	}

	page := parsePositiveInt(c.Query("current"), 1)
	pageSize := parsePositiveInt(c.Query("size"), 10)
	result, err := h.service.PageRuns(c.Request.Context(), ragservice.PageTraceRunsInput{
		Page:           page,
		PageSize:       pageSize,
		TraceID:        c.Query("traceId"),
		ConversationID: c.Query("conversationId"),
		TaskID:         c.Query("taskId"),
		Status:         c.Query("status"),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	records := make([]ragTraceRunVO, 0, len(result.Items))
	for _, item := range result.Items {
		username := strings.TrimSpace(item.UserID)
		resolvedUserName, err := h.service.ResolveUserName(c.Request.Context(), item.UserID)
		if err != nil {
			_ = c.Error(err)
			return
		}
		if resolvedUserName != "" {
			username = resolvedUserName
		}
		records = append(records, toRagTraceRunVO(item, username))
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	writeTraceSuccess(c, pageResult[ragTraceRunVO]{
		Records: records,
		Total:   result.Total,
		Size:    result.PageSize,
		Current: result.Page,
		Pages:   pages,
	})
}

// GetDetail 返回单条 trace run 及其节点详情。
func (h *TraceHandler) GetDetail(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("rag trace service is required", nil))
		return
	}

	traceID := strings.TrimSpace(c.Param("traceId"))
	detail, err := h.service.GetDetail(c.Request.Context(), traceID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	username := strings.TrimSpace(detail.Run.UserID)
	resolvedUserName, err := h.service.ResolveUserName(c.Request.Context(), detail.Run.UserID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if resolvedUserName != "" {
		username = resolvedUserName
	}

	writeTraceSuccess(c, ragTraceDetailVO{
		Run:   toRagTraceRunVO(detail.Run, username),
		Nodes: toRagTraceNodeVOs(detail.Nodes),
	})
}

// ListNodes 返回单条 trace 下的节点列表。
func (h *TraceHandler) ListNodes(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("rag trace service is required", nil))
		return
	}

	traceID := strings.TrimSpace(c.Param("traceId"))
	nodes, err := h.service.ListNodes(c.Request.Context(), traceID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	writeTraceSuccess(c, toRagTraceNodeVOs(nodes))
}

// toRagTraceRunVO 转换 trace run 出参。
func toRagTraceRunVO(item domain.RagTraceRun, username string) ragTraceRunVO {
	traceName := strings.TrimSpace(item.TraceName)
	if traceName == "" {
		traceName = defaultTraceName
	}
	entryMethod := strings.TrimSpace(item.EntryMethod)
	if entryMethod == "" {
		entryMethod = defaultEntryMethod
	}
	startTime := item.StartTime
	if startTime == nil && !item.CreateTime.IsZero() {
		startTime = &item.CreateTime
	}
	userValue := strings.TrimSpace(item.UserID)
	username = strings.TrimSpace(username)
	if username == "" {
		username = userValue
	}

	return ragTraceRunVO{
		TraceID:        item.TraceID,
		TraceName:      traceName,
		EntryMethod:    entryMethod,
		ConversationID: item.ConversationID,
		TaskID:         item.TaskID,
		UserID:         userValue,
		UserName:       username,
		Username:       username,
		Status:         item.Status,
		ErrorMessage:   item.ErrorMessage,
		DurationMs:     item.DurationMs,
		StartTime:      startTime,
		EndTime:        item.EndTime,
	}
}

// toRagTraceNodeVOs 批量转换 trace node 出参。
func toRagTraceNodeVOs(items []domain.RagTraceNode) []ragTraceNodeVO {
	result := make([]ragTraceNodeVO, 0, len(items))
	for _, item := range items {
		result = append(result, toRagTraceNodeVO(item))
	}
	return result
}

// toRagTraceNodeVO 转换 trace node 出参。
func toRagTraceNodeVO(item domain.RagTraceNode) ragTraceNodeVO {
	return ragTraceNodeVO{
		TraceID:      item.TraceID,
		NodeID:       item.NodeID,
		ParentNodeID: item.ParentNodeID,
		Depth:        intPointer(item.Depth),
		NodeType:     item.NodeType,
		NodeName:     item.NodeName,
		ClassName:    item.ClassName,
		MethodName:   item.MethodName,
		Status:       item.Status,
		ErrorMessage: item.ErrorMessage,
		DurationMs:   item.DurationMs,
		StartTime:    item.StartTime,
		EndTime:      item.EndTime,
	}
}

// parsePositiveInt 解析正整数参数。
func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// intPointer 把零值深度转为 nil。
func intPointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

// writeTraceSuccess 输出 trace 接口成功响应。
func writeTraceSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, convention.Result[T]{
		Code:      "0",
		RequestID: middleware.RequestID(c),
		Data:      data,
	})
}
