package chat

import (
	"local/rag-project/internal/framework/convention"
)

type LLMService interface {
	Chat(prompt string) (string, error)

	ChatWithRequest(request convention.ChatRequest) (string, error)

	ChatWithModel(request convention.ChatRequest, modelID string) (string, error)

	StreamChat(prompt string, callback StreamCallback) (StreamCancellationHandle, error)

	StreamChatWithRequest(request convention.ChatRequest, callback StreamCallback) (StreamCancellationHandle, error)
}

// UsageAwareLLMService is an optional extension for non-streaming chat calls that
// can return provider usage when available.
type UsageAwareLLMService interface {
	LLMService
	ChatWithRequestUsage(request convention.ChatRequest) (string, TokenUsage, error)
}
