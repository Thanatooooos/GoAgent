// Package service is the stable application-facing entry for RAG services.
// Implementation lives in responsibility-focused subpackages; this root package
// keeps constructor and type aliases for existing callers.
package service

import (
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	"local/rag-project/internal/app/rag/port"
	ragconversation "local/rag-project/internal/app/rag/service/conversation"
	ragsessionrecall "local/rag-project/internal/app/rag/service/sessionrecall"
	aichat "local/rag-project/internal/infra-ai/chat"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type (
	ConversationService                 = ragconversation.ConversationService
	CreateOrUpdateConversationInput     = ragconversation.CreateOrUpdateConversationInput
	RenameConversationInput             = ragconversation.RenameConversationInput
	DeleteConversationInput             = ragconversation.DeleteConversationInput
	ListConversationsInput              = ragconversation.ListConversationsInput
	ConversationMessageService          = ragconversation.MessageService
	AddConversationMessageInput         = ragconversation.AddConversationMessageInput
	ListConversationMessagesInput       = ragconversation.ListConversationMessagesInput
	AddConversationSummaryInput         = ragconversation.AddConversationSummaryInput
	GetLatestConversationSummaryInput   = ragconversation.GetLatestConversationSummaryInput
	ConversationMessageView             = ragconversation.ConversationMessageView
	ProcessedConversationMessageContent = ragconversation.ProcessedConversationMessageContent
	ConversationMessageContentProcessor = ragconversation.ConversationMessageContentProcessor
	MessageFeedbackService              = ragconversation.MessageFeedbackService
	SubmitMessageFeedbackInput          = ragconversation.SubmitMessageFeedbackInput

	ConversationDeleteTransaction        = port.ConversationDeleteTransaction
	ConversationMessageCreateTransaction = port.ConversationMessageCreateTransaction
	ConversationMessageChunkSink         = port.ConversationMessageChunkSink
	ProcessedConversationMessageChunk    = port.ProcessedConversationMessageChunk

	TokenEstimator              = ragsessionrecall.TokenEstimator
	RoughTokenEstimator         = ragsessionrecall.RoughTokenEstimator
	SessionRecallService        = ragsessionrecall.SessionRecallService
	SessionRecallInput          = ragsessionrecall.SessionRecallInput
	SessionRecallHit            = ragsessionrecall.SessionRecallHit
	SessionRecallResult         = ragsessionrecall.SessionRecallResult
	SessionRecallOptions        = ragsessionrecall.SessionRecallOptions
	SessionRecallCacheOptions   = ragsessionrecall.SessionRecallCacheOptions
	LongMessageProcessorOptions = ragsessionrecall.LongMessageProcessorOptions
	LongMessageContentProcessor = ragsessionrecall.LongMessageContentProcessor
)

func NewConversationService(
	conversationRepo port.ConversationRepository,
	messageRepo port.ConversationMessageRepository,
	summaryRepo port.ConversationSummaryRepository,
	promptLoader *ragprompt.TemplateLoader,
	llmService aichat.LLMService,
	titleMaxLength int,
	deleteTx port.ConversationDeleteTransaction,
) *ConversationService {
	return ragconversation.NewConversationService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		promptLoader,
		llmService,
		titleMaxLength,
		deleteTx,
	)
}

func NewConversationMessageService(
	conversationRepo port.ConversationRepository,
	messageRepo port.ConversationMessageRepository,
	summaryRepo port.ConversationSummaryRepository,
	feedbackRepo port.MessageFeedbackRepository,
) *ConversationMessageService {
	return ragconversation.NewMessageService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		feedbackRepo,
	)
}

func NewMessageFeedbackService(
	messageRepo port.ConversationMessageRepository,
	feedbackRepo port.MessageFeedbackRepository,
) *MessageFeedbackService {
	return ragconversation.NewMessageFeedbackService(messageRepo, feedbackRepo)
}

func NewSessionRecallService(
	repo port.SessionChunkRepository,
	embedding aiembedding.EmbeddingService,
	options SessionRecallOptions,
) SessionRecallService {
	return ragsessionrecall.NewSessionRecallService(repo, embedding, options)
}

func NewLongMessageContentProcessor(options LongMessageProcessorOptions) *LongMessageContentProcessor {
	return ragsessionrecall.NewLongMessageContentProcessor(options)
}
