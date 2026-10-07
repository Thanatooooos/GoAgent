package rag

import (
	"github.com/gin-gonic/gin"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	runtimetrace "local/rag-project/internal/app/runtime/trace"
	"local/rag-project/internal/framework/stream"
	"local/rag-project/internal/middleware"
)

// RegisterRoutes 注册最小 RAG 闭环相关路由。
func RegisterRoutes(
	r gin.IRouter,
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	preferenceCandidateService longtermmemory.PreferenceCandidateService,
	traceService *runtimetrace.Service,
	cacheMetrics *ragcachemetrics.Service,
	streamManager stream.StreamManager,
	runtimeChats ...runtimeChatService,
) {
	handler := NewHandler(conversationService, messageService, memoryService, feedbackService, preferenceCandidateService, streamManager)
	if len(runtimeChats) > 0 {
		handler.SetRuntimeChat(runtimeChats[0])
		if replay, ok := runtimeChats[0].(runtimeReplayService); ok {
			handler.SetRuntimeReplay(replay)
		}
	}
	r.GET("/conversations", handler.ListConversations)
	r.GET("/conversations/:conversationId/messages", handler.ListMessages)
	r.PUT("/conversations/:conversationId", handler.RenameConversation)
	r.DELETE("/conversations/:conversationId", handler.DeleteConversation)
	r.GET("/rag/v3/preferences/candidates/pending", handler.ListPendingPreferenceCandidates)
	r.POST("/rag/v3/preferences/candidates/:candidateId/confirm", handler.ConfirmPreferenceCandidate)
	r.POST("/rag/v3/preferences/candidates/:candidateId/reject", handler.RejectPreferenceCandidate)
	r.GET("/rag/v3/memories", handler.ListMemories)
	r.POST("/rag/v3/memories", handler.Remember)
	r.POST("/rag/v3/remember", handler.Remember)
	r.POST("/rag/v3/memories/:memoryId/expire", handler.ExpireMemory)
	r.POST("/conversations/messages/:messageId/feedback", handler.SubmitFeedback)
	r.GET("/rag/v3/chat", handler.Chat)
	r.POST("/rag/v3/chat", handler.Chat)
	r.GET("/rag/v3/chat/continue", handler.ContinueChat)
	r.POST("/rag/v3/stop", handler.StopChat)

	admin := r.Group("/")
	admin.Use(middleware.RequireRole("admin"))
	RegisterTraceRoutes(admin, traceService)
	RegisterMemoryCacheMetricsRoutes(admin, cacheMetrics)
}
