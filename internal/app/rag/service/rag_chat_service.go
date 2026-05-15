package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ragmemory "local/rag-project/internal/app/rag/core/memory"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	ragTraceStatusRunning   = "running"
	ragTraceStatusSuccess   = "success"
	ragTraceStatusFailed    = "failed"
	ragTraceStatusCancelled = "cancelled"
)

type RagChatInput struct {
	ConversationID   string
	UserID           string
	Question         string
	KnowledgeBaseIDs []string
	DeepThinking     bool
}

type RagChatMeta struct {
	ConversationID string `json:"conversationId"`
	TaskID         string `json:"taskId"`
}

type RagChatFinishPayload struct {
	MessageID string
	Title     string
}

type RagChatEventSink interface {
	SendMeta(meta RagChatMeta) error
	SendFallback(reason string) error
	SendAgentThink(message string) error
	SendThinking(delta string) error
	SendMessage(delta string) error
	SendToolStart(payload ragtool.ToolCallEvent) error
	SendToolResult(payload ragtool.ToolCallEvent) error
	SendTitle(title string) error
	SendTool(name string, status string, summary string) error
	SendFinish(payload RagChatFinishPayload) error
	SendCancel(payload RagChatFinishPayload) error
	SendError(err error) error
	SendDone() error
}

type RagChatService struct {
	conversationService *ConversationService
	messageService      *ConversationMessageService
	memoryService       ragmemory.Service
	rewriteService      ragrewrite.Service
	retrieveService     ragretrieve.Service
	promptService       *ragprompt.Service
	chatService         aichat.LLMService
	tracer              *ChatTracer
	toolWorkflow        ragtool.Workflow
	confidenceThreshold float64
	taskRegistry        *TaskRegistry
}

type ragChatRuntimeState struct {
	meta          RagChatMeta
	title         string
	userMessageID string
	traceID       string
	startTime     time.Time
}

type ragChatTraceNode struct {
	NodeID       string
	ParentNodeID string
	Depth        int
	NodeType     string
	NodeName     string
}

type ragChatConversationStageResult struct {
	conversationID string
	conversation   domain.Conversation
}

type ragChatMemoryStageResult struct {
	history []convention.ChatMessage
}

type ragChatUserMessageStageResult struct {
	message domain.ConversationMessage
}

type ragChatRuntimeStageResult struct {
	state ragChatRuntimeState
}

type ragChatRewriteStageResult struct {
	result ragrewrite.Result
}

type ragChatRetrieveStageResult struct {
	result ragretrieve.Result
	used   bool
}

type ragChatToolStageResult struct {
	result ragtool.WorkflowResult
}

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

type ragChatPromptStageResult struct {
	messages []convention.ChatMessage
}

type ragChatPreparedState struct {
	state          ragChatRuntimeState
	history        []convention.ChatMessage
	rewriteResult  ragrewrite.Result
	retrieveResult ragretrieve.Result
	retrievalUsed  bool
}

type ragChatStage[T any] struct {
	node       ragChatTraceNode
	run        func(context.Context) (T, error)
	buildExtra func(T) map[string]any
}

func NewRagChatService(
	conversationService *ConversationService,
	messageService *ConversationMessageService,
	memoryService ragmemory.Service,
	rewriteService ragrewrite.Service,
	retrieveService ragretrieve.Service,
	promptService *ragprompt.Service,
	chatService aichat.LLMService,
	tracer *ChatTracer,
) *RagChatService {
	return &RagChatService{
		conversationService: conversationService,
		messageService:      messageService,
		memoryService:       memoryService,
		rewriteService:      rewriteService,
		retrieveService:     retrieveService,
		promptService:       promptService,
		chatService:         chatService,
		tracer:              tracer,
		taskRegistry:        NewTaskRegistry(),
	}
}

func (s *RagChatService) SetConfidenceThreshold(threshold float64) {
	if s == nil {
		return
	}
	s.confidenceThreshold = threshold
}

func (s *RagChatService) SetToolWorkflow(workflow ragtool.Workflow) {
	if s == nil {
		return
	}
	s.toolWorkflow = workflow
}

func (s *RagChatService) Chat(ctx context.Context, input RagChatInput, sink RagChatEventSink) error {
	if err := s.validateDependencies(); err != nil {
		return err
	}
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

	prepared, err := s.prepareChat(ctx, input)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if err := sink.SendMeta(prepared.state.meta); err != nil {
		return err
	}
	if strings.TrimSpace(prepared.state.title) != "" {
		_ = sink.SendTitle(prepared.state.title)
	}

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

	toolStage, err := s.runToolWorkflowStage(
		ctx,
		input,
		prepared.history,
		prepared.rewriteResult,
		retrieveResult,
		prepared.retrievalUsed,
		prepared.state.traceID,
		sink,
	)
	if err != nil {
		toolStage = ragChatToolStageResult{
			result: ragtool.WorkflowResult{
				Degraded:      true,
				DegradeReason: err.Error(),
			},
		}
	}
	if toolStage.result.Degraded {
		_ = sink.SendTool("tool_workflow", ragtool.CallStatusFailed, toolStage.result.DegradeReason)
	}
	s.tracer.appendTraceRunExtra(ctx, prepared.state.traceID, map[string]any{
		"toolWorkflow": map[string]any{
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
		retrieveResult,
		toolStage.result.Context,
		toolStage.result.Control.PromptString(),
		toolStage.result.AnswerGuidance,
		effectiveFallbackPrompt(fallbackPrompt, toolStage.result.Used, question),
		prepared.state.traceID,
	)
	if err != nil {
		s.tracer.finishTraceRun(ctx, prepared.state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	task := s.taskRegistry.New()
	s.taskRegistry.Set(prepared.state.meta.TaskID, task, nil)
	defer s.taskRegistry.Delete(prepared.state.meta.TaskID)

	request := convention.ChatRequest{
		Messages: promptStage.messages,
	}
	if input.DeepThinking {
		request.Thinking = boolPointer(true)
	}

	callback := newRagChatStreamCallback(task, sink)
	handle, err := s.chatService.StreamChatWithRequest(request, callback)
	if err != nil {
		s.tracer.finishTraceRun(ctx, prepared.state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}
	s.taskRegistry.Set(prepared.state.meta.TaskID, task, handle)

	result := <-task.doneCh
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

	rewriteStage, err := s.runRewriteStage(ctx, input.Question, memoryStage.history, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	retrieveStage, err := s.runRetrieveStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}
	return ragChatPreparedState{
		state:          runtimeStage.state,
		history:        memoryStage.history,
		rewriteResult:  rewriteStage.result,
		retrieveResult: retrieveStage.result,
		retrievalUsed:  retrieveStage.used,
	}, nil
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
	if s.memoryService == nil {
		return exception.NewServiceException("rag memory service is required", nil)
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

func nextConversationExternalID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate conversation external id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func nextRagTaskID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag task id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func nextRagTraceID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag trace id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func nextRagTraceNodeID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag trace node id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func boolPointer(value bool) *bool {
	return &value
}

func timePointerValue(value time.Time) *time.Time {
	return &value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func runRagChatStage[T any](ctx context.Context, tracer *ChatTracer, traceID string, stage ragChatStage[T]) (T, error) {
	var zero T
	if tracer != nil {
		startedAt := tracer.now()
		result, err := stage.run(ctx)
		endedAt := tracer.now()
		if err != nil {
			if strings.TrimSpace(traceID) != "" && stage.node.NodeID != "" {
				_ = tracer.recordTraceNodeAt(ctx, traceID, stage.node, ragTraceStatusFailed, startedAt, endedAt, map[string]any{
					"error": err.Error(),
				})
			}
			return zero, err
		}
		if strings.TrimSpace(traceID) != "" && stage.node.NodeID != "" {
			extra := map[string]any{}
			if stage.buildExtra != nil {
				extra = stage.buildExtra(result)
			}
			_ = tracer.recordTraceNodeAt(ctx, traceID, stage.node, ragTraceStatusSuccess, startedAt, endedAt, extra)
		}
		return result, nil
	}
	result, err := stage.run(ctx)
	if err != nil {
		return zero, err
	}
	return result, nil
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

func (s *RagChatService) runMemoryStage(ctx context.Context, conversationID string, userID string) (ragChatMemoryStageResult, error) {
	history, err := s.memoryService.Load(ctx, conversationID, userID)
	if err != nil {
		return ragChatMemoryStageResult{}, exception.NewServiceException("failed to load rag memory", err)
	}
	return ragChatMemoryStageResult{history: history}, nil
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

func (s *RagChatService) runRewriteStage(ctx context.Context, question string, history []convention.ChatMessage, traceID string) (ragChatRewriteStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatRewriteStageResult]{
		node: ragChatTraceNode{
			NodeID:   "rewrite",
			NodeType: "rewrite",
			NodeName: "query_rewrite",
		},
		run: func(context.Context) (ragChatRewriteStageResult, error) {
			if s.rewriteService == nil {
				result := ragrewrite.Result{
					RewrittenQuestion: question,
					SubQuestions:      []string{question},
					NeedRetrieval:     ragrewrite.InferNeedRetrieval(question),
				}
				return ragChatRewriteStageResult{result: result}, nil
			}
			result := s.rewriteService.RewriteWithHistory(question, history)
			return ragChatRewriteStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatRewriteStageResult) map[string]any {
			return map[string]any{
				"subQuestionCount": len(result.result.SubQuestions),
			}
		},
	})
}

func (s *RagChatService) runRetrieveStage(ctx context.Context, input RagChatInput, rewriteResult ragrewrite.Result, traceID string) (ragChatRetrieveStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatRetrieveStageResult]{
		node: ragChatTraceNode{
			NodeID:   "retrieve",
			NodeType: "retrieve",
			NodeName: "vector_retrieve",
		},
		run: func(ctx context.Context) (ragChatRetrieveStageResult, error) {
			if !shouldRunRetrieve(input, rewriteResult) {
				return ragChatRetrieveStageResult{}, nil
			}
			subQuestions := rewriteResult.SubQuestions
			if len(subQuestions) == 0 {
				subQuestions = []string{strings.TrimSpace(input.Question)}
			}

			results := make([]ragretrieve.Result, 0, len(subQuestions))
			for _, q := range subQuestions {
				retrieveResult, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
					Query:            strings.TrimSpace(q),
					KnowledgeBaseIDs: input.KnowledgeBaseIDs,
					SearchMode:       ragretrieve.SearchModeHybrid,
				})
				if err != nil {
					continue
				}
				results = append(results, retrieveResult)
			}
			if len(results) == 0 {
				retrieveResult, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
					Query:            strings.TrimSpace(input.Question),
					KnowledgeBaseIDs: input.KnowledgeBaseIDs,
					SearchMode:       ragretrieve.SearchModeHybrid,
				})
				if err != nil {
					return ragChatRetrieveStageResult{}, exception.NewServiceException("failed to retrieve rag knowledge", err)
				}
				return ragChatRetrieveStageResult{result: retrieveResult, used: true}, nil
			}

			merged := ragretrieve.MergeResults(results, defaultTopK)
			return ragChatRetrieveStageResult{result: merged, used: true}, nil
		},
		buildExtra: func(result ragChatRetrieveStageResult) map[string]any {
			extra := map[string]any{
				"used":       result.used,
				"chunkCount": len(result.result.Chunks),
				"topScore":   topChunkScore(result.result),
			}
			if len(result.result.SearchChannels) > 0 {
				extra["searchChannels"] = append([]string(nil), result.result.SearchChannels...)
			}

			if len(result.result.ChannelStats) > 0 {
				stats := make([]map[string]any, 0, len(result.result.ChannelStats))
				for _, stat := range result.result.ChannelStats {
					item := map[string]any{
						"name":       stat.Name,
						"chunkCount": stat.ChunkCount,
						"latencyMs":  stat.LatencyMs,
					}
					if stat.Error != "" {
						item["error"] = stat.Error
					}
					if len(stat.Metadata) > 0 {
						item["metadata"] = stat.Metadata
					}
					stats = append(stats, item)
				}
				extra["channelStats"] = stats
			}
			return extra
		},
	})
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
	if s == nil || s.toolWorkflow == nil || !retrievalUsed {
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

const defaultTopK = 5

func shouldRunRetrieve(input RagChatInput, rewriteResult ragrewrite.Result) bool {
	if len(input.KnowledgeBaseIDs) == 0 {
		return false
	}
	return rewriteResult.NeedRetrieval
}

func topChunkScore(result ragretrieve.Result) float32 {
	if len(result.Chunks) == 0 {
		return 0
	}
	maxScore := result.Chunks[0].Score
	for _, c := range result.Chunks[1:] {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}
	return maxScore
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
