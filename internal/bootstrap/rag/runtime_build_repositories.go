package rag

import (
	"gorm.io/gorm"

	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
)

type repositoriesBundle struct {
	conversationRepo             *postgresrag.ConversationRepository
	messageRepo                  *postgresrag.ConversationMessageRepository
	summaryRepo                  *postgresrag.ConversationSummaryRepository
	feedbackRepo                 *postgresrag.MessageFeedbackRepository
	memoryItemRepo               *postgresrag.MemoryItemRepository
	memoryItemEmbeddingRepo      *postgresrag.MemoryItemEmbeddingRepository
	userMemoryProfileRepo        *postgresrag.UserMemoryProfileRepository
	conversationProfileStateRepo *postgresrag.ConversationProfileStateRepository
	sessionChunkRepo             *postgresrag.SessionChunkRepository
}

func buildRepositories(db *gorm.DB) repositoriesBundle {
	return repositoriesBundle{
		conversationRepo:             postgresrag.NewConversationRepository(db),
		messageRepo:                  postgresrag.NewConversationMessageRepository(db),
		summaryRepo:                  postgresrag.NewConversationSummaryRepository(db),
		feedbackRepo:                 postgresrag.NewMessageFeedbackRepository(db),
		memoryItemRepo:               postgresrag.NewMemoryItemRepository(db),
		memoryItemEmbeddingRepo:      postgresrag.NewMemoryItemEmbeddingRepository(db),
		userMemoryProfileRepo:        postgresrag.NewUserMemoryProfileRepository(db),
		conversationProfileStateRepo: postgresrag.NewConversationProfileStateRepository(db),
		sessionChunkRepo:             postgresrag.NewSessionChunkRepository(db),
	}
}
