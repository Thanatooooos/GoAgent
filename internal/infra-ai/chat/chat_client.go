package chat

import (
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
