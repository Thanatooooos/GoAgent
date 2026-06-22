package chat

import (
	"context"
	"strings"

	"local/rag-project/internal/framework/exception"
)

func (s *RagChatService) runMemoryStage(ctx context.Context, conversationID string, userID string) (ragChatMemoryStageResult, error) {
	history, err := s.historyService.Load(ctx, conversationID, userID)
	if err != nil {
		return ragChatMemoryStageResult{}, exception.NewServiceException("failed to load rag memory", err)
	}
	return ragChatMemoryStageResult{history: history}, nil
}

func (s *RagChatService) runRuntimeStage(
	ctx context.Context,
	input RagChatInput,
	conversationStage ragChatConversationStageResult,
	userMessageStage ragChatUserMessageStageResult,
) (ragChatRuntimeStageResult, error) {
	traceID, err := nextRagTraceID()
	if err != nil {
		return ragChatRuntimeStageResult{}, err
	}
	taskID, err := nextRagTaskID()
	if err != nil {
		return ragChatRuntimeStageResult{}, err
	}

	state := ragChatRuntimeState{
		meta: RagChatMeta{
			ConversationID: conversationStage.conversationID,
			TaskID:         taskID,
		},
		title:         conversationStage.conversation.Title,
		userMessageID: userMessageStage.message.ID,
		traceID:       traceID,
		startTime:     s.tracer.now(),
	}
	_ = s.tracer.startTraceRunAt(ctx, traceID, conversationStage.conversationID, taskID, strings.TrimSpace(input.UserID), state.startTime)

	return ragChatRuntimeStageResult{state: state}, nil
}
