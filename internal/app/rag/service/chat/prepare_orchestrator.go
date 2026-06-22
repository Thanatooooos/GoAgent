package chat

import (
	"context"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/log"
)

const defaultTopK = 5

const (
	subQuestionStatusSuccess = "success"
	subQuestionStatusEmpty   = "empty"
	subQuestionStatusFailed  = "failed"
	subQuestionStatusCancel  = "cancelled"

	retrieveExecutionModeSerial               = "serial"
	retrieveExecutionModeParallel             = "parallel"
	retrieveExecutionModeSerialDependencyRisk = "serial_due_to_dependency_risk"
	defaultSubquestionConcurrency             = 2
)

type subQuestionRetrieveResult struct {
	Query        string
	Status       string
	DurationMs   int64
	ChunkCount   int
	Error        string
	FallbackUsed bool
	Result       ragretrieve.Result
}

func (s *RagChatService) prepareChat(ctx context.Context, input RagChatInput) (ragChatPreparedState, error) {
	conversationStage, err := s.runConversationStage(ctx, input)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	memoryStage, err := s.runMemoryStage(ctx, conversationStage.conversationID, strings.TrimSpace(input.UserID))
	if err != nil {
		return ragChatPreparedState{}, err
	}

	userMessageStage, err := s.runUserMessageStage(ctx, input, conversationStage.conversationID)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	runtimeStage, err := s.runRuntimeStage(ctx, input, conversationStage, userMessageStage)
	if err != nil {
		return ragChatPreparedState{}, err
	}
	ctx = enrichRagChatLogContext(
		ctx,
		runtimeStage.state.traceID,
		conversationStage.conversationID,
		input.UserID,
		runtimeStage.state.meta.TaskID,
	)

	rewriteStage, err := s.runRewriteStage(ctx, input.Question, memoryStage.history, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	longTermMemoryStage, err := s.runLongTermMemoryStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		log.FromContext(ctx).Warnw("rag chat long-term memory stage failed open", "error", err)
		longTermMemoryStage = ragChatLongTermMemoryStageResult{}
	}

	sessionRecallStage, err := s.runSessionRecallStage(ctx, conversationStage.conversationID, input, userMessageStage.message.ID, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		log.FromContext(ctx).Warnw("rag chat session recall stage failed open", "error", err)
		sessionRecallStage = ragChatSessionRecallStageResult{}
	}

	retrieveStage, err := s.runRetrieveStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}
	return ragChatPreparedState{
		state:          runtimeStage.state,
		history:        memoryStage.history,
		userMessage:    userMessageStage.message,
		rewriteResult:  rewriteStage.result,
		memoryContext:  longTermMemoryStage.result.Context,
		sessionRecall:  sessionRecallStage.result,
		sessionContext: sessionRecallStage.result.Context,
		retrieveResult: retrieveStage.result,
		retrievalUsed:  retrieveStage.used,
	}, nil
}
