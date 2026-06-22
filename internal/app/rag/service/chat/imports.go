package chat

import (
	"local/rag-project/internal/app/rag/port"
	ragconversation "local/rag-project/internal/app/rag/service/conversation"
	ragsessionrecall "local/rag-project/internal/app/rag/service/sessionrecall"
)

type (
	ConversationService               = ragconversation.ConversationService
	ConversationMessageService        = ragconversation.MessageService
	CreateOrUpdateConversationInput   = ragconversation.CreateOrUpdateConversationInput
	AddConversationMessageInput       = ragconversation.AddConversationMessageInput
	ProcessedConversationMessageChunk = port.ProcessedConversationMessageChunk

	SessionRecallService = ragsessionrecall.SessionRecallService
	SessionRecallResult  = ragsessionrecall.SessionRecallResult
	SessionRecallInput   = ragsessionrecall.SessionRecallInput
	SessionRecallHit     = ragsessionrecall.SessionRecallHit
	SessionRecallOptions = ragsessionrecall.SessionRecallOptions
	TokenEstimator       = ragsessionrecall.TokenEstimator
	RoughTokenEstimator  = ragsessionrecall.RoughTokenEstimator
)
