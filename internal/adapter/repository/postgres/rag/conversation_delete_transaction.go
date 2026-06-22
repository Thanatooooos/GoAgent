package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/port"
)

// NewConversationDeleteTransaction 创建会话级联删除事务包装器。
func NewConversationDeleteTransaction(db *gorm.DB) port.ConversationDeleteTransaction {
	return func(
		ctx context.Context,
		fn func(
			ctx context.Context,
			conversationRepo port.ConversationRepository,
			messageRepo port.ConversationMessageRepository,
			summaryRepo port.ConversationSummaryRepository,
		) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(
				ctx,
				NewConversationRepository(tx),
				NewConversationMessageRepository(tx),
				NewConversationSummaryRepository(tx),
			)
		})
	}
}
