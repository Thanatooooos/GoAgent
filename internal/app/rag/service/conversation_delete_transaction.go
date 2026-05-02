package service

import (
	"context"

	"local/rag-project/internal/app/rag/port"
)

// ConversationDeleteTransaction 包裹会话及其关联消息、摘要的级联删除事务。
type ConversationDeleteTransaction func(
	ctx context.Context,
	fn func(
		ctx context.Context,
		conversationRepo port.ConversationRepository,
		messageRepo port.ConversationMessageRepository,
		summaryRepo port.ConversationSummaryRepository,
	) error,
) error
