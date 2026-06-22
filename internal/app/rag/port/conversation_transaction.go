package port

import "context"

// ConversationDeleteTransaction wraps cascading deletion of a conversation and its
// related messages and summaries inside one storage transaction.
type ConversationDeleteTransaction func(
	ctx context.Context,
	fn func(
		ctx context.Context,
		conversationRepo ConversationRepository,
		messageRepo ConversationMessageRepository,
		summaryRepo ConversationSummaryRepository,
	) error,
) error
