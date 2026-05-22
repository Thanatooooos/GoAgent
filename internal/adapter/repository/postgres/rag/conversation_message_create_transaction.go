package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service"
	infraembedding "local/rag-project/internal/infra-ai/embedding"
)

// NewConversationMessageCreateTransaction wraps message creation and session chunk persistence
// in a single database transaction.
func NewConversationMessageCreateTransaction(
	db *gorm.DB,
	embedding infraembedding.EmbeddingService,
) service.ConversationMessageCreateTransaction {
	return func(
		ctx context.Context,
		fn func(
			ctx context.Context,
			messageRepo port.ConversationMessageRepository,
			chunkSink service.ConversationMessageChunkSink,
		) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(
				ctx,
				NewConversationMessageRepository(tx),
				NewConversationMessageChunkSink(tx, embedding),
			)
		})
	}
}
