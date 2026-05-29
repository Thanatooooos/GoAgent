package rag

import (
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

// Handler 负责承接最小 RAG 闭环的 HTTP 请求。
type Handler struct {
	conversationService *ragservice.ConversationService
	messageService      *ragservice.ConversationMessageService
	memoryService       *longtermmemory.MemoryService
	feedbackService     *ragservice.MessageFeedbackService
	chatService         *ragservice.RagChatService
}

// NewHandler 创建 RAG HTTP 处理器。
func NewHandler(
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
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
