package rag

import (
	"context"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

type chatService interface {
	Chat(ctx context.Context, input ragservice.RagChatInput, sink ragservice.RagChatEventSink) error
	ResumeAfterApproval(ctx context.Context, input ragservice.RagChatApprovalResumeInput, sink ragservice.RagChatEventSink) error
	GetPendingApproval(ctx context.Context, input ragservice.RagChatApprovalPendingQueryInput) (*ragservice.RagChatApprovalPendingPayload, error)
	CancelTask(taskID string) bool
}

// Handler 负责承接最小 RAG 闭环的 HTTP 请求。
type Handler struct {
	conversationService *ragservice.ConversationService
	messageService      *ragservice.ConversationMessageService
	memoryService       *longtermmemory.MemoryService
	feedbackService     *ragservice.MessageFeedbackService
	chatService         chatService
	preferenceCandidateService longtermmemory.PreferenceCandidateService
}

// NewHandler 创建 RAG HTTP 处理器。
func NewHandler(
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService chatService,
	preferenceCandidateService longtermmemory.PreferenceCandidateService,
) *Handler {
	return &Handler{
		conversationService: conversationService,
		messageService:      messageService,
		memoryService:       memoryService,
		feedbackService:     feedbackService,
		chatService:         chatService,
		preferenceCandidateService: preferenceCandidateService,
	}
}
