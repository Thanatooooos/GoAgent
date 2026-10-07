package runtimeadapter

import (
	"context"
	"fmt"

	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	ragservice "local/rag-project/internal/app/rag/service"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/convention"
)

type ConversationMessages struct {
	service *ragservice.ConversationMessageService
	chunks  *postgresrag.ConversationMessageChunkSink
}

func NewConversationMessages(service *ragservice.ConversationMessageService, chunks ...*postgresrag.ConversationMessageChunkSink) ConversationMessages {
	result := ConversationMessages{service: service}
	if len(chunks) > 0 {
		result.chunks = chunks[0]
	}
	return result
}

func (s ConversationMessages) PreparePublication(ctx context.Context, message conversationruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
	if s.service == nil {
		return postgresruntime.PreparedPublication{}, fmt.Errorf("message service is required")
	}
	role, err := conversationRole(message.Role)
	if err != nil {
		return postgresruntime.PreparedPublication{}, err
	}
	prepared, chunks, err := s.service.PrepareMessage(ctx, ragservice.AddConversationMessageInput{ConversationID: message.ConversationID, UserID: message.UserID, Role: role, Content: message.Content})
	if err != nil {
		return postgresruntime.PreparedPublication{}, err
	}
	batch, err := s.chunks.PrepareChunks(ctx, prepared, chunks)
	return postgresruntime.PreparedPublication{Message: prepared, Chunks: batch}, err
}

func (s ConversationMessages) Create(ctx context.Context, message conversationruntime.ConversationMessage) (conversationruntime.ConversationMessage, error) {
	if s.service == nil {
		return conversationruntime.ConversationMessage{}, fmt.Errorf("conversation message service is required")
	}
	role, err := conversationRole(message.Role)
	if err != nil {
		return conversationruntime.ConversationMessage{}, err
	}
	created, err := s.service.AddMessage(ctx, ragservice.AddConversationMessageInput{
		ConversationID: message.ConversationID,
		UserID:         message.UserID,
		Role:           role,
		Content:        message.Content,
	})
	if err != nil {
		return conversationruntime.ConversationMessage{}, err
	}
	return conversationruntime.ConversationMessage{ID: created.ID, ConversationID: created.ConversationID, UserID: created.UserID, Role: message.Role, Content: created.DisplayContent(), Sources: created.Sources}, nil
}

func conversationRole(role conversationruntime.ModelRole) (convention.Role, error) {
	switch role {
	case conversationruntime.ModelRoleUser:
		return convention.UserRole, nil
	case conversationruntime.ModelRoleAssistant:
		return convention.AssistantRole, nil
	default:
		return "", fmt.Errorf("conversation message role %q is not supported", role)
	}
}

var _ conversationruntime.ConversationMessages = ConversationMessages{}
