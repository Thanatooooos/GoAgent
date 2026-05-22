package service

import (
	"context"
	"strings"
	"sync"

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
	}); err != nil {
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

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
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

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
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
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	retrievalUsed bool,
	traceID string,
	sink RagChatEventSink,
) (ragChatToolStageResult, error) {
	if s == nil || s.toolWorkflow == nil || !shouldRunToolWorkflow(input, rewriteResult, retrievalUsed) {
		return ragChatToolStageResult{}, nil
	}

	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatToolStageResult]{
		node: ragChatTraceNode{
			NodeID:   "tool_workflow",
			NodeType: "tool",
			NodeName: "tool_workflow",
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
			return ragChatToolStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatToolStageResult) map[string]any {
			names := make([]string, 0, len(result.result.Calls))
			for _, call := range result.result.Calls {
				names = append(names, strings.TrimSpace(call.Name))
			}
			return map[string]any{
				"used":                result.result.Used,
				"toolCallCount":       len(result.result.Calls),
				"roundCount":          len(result.result.Rounds),
				"toolNames":           names,
				"degraded":            result.result.Degraded,
				"degradeReason":       strings.TrimSpace(result.result.DegradeReason),
				"capability":          strings.TrimSpace(result.result.TraceMeta.Capability),
				"executionMode":       strings.TrimSpace(result.result.TraceMeta.ExecutionMode),
				"riskLevel":           strings.TrimSpace(result.result.TraceMeta.RiskLevel),
				"approvalRequirement": strings.TrimSpace(result.result.TraceMeta.ApprovalRequirement),
				"evidenceSources":     append([]string(nil), result.result.TraceMeta.EvidenceSources...),
			}
		},
	})
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
