package chat

import (
	"context"

	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/model"
)

type ChatClient interface {
	Provider() string

	Chat(request convention.ChatRequest, target model.ModelTarget) (string, error)

	StreamChat(request convention.ChatRequest, callback StreamCallback, target model.ModelTarget) (StreamCancellationHandle, error)
}

// UsageAwareChatClient is an optional extension for chat clients that can return
// provider usage on non-streaming responses.
type UsageAwareChatClient interface {
	ChatClient
	ChatWithUsage(request convention.ChatRequest, target model.ModelTarget) (string, TokenUsage, error)
}

// ContextAwareChatClient is an optional extension for synchronous chat calls
// that should honor caller context.
type ContextAwareChatClient interface {
	ChatClient
	ChatContext(ctx context.Context, request convention.ChatRequest, target model.ModelTarget) (string, error)
}

// ContextAwareUsageAwareChatClient is an optional extension for usage-aware
// synchronous chat calls that should honor caller context.
type ContextAwareUsageAwareChatClient interface {
	UsageAwareChatClient
	ChatWithUsageContext(ctx context.Context, request convention.ChatRequest, target model.ModelTarget) (string, TokenUsage, error)
}
