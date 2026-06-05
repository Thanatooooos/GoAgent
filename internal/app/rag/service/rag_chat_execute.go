package service

import (
	"context"
	"strings"
	"sync"

	agentstate "local/rag-project/internal/app/agent/state"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
)

type ragChatWorkflowEventSink struct {
	sink RagChatEventSink
}

func (s ragChatWorkflowEventSink) OnAgentThink(message string) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendAgentThink(message)
}

func (s ragChatWorkflowEventSink) OnToolStart(event ragtool.ToolCallEvent) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendToolStart(event)
}

func (s ragChatWorkflowEventSink) OnToolResult(event ragtool.ToolCallEvent) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendToolResult(event)
}

type ragChatStreamCallback struct {
	task *ragChatTask
	sink RagChatEventSink

	mu       sync.Mutex
	content  strings.Builder
	thinking strings.Builder
}

func newRagChatStreamCallback(task *ragChatTask, sink RagChatEventSink) *ragChatStreamCallback {
	callback := &ragChatStreamCallback{
		task: task,
		sink: sink,
	}
	go callback.watchCancel()
	return callback
}

func (c *ragChatStreamCallback) OnContent(content string) {
	c.mu.Lock()
	c.content.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendMessage(content)
}

func (c *ragChatStreamCallback) OnThinking(content string) {
	c.mu.Lock()
	c.thinking.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendThinking(content)
}

func (c *ragChatStreamCallback) OnComplete() {
	c.task.doneCh <- ragChatTaskResult{
		content:  c.currentContent(),
		thinking: c.currentThinking(),
	}
}

func (c *ragChatStreamCallback) OnError(err error) {
	c.task.doneCh <- ragChatTaskResult{
		content:  c.currentContent(),
		thinking: c.currentThinking(),
		err:      err,
	}
}

func (c *ragChatStreamCallback) watchCancel() {
	<-c.task.cancelCh
	c.task.doneCh <- ragChatTaskResult{
		cancelled: true,
		content:   c.currentContent(),
		thinking:  c.currentThinking(),
	}
}

func (c *ragChatStreamCallback) currentContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.content.String()
}

func (c *ragChatStreamCallback) currentThinking() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.thinking.String()
}

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

	callback := newRagChatStreamCallback(task, sink)
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
	logRagChatCompletion(state.traceID, state.meta.ConversationID, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		logRagChatTerminalError(state.traceID, "persist_cancelled_result", err)
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
	logRagChatCompletion(state.traceID, state.meta.ConversationID, result)
	logRagChatTerminalError(state.traceID, "stream_result", result.err)
	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, result.err)
	_ = sink.SendError(result.err)
	_ = sink.SendDone()
	return result.err
}

func (s *RagChatService) handleSucceededResult(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.tracer.recordChatTraceNode(ctx, state.traceID, ragTraceStatusSuccess, result)
	logRagChatCompletion(state.traceID, state.meta.ConversationID, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		logRagChatTerminalError(state.traceID, "persist_succeeded_result", err)
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusSuccess, nil)
	_ = sink.SendFinish(payload)
	_ = sink.SendDone()
	return nil
}

func (s *RagChatService) runToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	retrievalUsed bool,
	traceID string,
	sink RagChatEventSink,
) (ragChatToolStageResult, error) {
	if s == nil || !shouldRunToolWorkflow(input, rewriteResult, retrievalUsed) {
		return ragChatToolStageResult{}, nil
	}
	if s.shouldUseAgentRuntimeForToolStage(input, rewriteResult, retrievalUsed) {
		agentResult, err := s.runAgentToolWorkflowStage(ctx, input, history, memoryContext, sessionContext, rewriteResult, retrieveResult, traceID, sink)
		if err != nil {
			return ragChatToolStageResult{}, err
		}
		if shouldFallbackFromAgentToolStage(agentResult) && s.toolWorkflow != nil {
			legacyResult, legacyErr := s.runLegacyToolWorkflowStage(ctx, input, history, rewriteResult, retrieveResult, traceID, sink, true)
			if legacyErr == nil {
				legacyResult.fallbackFrom = "agent_runtime"
				legacyResult.fallbackReason = firstNonEmptyString(agentResult.result.DegradeReason, agentResult.agentError.Message)
				legacyResult.agentError = agentResult.agentError
				return legacyResult, nil
			}
			return ragChatToolStageResult{}, legacyErr
		}
		if agentResult.agentError != nil && sink != nil {
			_ = sink.SendAgentServiceError(*agentResult.agentError)
		}
		return agentResult, nil
	}
	return s.runLegacyToolWorkflowStage(ctx, input, history, rewriteResult, retrieveResult, traceID, sink, false)
}

func (s *RagChatService) runLegacyToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	traceID string,
	sink RagChatEventSink,
	fallback bool,
) (ragChatToolStageResult, error) {
	if s == nil || s.toolWorkflow == nil {
		return ragChatToolStageResult{}, nil
	}
	nodeID := "tool_workflow"
	nodeName := "tool_workflow"
	if fallback {
		nodeID = "tool_workflow_fallback"
		nodeName = "tool_workflow_fallback"
	}
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatToolStageResult]{
		node: ragChatTraceNode{
			NodeID:   nodeID,
			NodeType: "tool",
			NodeName: nodeName,
		},
		run: func(ctx context.Context) (ragChatToolStageResult, error) {
			result, err := s.toolWorkflow.Run(ctx, ragtool.WorkflowInput{
				Question:         strings.TrimSpace(input.Question),
				UserID:           strings.TrimSpace(input.UserID),
				ConversationID:   strings.TrimSpace(input.ConversationID),
				TraceID:          strings.TrimSpace(traceID),
				Control:          defaultWorkflowControl(),
				KnowledgeBaseIDs: append([]string(nil), input.KnowledgeBaseIDs...),
				History:          append([]convention.ChatMessage(nil), history...),
				RewriteResult:    rewriteResult,
				RetrieveResult:   retrieveResult,
				EventSink:        ragChatWorkflowEventSink{sink: sink},
			})
			if err != nil {
				return ragChatToolStageResult{}, err
			}
			return ragChatToolStageResult{result: result, backend: "tool_workflow"}, nil
		},
		buildExtra: func(result ragChatToolStageResult) map[string]any {
			return buildToolWorkflowStageTraceExtra(result)
		},
	})
}

func (s *RagChatService) runAgentToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	traceID string,
	sink RagChatEventSink,
) (ragChatToolStageResult, error) {
	if s == nil || s.agentRuntime == nil {
		return ragChatToolStageResult{}, nil
	}

	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatToolStageResult]{
		node: ragChatTraceNode{
			NodeID:   "agent_tool_workflow",
			NodeType: "agent",
			NodeName: "agent_tool_workflow",
		},
		run: func(ctx context.Context) (ragChatToolStageResult, error) {
			req := buildAgentToolStageRequest(input, traceID, history, memoryContext, sessionContext, rewriteResult, retrieveResult)
			req.Options.OutputMode = agentstate.OutputModeFinalAnswer
			run, err := s.agentRuntime.RunDetailed(ctx, req)
			if err != nil {
				payload := newRagChatAgentServiceErrorPayload(err)
				return ragChatToolStageResult{
					backend:    "agent_runtime",
					agentError: &payload,
					result: ragtool.WorkflowResult{
						Used:          true,
						Degraded:      true,
						DegradeReason: err.Error(),
						Control:       defaultAgentWorkflowControl(),
						TraceMeta:     defaultAgentWorkflowTraceMeta(),
					},
				}, nil
			}
			emitProjectedAgentToolEvents(sink, run)
			return ragChatToolStageResult{
				result:   workflowResultFromAgentRun(run),
				backend:  "agent_runtime",
				agentRun: &run,
			}, nil
		},
		buildExtra: func(result ragChatToolStageResult) map[string]any {
			return buildAgentRuntimeToolStageTraceExtra(result)
		},
	})
}

func shouldFallbackFromAgentToolStage(result ragChatToolStageResult) bool {
	return result.agentRun == nil && result.agentError != nil
}

func buildFallbackPrompt(question string) string {
	return "Knowledge retrieval confidence is low for question: " + question + ". Respond in Chinese, clearly state no matching knowledge was found, and note the answer may rely on general model knowledge."
}

func effectiveFallbackPrompt(fallbackPrompt string, toolUsed bool, question string) string {
	if fallbackPrompt == "" {
		return ""
	}
	if toolUsed {
		return ""
	}
	return fallbackPrompt
}

func (s *RagChatService) runPromptStage(
	ctx context.Context,
	question string,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	promptCtx ragretrieve.Result,
	toolContext string,
	workflowPolicy string,
	answerGuidance string,
	systemPromptOverride string,
	traceID string,
) (ragChatPromptStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatPromptStageResult]{
		node: ragChatTraceNode{
			NodeID:   "prompt",
			NodeType: "prompt",
			NodeName: "build_messages",
		},
		run: func(context.Context) (ragChatPromptStageResult, error) {
			messages, err := s.promptService.BuildMessages(ragprompt.Context{
				Question:         question,
				MemoryContext:    memoryContext,
				SessionContext:   sessionContext,
				KnowledgeContext: promptCtx.KnowledgeContext,
				ToolContext:      toolContext,
				WorkflowPolicy:   workflowPolicy,
				AnswerGuidance:   answerGuidance,
				History:          history,
				SystemPrompt:     systemPromptOverride,
			})
			if err != nil {
				return ragChatPromptStageResult{}, err
			}
			return ragChatPromptStageResult{messages: messages}, nil
		},
		buildExtra: func(result ragChatPromptStageResult) map[string]any {
			return map[string]any{
				"messageCount": len(result.messages),
			}
		},
	})
}

func defaultWorkflowControl() ragtool.WorkflowControl {
	return ragtool.WorkflowControl{
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}

func defaultAgentWorkflowControl() ragtool.WorkflowControl {
	return ragtool.WorkflowControl{
		Capability:          ragtool.CapabilityGeneral,
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}

func defaultAgentWorkflowTraceMeta() ragtool.WorkflowTraceMeta {
	return ragtool.WorkflowTraceMeta{
		Capability:          ragtool.CapabilityGeneral,
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}
