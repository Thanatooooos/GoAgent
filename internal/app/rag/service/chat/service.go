package chat

import (
	"context"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	ragcache "local/rag-project/internal/app/rag/cache"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

func (s *RagChatService) Chat(ctx context.Context, input RagChatInput, sink RagChatEventSink) error {
	if sink == nil {
		return exception.NewServiceException("rag chat event sink is required", nil)
	}

	question := strings.TrimSpace(input.Question)
	userID := strings.TrimSpace(input.UserID)
	if question == "" {
		return exception.NewClientException("question is required", nil)
	}
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	chatPath := resolveChatPath(input)
	ctx = log.NewContext(ctx,
		"conversation_id", strings.TrimSpace(input.ConversationID),
		"user_id", userID,
	)
	logRagChatStart(ctx, input, s.agentRuntimeMode, chatPath)
	if input.UseAgentRuntime {
		return s.runAgentChat(ctx, input, sink)
	}
	if err := s.validateDependencies(); err != nil {
		return err
	}

	ctx = ragcache.WithRequestCache(ctx, ragcache.NewRequestCache(s.requestCacheMaxEntries))
	prepared, err := s.prepareChat(ctx, input)
	if err != nil {
		logRagChatTerminalError(ctx, "prepare_chat", err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if err := sink.SendMeta(prepared.state.meta); err != nil {
		return err
	}
	ctx = enrichRagChatLogContext(
		ctx,
		prepared.state.traceID,
		prepared.state.meta.ConversationID,
		input.UserID,
		prepared.state.meta.TaskID,
	)
	s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, buildRuntimePathTraceExtra(chatPath, "", input, s.agentRuntimeMode))
	if strings.TrimSpace(prepared.state.title) != "" {
		_ = sink.SendTitle(prepared.state.title)
	}
	s.emitPreparedObservabilityEvents(prepared, question, sink)

	retrieveResult, fallbackPrompt := s.applyFallbackGuard(ctx, prepared, question, sink)

	toolStage, err := s.runToolWorkflowStage(
		ctx,
		input,
		prepared.history,
		prepared.memoryContext,
		prepared.sessionContext,
		prepared.rewriteResult,
		retrieveResult,
		prepared.retrievalUsed,
		prepared.state.traceID,
		sink,
	)
	if err != nil {
		logRagChatTerminalError(ctx, "tool_stage", err)
		toolStage = ragChatToolStageResult{
			result: ragtool.WorkflowResult{
				Degraded:      true,
				DegradeReason: err.Error(),
			},
		}
	}
	logRagChatToolStageResult(ctx, toolStage)
	s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, buildRuntimePathTraceExtra(chatPath, resolveToolBackend(toolStage), input, s.agentRuntimeMode))
	if toolStage.result.Degraded {
		_ = sink.SendTool("tool_workflow", ragtool.CallStatusFailed, toolStage.result.DegradeReason)
	}
	if toolStage.agentRun != nil && toolStage.agentRun.Outcome.Status == agentapp.RunStatusAwaitingApproval {
		s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, map[string]any{
			"toolWorkflow": map[string]any{
				"backend":          firstNonEmptyString(toolStage.backend, "agent_runtime"),
				"awaitingApproval": true,
				"checkpointId":     strings.TrimSpace(toolStage.agentRun.Outcome.CheckpointID),
			},
		})
		return s.handleAgentToolStageApproval(ctx, prepared.state, sink, *toolStage.agentRun)
	}
	s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, map[string]any{
		"toolWorkflow": map[string]any{
			"backend":           firstNonEmptyString(toolStage.backend, "tool_workflow"),
			"used":              toolStage.result.Used,
			"degraded":          toolStage.result.Degraded,
			"degradeReason":     strings.TrimSpace(toolStage.result.DegradeReason),
			"control":           toolStage.result.Control,
			"traceMeta":         toolStage.result.TraceMeta,
			"toolCallCount":     len(toolStage.result.Calls),
			"roundCount":        len(toolStage.result.Rounds),
			"hasAnswerGuidance": strings.TrimSpace(toolStage.result.AnswerGuidance) != "",
			"hasToolContext":    strings.TrimSpace(toolStage.result.Context) != "",
		},
	})
	s.tracer.recordAgentWorkflowTraceNodes(ctx, prepared.state.traceID, toolStage.result)

	promptStage, err := s.runPromptStage(
		ctx,
		question,
		prepared.history,
		prepared.memoryContext,
		prepared.sessionContext,
		retrieveResult,
		toolStage.result.Context,
		toolStage.result.Control.PromptString(),
		toolStage.result.AnswerGuidance,
		effectiveFallbackPrompt(fallbackPrompt, toolStage.result.Used, question),
		prepared.state.traceID,
	)
	if err != nil {
		logRagChatTerminalError(ctx, "prompt_stage", err)
		s.tracer.finishTraceRun(ctx, prepared.state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	result, err := s.runStreamingAnswer(
		ctx,
		prepared.state,
		promptStage.messages,
		promptStage.budget.EstimatedPromptTokens,
		input.DeepThinking,
		sink,
	)
	if err != nil {
		logRagChatTerminalError(ctx, "streaming_answer", err)
		s.tracer.finishTraceRun(ctx, prepared.state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if result.cancelled {
		return s.handleCancelledResult(ctx, input, prepared.state, result, sink)
	}
	if result.err != nil {
		return s.handleFailedResult(ctx, prepared.state, result, sink)
	}
	return s.handleSucceededResult(ctx, input, prepared.state, result, sink)
}

func (s *RagChatService) CancelTask(taskID string) bool {
	if s == nil || s.taskRegistry == nil {
		return false
	}
	return s.taskRegistry.Cancel(taskID)
}

func (s *RagChatService) validateDependencies() error {
	if s == nil {
		return exception.NewServiceException("rag chat service is required", nil)
	}
	if s.conversationService == nil {
		return exception.NewServiceException("conversation service is required", nil)
	}
	if s.messageService == nil {
		return exception.NewServiceException("conversation message service is required", nil)
	}
	if s.historyService == nil {
		return exception.NewServiceException("rag history service is required", nil)
	}
	if s.retrieveService == nil {
		return exception.NewServiceException("rag retrieve service is required", nil)
	}
	if s.promptService == nil {
		return exception.NewServiceException("rag prompt service is required", nil)
	}
	if s.chatService == nil {
		return exception.NewServiceException("chat model service is required", nil)
	}
	return nil
}
