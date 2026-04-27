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
