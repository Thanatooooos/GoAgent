package chat

import (
	"context"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

func (s *RagChatService) applyFallbackGuard(
	ctx context.Context,
	prepared ragChatPreparedState,
	question string,
	sink RagChatEventSink,
) (ragretrieve.Result, string) {
	fallbackPrompt := ""
	retrieveResult := prepared.retrieveResult
	if prepared.retrievalUsed && s.confidenceThreshold > 0 {
		maxScore := topChunkScore(retrieveResult)
		if maxScore < float32(s.confidenceThreshold) {
			fallbackReason := "low confidence retrieval, fallback to general model"
			fallbackPrompt = buildFallbackPrompt(question)
			retrieveResult.KnowledgeContext = ""
			_ = sink.SendFallback(fallbackReason)
			s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, map[string]any{
				"fallback": map[string]any{
					"triggered": true,
					"reason":    fallbackReason,
				},
			})
			_ = s.tracer.recordTraceNode(ctx, prepared.state.traceID, ragChatTraceNode{
				NodeID:   "fallback",
				NodeType: "fallback",
				NodeName: "fallback_to_general_model",
			}, ragTraceStatusSuccess, map[string]any{
				"reason": fallbackReason,
			})
		}
	}
	return retrieveResult, fallbackPrompt
}

func (s *RagChatService) runStreamingAnswer(
	ctx context.Context,
	state ragChatRuntimeState,
	messages []convention.ChatMessage,
	promptTokensEstimate int,
	deepThinking bool,
	sink RagChatEventSink,
) (ragChatTaskResult, error) {
	task := s.taskRegistry.New()
	s.taskRegistry.Set(state.meta.TaskID, task, nil)
	defer s.taskRegistry.Delete(state.meta.TaskID)

	request := convention.ChatRequest{
		Messages: messages,
	}
	if deepThinking {
		request.Thinking = boolPointer(true)
	}

	callback := newRagChatStreamCallback(
		task,
		sink,
		s.chatContextBudget.normalized().Estimator,
		promptTokensEstimate,
	)
	handle, err := s.chatService.StreamChatWithRequest(request, callback)
	if err != nil {
		return ragChatTaskResult{}, err
	}
	s.taskRegistry.Set(state.meta.TaskID, task, handle)

	return <-task.doneCh, nil
}

func (s *RagChatService) persistAssistantMessage(
	ctx context.Context,
	state ragChatRuntimeState,
	input RagChatInput,
	content string,
	thinking string,
) (RagChatFinishPayload, error) {
	content = strings.TrimSpace(content)
	thinking = strings.TrimSpace(thinking)
	if content == "" && thinking == "" {
		return RagChatFinishPayload{Title: state.title}, nil
	}

	thinkingDuration := 0
	if thinking != "" {
		thinkingDuration = 1
	}

	created, err := s.messageService.AddMessage(ctx, AddConversationMessageInput{
		ConversationID:  state.meta.ConversationID,
		UserID:          strings.TrimSpace(input.UserID),
		Role:            convention.AssistantRole,
		Content:         firstNonEmptyString(content, " "),
		ThinkingContent: thinking,
		ThinkingDuration: func() *int {
			if thinking == "" {
				return nil
			}
			return &thinkingDuration
		}(),
	})
	if err != nil {
		return RagChatFinishPayload{}, err
	}

	if _, err := s.conversationService.CreateOrUpdate(ctx, CreateOrUpdateConversationInput{
		ConversationID: state.meta.ConversationID,
		UserID:         strings.TrimSpace(input.UserID),
		Question:       strings.TrimSpace(input.Question),
		LastTime:       timePointerValue(s.tracer.now()),
	}); err != nil && strings.TrimSpace(input.Question) != "" {
		return RagChatFinishPayload{}, err
	}

	return RagChatFinishPayload{
		MessageID: created.ID,
		Title:     state.title,
	}, nil
}

func (s *RagChatService) handleCancelledResult(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.tracer.recordChatTraceNode(ctx, state.traceID, ragTraceStatusCancelled, result)
	ctx = enrichRagChatLogContext(ctx, state.traceID, state.meta.ConversationID, input.UserID, state.meta.TaskID)
	logRagChatCompletion(ctx, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		logRagChatTerminalError(ctx, "persist_cancelled_result", err)
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusCancelled, nil)
	_ = sink.SendCancel(payload)
	_ = sink.SendDone()
	return nil
}

func (s *RagChatService) handleFailedResult(
	ctx context.Context,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.tracer.recordChatTraceNode(ctx, state.traceID, ragTraceStatusFailed, result)
	ctx = enrichRagChatLogContext(ctx, state.traceID, state.meta.ConversationID, "", state.meta.TaskID)
	logRagChatCompletion(ctx, result)
	logRagChatTerminalError(ctx, "stream_result", result.err)
	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, result.err)
	_ = sink.SendError(result.err)
	_ = sink.SendDone()
	return result.err
}

func (s *RagChatService) triggerLongTermMemoryWriteback(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
) {
	if s.longTermMemoryWriteback == nil {
		return
	}

	writebackCtx := context.Background()
	if ctx != nil {
		writebackCtx = context.WithoutCancel(ctx)
	}
	writebackInput := LongTermMemoryWritebackInput{
		UserID:          strings.TrimSpace(input.UserID),
		Message:         strings.TrimSpace(input.Question),
		SourceMessageID: strings.TrimSpace(state.userMessageID),
	}

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logRagChatTerminalError(writebackCtx, "long_term_memory_writeback", fmt.Errorf("panic: %v", recovered))
			}
		}()
		s.longTermMemoryWriteback.CapturePreferenceCandidate(writebackCtx, writebackInput)
	}()
}

func (s *RagChatService) handleSucceededResult(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.tracer.recordChatTraceNode(ctx, state.traceID, ragTraceStatusSuccess, result)
	ctx = enrichRagChatLogContext(ctx, state.traceID, state.meta.ConversationID, input.UserID, state.meta.TaskID)
	logRagChatCompletion(ctx, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		logRagChatTerminalError(ctx, "persist_succeeded_result", err)
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusSuccess, nil)
	_ = sink.SendFinish(payload)
	_ = sink.SendDone()
	s.triggerLongTermMemoryWriteback(ctx, input, state)
	return nil
}
