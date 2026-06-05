package rag

import (
	"github.com/gin-gonic/gin"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/middleware"
)

// RegisterRoutes 注册最小 RAG 闭环相关路由。
func RegisterRoutes(
	r gin.IRouter,
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService chatService,
	traceService *ragservice.TraceService,
	cacheMetrics *ragcachemetrics.Service,
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
	r.POST("/rag/v3/chat/approval/resume", handler.ResumeAfterApproval)
	r.POST("/rag/v3/stop", handler.StopChat)

	admin := r.Group("/")
	admin.Use(middleware.RequireRole("admin"))
	RegisterTraceRoutes(admin, traceService)
	RegisterMemoryCacheMetricsRoutes(admin, cacheMetrics)
}
