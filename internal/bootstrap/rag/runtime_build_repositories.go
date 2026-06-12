package rag

import (
	"gorm.io/gorm"

	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresuser "local/rag-project/internal/adapter/repository/postgres/user"
)

type repositoriesBundle struct {
	conversationRepo        *postgresrag.ConversationRepository
	messageRepo             *postgresrag.ConversationMessageRepository
	summaryRepo             *postgresrag.ConversationSummaryRepository
	feedbackRepo            *postgresrag.MessageFeedbackRepository
	memoryItemRepo          *postgresrag.MemoryItemRepository
	memoryItemEmbeddingRepo *postgresrag.MemoryItemEmbeddingRepository
	sessionChunkRepo        *postgresrag.SessionChunkRepository
	traceRunRepo            *postgresrag.RagTraceRunRepository
	traceNodeRepo           *postgresrag.RagTraceNodeRepository
	userRepo                *postgresuser.UserRepository
}

func buildRepositories(db *gorm.DB) repositoriesBundle {
	return repositoriesBundle{
		conversationRepo:        postgresrag.NewConversationRepository(db),
		messageRepo:             postgresrag.NewConversationMessageRepository(db),
		summaryRepo:             postgresrag.NewConversationSummaryRepository(db),
		feedbackRepo:            postgresrag.NewMessageFeedbackRepository(db),
		memoryItemRepo:          postgresrag.NewMemoryItemRepository(db),
		memoryItemEmbeddingRepo: postgresrag.NewMemoryItemEmbeddingRepository(db),
		sessionChunkRepo:        postgresrag.NewSessionChunkRepository(db),
		traceRunRepo:            postgresrag.NewRagTraceRunRepository(db),
		traceNodeRepo:           postgresrag.NewRagTraceNodeRepository(db),
		userRepo:                postgresuser.NewUserRepository(db),
	}
}
