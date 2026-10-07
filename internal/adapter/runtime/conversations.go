package runtimeadapter

import (
	"context"
	"fmt"

	ragservice "local/rag-project/internal/app/rag/service"
	conversationruntime "local/rag-project/internal/app/runtime"
)

type Conversations struct {
	service *ragservice.ConversationService
}

func NewConversations(service *ragservice.ConversationService) Conversations {
	return Conversations{service: service}
}

func (s Conversations) Ensure(ctx context.Context, conversationID, userID, question string) error {
	if s.service == nil {
		return fmt.Errorf("conversation service is required")
	}
	_, err := s.service.CreateOrUpdate(ctx, ragservice.CreateOrUpdateConversationInput{ConversationID: conversationID, UserID: userID, Question: question})
	return err
}

var _ conversationruntime.Conversations = Conversations{}
