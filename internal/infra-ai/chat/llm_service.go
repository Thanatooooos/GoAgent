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
