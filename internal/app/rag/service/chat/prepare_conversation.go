package chat

import (
	"context"
	"strings"

	"local/rag-project/internal/framework/convention"
)

func (s *RagChatService) runConversationStage(ctx context.Context, input RagChatInput) (ragChatConversationStageResult, error) {
	question := strings.TrimSpace(input.Question)
	userID := strings.TrimSpace(input.UserID)
	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		nextID, err := nextConversationExternalID()
		if err != nil {
			return ragChatConversationStageResult{}, err
		}
		conversationID = nextID
	}

	conversation, err := s.conversationService.CreateOrUpdate(ctx, CreateOrUpdateConversationInput{
		ConversationID: conversationID,
		UserID:         userID,
		Question:       question,
	})
	if err != nil {
		return ragChatConversationStageResult{}, err
	}

	return ragChatConversationStageResult{
		conversationID: conversationID,
		conversation:   conversation,
	}, nil
}

func (s *RagChatService) runUserMessageStage(ctx context.Context, input RagChatInput, conversationID string) (ragChatUserMessageStageResult, error) {
	userMessage, err := s.messageService.AddMessage(ctx, AddConversationMessageInput{
		ConversationID: conversationID,
		UserID:         strings.TrimSpace(input.UserID),
		Role:           convention.UserRole,
		Content:        strings.TrimSpace(input.Question),
	})
	if err != nil {
		return ragChatUserMessageStageResult{}, err
	}
	return ragChatUserMessageStageResult{message: userMessage}, nil
}
