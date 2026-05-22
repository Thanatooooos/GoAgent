package rag

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	ragservice "local/rag-project/internal/app/rag/service"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	fwweb "local/rag-project/internal/framework/web"
	"local/rag-project/internal/middleware"
)

// Handler 负责承接最小 RAG 闭环的 HTTP 请求。
type Handler struct {
	conversationService *ragservice.ConversationService
	messageService      *ragservice.ConversationMessageService
	memoryService       *ragservice.MemoryService
	feedbackService     *ragservice.MessageFeedbackService
	chatService         *ragservice.RagChatService
}

// NewHandler 创建 RAG HTTP 处理器。
func NewHandler(
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *ragservice.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService *ragservice.RagChatService,
) *Handler {
	return &Handler{
		conversationService: conversationService,
		messageService:      messageService,
		memoryService:       memoryService,
		feedbackService:     feedbackService,
		chatService:         chatService,
	}
}

// RegisterRoutes 注册最小 RAG 闭环相关路由。
func RegisterRoutes(
	r gin.IRouter,
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *ragservice.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService *ragservice.RagChatService,
	traceService *ragservice.TraceService,
) {
	handler := NewHandler(conversationService, messageService, memoryService, feedbackService, chatService)
	r.GET("/conversations", handler.ListConversations)
	r.GET("/conversations/:conversationId/messages", handler.ListMessages)
	r.PUT("/conversations/:conversationId", handler.RenameConversation)
	r.DELETE("/conversations/:conversationId", handler.DeleteConversation)
	r.GET("/rag/v3/memories", handler.ListMemories)
	r.POST("/rag/v3/memories", handler.Remember)
	r.POST("/rag/v3/remember", handler.Remember)
	r.POST("/rag/v3/memories/:memoryId/expire", handler.ExpireMemory)
	r.POST("/conversations/messages/:messageId/feedback", handler.SubmitFeedback)
	r.GET("/rag/v3/chat", handler.Chat)
	r.POST("/rag/v3/stop", handler.StopChat)

	admin := r.Group("/")
	admin.Use(middleware.RequireRole("admin"))
	RegisterTraceRoutes(admin, traceService)
}

type renameConversationRequest struct {
	Title string `json:"title"`
}

type feedbackRequest struct {
	Vote int `json:"vote"`
}

type rememberRequest struct {
	ScopeType       string `json:"scopeType"`
	ScopeID         string `json:"scopeId"`
	MemoryType      string `json:"memoryType"`
	SourceMessageID string `json:"sourceMessageId"`
	Content         string `json:"content"`
	Summary         string `json:"summary"`
}

type conversationVO struct {
	ConversationID string     `json:"conversationId"`
	Title          string     `json:"title"`
	LastTime       *time.Time `json:"lastTime,omitempty"`
}

type messageVO struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversationId"`
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	RawContent       string     `json:"rawContent,omitempty"`
	ContentSummary   string     `json:"contentSummary,omitempty"`
	IsSummarized     bool       `json:"isSummarized,omitempty"`
	ThinkingContent  string     `json:"thinkingContent,omitempty"`
	ThinkingDuration *int       `json:"thinkingDuration,omitempty"`
	Vote             *int       `json:"vote"`
	CreateTime       *time.Time `json:"createTime,omitempty"`
}

type memoryItemVO struct {
	ID              string     `json:"id"`
	UserID          string     `json:"userId"`
	ScopeType       string     `json:"scopeType"`
	ScopeID         string     `json:"scopeId,omitempty"`
	MemoryType      string     `json:"memoryType"`
	SourceMessageID string     `json:"sourceMessageId,omitempty"`
	Content         string     `json:"content"`
	Summary         string     `json:"summary,omitempty"`
	Confidence      float64    `json:"confidence"`
	Status          string     `json:"status"`
	LastConfirmedAt *time.Time `json:"lastConfirmedAt,omitempty"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
	CreateTime      *time.Time `json:"createTime,omitempty"`
	UpdateTime      *time.Time `json:"updateTime,omitempty"`
}

// ListConversations 返回当前登录用户的会话列表。
func (h *Handler) ListConversations(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.conversationService.List(c.Request.Context(), ragservice.ListConversationsInput{
		UserID: user.UserID,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]conversationVO, 0, len(items))
	for _, item := range items {
		result = append(result, toConversationVO(item))
	}
	writeSuccess(c, result)
}

// ListMessages 返回当前会话的消息列表。
func (h *Handler) ListMessages(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.messageService.ListMessages(c.Request.Context(), ragservice.ListConversationMessagesInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
		Order:          port.ConversationMessageOrderAsc,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]messageVO, 0, len(items))
	for _, item := range items {
		result = append(result, toMessageVO(item))
	}
	writeSuccess(c, result)
}

// RenameConversation 修改会话标题。
func (h *Handler) RenameConversation(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req renameConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.conversationService.Rename(c.Request.Context(), ragservice.RenameConversationInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
		Title:          req.Title,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// DeleteConversation 删除一个会话及其消息。
func (h *Handler) DeleteConversation(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	if err := h.conversationService.Delete(c.Request.Context(), ragservice.DeleteConversationInput{
		ConversationID: c.Param("conversationId"),
		UserID:         user.UserID,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// SubmitFeedback 保存一条 assistant 消息反馈。
func (h *Handler) SubmitFeedback(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.feedbackService.Submit(c.Request.Context(), ragservice.SubmitMessageFeedbackInput{
		MessageID: c.Param("messageId"),
		UserID:    user.UserID,
		Vote:      req.Vote,
	}); err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess[any](c, nil)
}

// Chat 通过 SSE 输出最小聊天闭环的流式结果。
func (h *Handler) Chat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}

	sender := fwweb.NewSseEmitterSender(c)
	sink := &sseChatSink{sender: sender}
	if err := h.chatService.Chat(c.Request.Context(), ragservice.RagChatInput{
		ConversationID:   strings.TrimSpace(c.Query("conversationId")),
		UserID:           user.UserID,
		Question:         strings.TrimSpace(c.Query("question")),
		KnowledgeBaseIDs: splitCommaValues(c.Query("knowledgeBaseId")),
		DeepThinking:     parseBool(c.Query("deepThinking")),
	}, sink); err != nil {
		return
	}
}

// StopChat 取消一个正在执行的聊天任务。
func (h *Handler) StopChat(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	if !h.chatService.CancelTask(taskID) {
		_ = c.Error(exception.NewClientException("chat task not found", nil))
		return
	}
	writeSuccess[any](c, nil)
}

type sseChatSink struct {
	sender *fwweb.SseEmitterSender
}

// SendMeta 发送 SSE meta 事件。
func (s *sseChatSink) SendMeta(meta ragservice.RagChatMeta) error {
	return s.sender.SendEvent("meta", meta)
}

// SendFallback 发送检索回退通知事件。
func (s *sseChatSink) SendFallback(reason string) error {
	return s.sender.SendEvent("fallback", gin.H{"reason": reason})
}

// SendAgentThink 发送 agent 观察/继续规划事件。
func (s *sseChatSink) SendAgentThink(message string) error {
	return s.sender.SendEvent("agent_think", gin.H{"message": message})
}

func (s *sseChatSink) SendMemoryStored(payload ragservice.RagChatMemoryStoredPayload) error {
	return s.sender.SendEvent("memory_stored", payload)
}

func (s *sseChatSink) SendSessionRecall(payload ragservice.RagChatSessionRecallPayload) error {
	return s.sender.SendEvent("session_recall", payload)
}

// SendThinking 发送 SSE thinking 事件。
func (s *sseChatSink) SendThinking(delta string) error {
	return s.sender.SendEvent("message", gin.H{
		"type":  "think",
		"delta": delta,
	})
}

// SendMessage 发送 SSE response 事件。
func (s *sseChatSink) SendMessage(delta string) error {
	return s.sender.SendEvent("message", gin.H{
		"type":  "response",
		"delta": delta,
	})
}

// SendToolStart 发送单个 tool 开始事件。
func (s *sseChatSink) SendToolStart(payload ragtool.ToolCallEvent) error {
	return s.sender.SendEvent("tool_start", payload)
}

// SendToolResult 发送单个 tool 结果事件。
func (s *sseChatSink) SendToolResult(payload ragtool.ToolCallEvent) error {
	return s.sender.SendEvent("tool_result", payload)
}

// SendTool 发送单个 tool 调用摘要事件。
func (s *sseChatSink) SendTool(name string, status string, summary string) error {
	return s.sender.SendEvent("tool", gin.H{
		"name":    name,
		"status":  status,
		"summary": summary,
	})
}

// SendTitle 发送会话标题事件。
func (s *sseChatSink) SendTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	return s.sender.SendEvent("title", gin.H{"title": title})
}

// SendFinish 发送完成事件。
func (s *sseChatSink) SendFinish(payload ragservice.RagChatFinishPayload) error {
	return s.sender.SendEvent("finish", gin.H{
		"messageId": payload.MessageID,
		"title":     payload.Title,
	})
}

// SendCancel 发送取消事件。
func (s *sseChatSink) SendCancel(payload ragservice.RagChatFinishPayload) error {
	return s.sender.SendEvent("cancel", gin.H{
		"messageId": payload.MessageID,
		"title":     payload.Title,
	})
}

// SendError 发送错误事件。
func (s *sseChatSink) SendError(err error) error {
	if err == nil {
		return nil
	}
	return s.sender.SendEvent("error", gin.H{"error": err.Error()})
}

// SendDone 发送 done 事件并关闭 SSE。
func (s *sseChatSink) SendDone() error {
	if err := s.sender.SendEvent("done", gin.H{}); err != nil {
		return err
	}
	s.sender.Complete()
	return nil
}

// requireLoginUser 提取当前登录用户。
func requireLoginUser(c *gin.Context) *contextx.LoginUser {
	user := contextx.Get(c)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		_ = c.Error(exception.NewClientException("unauthorized", nil))
		return nil
	}
	return user
}

// splitCommaValues 按逗号拆分查询参数。
func splitCommaValues(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseBool 解析查询参数中的布尔值。
func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

// toConversationVO 转换会话出参。
func toConversationVO(item domain.Conversation) conversationVO {
	return conversationVO{
		ConversationID: item.ConversationID,
		Title:          item.Title,
		LastTime:       item.LastTime,
	}
}

// toMessageVO 转换消息出参。
func toMessageVO(item ragservice.ConversationMessageView) messageVO {
	return messageVO{
		ID:               item.ID,
		ConversationID:   item.ConversationID,
		Role:             string(item.Role),
		Content:          item.Content,
		RawContent:       item.RawContent,
		ContentSummary:   item.ContentSummary,
		IsSummarized:     item.IsSummarized,
		ThinkingContent:  item.ThinkingContent,
		ThinkingDuration: item.ThinkingDuration,
		Vote:             item.Vote,
		CreateTime:       timePointer(item.CreateTime),
	}
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

func (h *Handler) ListMemories(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	items, err := h.memoryService.ListMemories(c.Request.Context(), ragservice.ListMemoriesInput{
		UserID:     user.UserID,
		ScopeType:  c.Query("scopeType"),
		ScopeID:    c.Query("scopeId"),
		MemoryType: c.Query("memoryType"),
		Status:     c.Query("status"),
		Page:       parsePositiveInt(c.Query("current"), 1),
		PageSize:   parsePositiveInt(c.Query("size"), 20),
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := make([]memoryItemVO, 0, len(items))
	for _, item := range items {
		result = append(result, toMemoryItemVO(item))
	}
	writeSuccess(c, result)
}

func (h *Handler) Remember(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req rememberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	item, err := h.memoryService.SaveExplicitMemory(c.Request.Context(), ragservice.SaveExplicitMemoryInput{
		UserID:          user.UserID,
		ScopeType:       req.ScopeType,
		ScopeID:         req.ScopeID,
		MemoryType:      req.MemoryType,
		SourceMessageID: req.SourceMessageID,
		Content:         req.Content,
		Summary:         req.Summary,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toMemoryItemVO(item))
}

func (h *Handler) ExpireMemory(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	item, err := h.memoryService.ExpireMemory(c.Request.Context(), user.UserID, c.Param("memoryId"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toMemoryItemVO(item))
}

func toMemoryItemVO(item domain.MemoryItem) memoryItemVO {
	return memoryItemVO{
		ID:              item.ID,
		UserID:          item.UserID,
		ScopeType:       item.ScopeType,
		ScopeID:         item.ScopeID,
		MemoryType:      item.MemoryType,
		SourceMessageID: item.SourceMessageID,
		Content:         item.Content,
		Summary:         item.Summary,
		Confidence:      item.Confidence,
		Status:          item.Status,
		LastConfirmedAt: item.LastConfirmedAt,
		ExpiresAt:       item.ExpiresAt,
		CreateTime:      timePointer(item.CreateTime),
		UpdateTime:      timePointer(item.UpdateTime),
	}
}
