package service

import (
	"context"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	"local/rag-project/internal/framework/exception"
)

func (s *RagChatService) GetPendingApproval(ctx context.Context, input RagChatApprovalPendingQueryInput) (*RagChatApprovalPendingPayload, error) {
	if s == nil {
		return nil, exception.NewServiceException("rag chat service is required", nil)
	}
	if s.agentRuntime == nil {
		return nil, exception.NewServiceException("agent runtime service is required", nil)
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		return nil, exception.NewClientException("conversation id is required", nil)
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil, exception.NewClientException("user id is required", nil)
	}

	pending, ok, err := s.agentRuntime.GetPendingApproval(ctx, agentapp.PendingApprovalLookupRequest{
		ConversationID: conversationID,
		UserID:         userID,
	})
	if err != nil || !ok || pending == nil {
		return nil, err
	}
	return newRagChatApprovalPendingPayload(pending), nil
}
