package port

import (
	"context"

	"local/rag-project/internal/app/rag/domain"
)

// ProcessedConversationMessageChunk is the normalized chunk payload produced while
// persisting a long or summarized conversation message.
type ProcessedConversationMessageChunk struct {
	ChunkIndex     int
	Content        string
	ContentSummary string
	TokenEstimate  int
}

// ConversationMessageChunkSink persists session chunks for a conversation message.
type ConversationMessageChunkSink interface {
	PersistMessageChunks(ctx context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error
}

// ConversationMessageCreateTransaction wraps message creation and optional session
// chunk persistence so they can share the same storage transaction.
type ConversationMessageCreateTransaction func(
	ctx context.Context,
	fn func(
		ctx context.Context,
		messageRepo ConversationMessageRepository,
		chunkSink ConversationMessageChunkSink,
	) error,
) error
