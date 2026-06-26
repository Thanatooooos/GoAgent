package history

import (
	"context"

	"local/rag-project/internal/framework/convention"
)

type Store interface {
	// LoadHistory 读取指定会话最近一段历史消息。
	LoadHistory(ctx context.Context, conversationID string, userID string, limit int) ([]convention.ChatMessage, error)
	// Append 追加一条会话消息，并返回新消息 ID。
	Append(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) (string, error)
	// RefreshCache 为后续缓存型实现预留刷新入口。
	RefreshCache(ctx context.Context, conversationID string, userID string) error
}

type SummaryService interface {
	// LoadLatestSummary 读取指定会话最近的一条摘要。
	LoadLatestSummary(ctx context.Context, conversationID string, userID string) (*convention.ChatMessage, error)
	// DecorateIfNeeded 在摘要进入上下文前补齐统一包装。
	DecorateIfNeeded(summary *convention.ChatMessage) *convention.ChatMessage
	// CompressIfNeeded 为后续摘要压缩策略预留入口。
	CompressIfNeeded(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) error
}

type SummaryTrigger interface {
	EnqueueSummaryCheck(ctx context.Context, input SummaryJobInput) error
}

type Service interface {
	// Load 加载一段可直接喂给模型的会话上下文。
	Load(ctx context.Context, conversationID string, userID string) ([]convention.ChatMessage, error)
	// Append 将一条新消息写入记忆存储。
	Append(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) (string, error)
	// LoadAndAppend 先读取上下文，再把当前消息写入存储。
	LoadAndAppend(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) ([]convention.ChatMessage, error)
}
