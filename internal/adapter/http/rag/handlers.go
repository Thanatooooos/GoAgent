package rag

import (
	"context"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/stream"
)

type runtimeChatService interface {
	Admit(context.Context, conversationruntime.ChatInput) (conversationruntime.ChatInput, error)
	AuthorizeTask(context.Context, string, string) (bool, error)
	Chat(context.Context, conversationruntime.ChatInput, conversationruntime.EventSink) (conversationruntime.ChatResult, error)
	CancelTask(string) bool
}

type runtimeReplayService interface {
	Replay(context.Context, string, string, conversationruntime.EventSink) (bool, error)
}

// Handler 负责承接最小 RAG 闭环的 HTTP 请求。
type Handler struct {
	conversationService        *ragservice.ConversationService
	messageService             *ragservice.ConversationMessageService
	memoryService              *longtermmemory.MemoryService
	feedbackService            *ragservice.MessageFeedbackService
	runtimeChat                runtimeChatService
	runtimeReplay              runtimeReplayService
	preferenceCandidateService longtermmemory.PreferenceCandidateService
	streamManager              stream.StreamManager
}

func (h *Handler) SetRuntimeChat(service runtimeChatService)     { h.runtimeChat = service }
func (h *Handler) SetRuntimeReplay(service runtimeReplayService) { h.runtimeReplay = service }

// NewHandler 创建 RAG HTTP 处理器。
func NewHandler(
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	preferenceCandidateService longtermmemory.PreferenceCandidateService,
	streamManager stream.StreamManager,
) *Handler {
	if streamManager == nil {
		streamManager = stream.NewMemoryStreamManager()
	}
	return &Handler{
		conversationService:        conversationService,
		messageService:             messageService,
		memoryService:              memoryService,
		feedbackService:            feedbackService,
		preferenceCandidateService: preferenceCandidateService,
		streamManager:              streamManager,
	}
}
