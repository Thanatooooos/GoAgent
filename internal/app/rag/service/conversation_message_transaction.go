package service

import (
	"context"

	"local/rag-project/internal/app/rag/port"
)

// ConversationMessageCreateTransaction wraps message creation and optional session chunk persistence
// so they can share the same storage transaction.
type ConversationMessageCreateTransaction func(
	ctx context.Context,
	fn func(
		ctx context.Context,
		messageRepo port.ConversationMessageRepository,
		chunkSink ConversationMessageChunkSink,
	) error,
) error
