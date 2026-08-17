package chat

import (
	"context"
	"fmt"
	"strings"

	ragcitation "local/rag-project/internal/app/rag/core/citation"
	raghistory "local/rag-project/internal/app/rag/core/history"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/log"
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
	expander *ragcitation.StreamExpander,
	sink RagChatEventSink,
	task *ragChatTask,
) (ragChatTaskResult, error) {
	if task == nil {
		return ragChatTaskResult{}, fmt.Errorf("rag chat task is required")
	}
	if task.Context().Err() != nil {
		return ragChatTaskResult{cancelled: true}, nil
	}

	request := convention.ChatRequest{
		Messages: messages,
	}
	if deepThinking {
		request.Thinking = boolPointer(true)
	}

	const maxStreamAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxStreamAttempts; attempt++ {
		callback := newRagChatStreamCallback(
			task,
			sink,
			s.chatContextBudget.normalized().Estimator,
			promptTokensEstimate,
			expander,
		)
		handle, err := s.chatService.StreamChatWithRequest(request, callback)
		if err != nil {
			lastErr = err
			if task.Context().Err() != nil {
				return ragChatTaskResult{cancelled: true}, nil
			}
			continue
		}
		s.taskRegistry.Set(state.meta.TaskID, task, handle)

		result := <-task.doneCh
		if result.err == nil || result.cancelled {
			return result, nil
		}
		if strings.TrimSpace(result.content) != "" || strings.TrimSpace(result.thinking) != "" {
			return result, nil
		}
		lastErr = result.err
	}
	return ragChatTaskResult{}, lastErr
}

func (s *RagChatService) persistAssistantMessage(
	ctx context.Context,
	state ragChatRuntimeState,
	input RagChatInput,
	content string,
	thinking string,
) (RagChatFinishPayload, error) {
	content = normalizeAssistantMarkdown(content)
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
	if s.summaryTrigger != nil {
		if err := s.summaryTrigger.EnqueueSummaryCheck(context.WithoutCancel(ctx), raghistory.SummaryJobInput{
			ConversationID:  state.meta.ConversationID,
			UserID:          strings.TrimSpace(input.UserID),
			TargetMessageID: created.ID,
			RebuildReason:   "token_threshold_reached",
		}); err != nil {
			log.FromContext(ctx).Warnw("enqueue summary check failed", "error", err)
		}
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
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.tracer.recordChatTraceNode(ctx, state.traceID, ragTraceStatusFailed, result)
	ctx = enrichRagChatLogContext(ctx, state.traceID, state.meta.ConversationID, input.UserID, state.meta.TaskID)
	logRagChatCompletion(ctx, result)
	logRagChatTerminalError(ctx, "stream_result", result.err)
	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, result.err)

	if persistErr := s.persistFailedAssistantMessage(ctx, state, input, result); persistErr != nil {
		logRagChatTerminalError(ctx, "persist_failed_result", persistErr)
	} else {
		_ = sink.SendFinish(RagChatFinishPayload{Title: state.title})
	}
	_ = sink.SendError(result.err)
	_ = sink.SendDone()
	return result.err
}

// persistFailedAssistantMessage persists a best-effort assistant message on
// failure so the conversation history stays complete and the partial answer
// is not lost when streaming breaks mid-generation.
func (s *RagChatService) persistFailedAssistantMessage(
	ctx context.Context,
	state ragChatRuntimeState,
	input RagChatInput,
	result ragChatTaskResult,
) error {
	content := strings.TrimSpace(result.content)
	if errText := strings.TrimSpace(errorMessageOf(result.err)); errText != "" {
		if content != "" {
			content += "\n\n"
		}
		content += "(answer generation failed: " + errText + ")"
	}
	_, err := s.persistAssistantMessage(ctx, state, input, content, result.thinking)
	return err
}

func errorMessageOf(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
