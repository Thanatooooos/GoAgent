package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	ragmemory "local/rag-project/internal/app/rag/core/memory"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
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

// RagChatInput 描述最小聊天闭环的入参。
type RagChatInput struct {
	ConversationID   string
	UserID           string
	Question         string
	KnowledgeBaseIDs []string
	DeepThinking     bool
}

// RagChatMeta 表示聊天建立后的基础元数据。
type RagChatMeta struct {
	ConversationID string `json:"conversationId"`
	TaskID         string `json:"taskId"`
}

// RagChatFinishPayload 表示聊天完成时的最终元数据。
type RagChatFinishPayload struct {
	MessageID string
	Title     string
}

// RagChatEventSink 负责接收聊天过程中的事件。
type RagChatEventSink interface {
	// SendMeta 发送会话和任务元数据。
	SendMeta(meta RagChatMeta) error
	// SendThinking 发送思考流。
	SendThinking(delta string) error
	// SendMessage 发送回答流。
	SendMessage(delta string) error
	// SendTitle 发送标题更新事件。
	SendTitle(title string) error
	// SendFinish 发送完成事件。
	SendFinish(payload RagChatFinishPayload) error
	// SendCancel 发送取消事件。
	SendCancel(payload RagChatFinishPayload) error
	// SendError 发送错误事件。
	SendError(err error) error
	// SendDone 发送流结束事件。
	SendDone() error
}

// RagChatService 负责编排最小 RAG 聊天闭环。
type RagChatService struct {
	conversationService *ConversationService
	messageService      *ConversationMessageService
	memoryService       ragmemory.Service
	retrieveService     ragretrieve.Service
	promptService       *ragprompt.Service
	chatService         aichat.LLMService
	traceRunRepo        port.RagTraceRunRepository
	traceNodeRepo       port.RagTraceNodeRepository
	now                 func() time.Time

	taskMu sync.Mutex
	tasks  map[string]*ragChatTask
}

type ragChatTask struct {
	handle aichat.StreamCancellationHandle

	cancelOnce sync.Once
	cancelCh   chan struct{}
	doneCh     chan ragChatTaskResult
}

type ragChatTaskResult struct {
	cancelled bool
	content   string
	thinking  string
	err       error
}

type ragChatRuntimeState struct {
	meta         RagChatMeta
	title        string
	userMessageID string
	traceID      string
	startTime    time.Time
}

type ragChatTraceNode struct {
	NodeID   string
	NodeType string
	NodeName string
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

type ragChatRetrieveStageResult struct {
	result ragretrieve.Result
}

type ragChatPromptStageResult struct {
	messages []convention.ChatMessage
}

type ragChatStage[T any] struct {
	node       ragChatTraceNode
	run        func(context.Context) (T, error)
	buildExtra func(T) map[string]any
}

// NewRagChatService 创建最小聊天闭环服务。
func NewRagChatService(
	conversationService *ConversationService,
	messageService *ConversationMessageService,
	memoryService ragmemory.Service,
	retrieveService ragretrieve.Service,
	promptService *ragprompt.Service,
	chatService aichat.LLMService,
	traceRunRepo port.RagTraceRunRepository,
	traceNodeRepo port.RagTraceNodeRepository,
) *RagChatService {
	return &RagChatService{
		conversationService: conversationService,
		messageService:      messageService,
		memoryService:       memoryService,
		retrieveService:     retrieveService,
		promptService:       promptService,
		chatService:         chatService,
		traceRunRepo:        traceRunRepo,
		traceNodeRepo:       traceNodeRepo,
		now:                 time.Now,
		tasks:               map[string]*ragChatTask{},
	}
}

// Chat 执行一次最小 RAG 聊天闭环并持续输出流式事件。
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

	// 先准备会话、用户消息和 trace，这样后续流式阶段只关注输出。
	state, history, promptCtx, err := s.prepareChat(ctx, input)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if err := sink.SendMeta(state.meta); err != nil {
		return err
	}
	if strings.TrimSpace(state.title) != "" {
		_ = sink.SendTitle(state.title)
	}

	promptStage, err := s.runPromptStage(ctx, question, history, promptCtx, state.traceID)
	if err != nil {
		s.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	// 真正进入模型流式调用阶段，并把取消控制挂到任务表里。
	task := s.newTask()
	s.setTaskHandle(state.meta.TaskID, task, nil)
	defer s.deleteTask(state.meta.TaskID)

	request := convention.ChatRequest{
		Messages: promptStage.messages,
	}
	if input.DeepThinking {
		request.Thinking = boolPointer(true)
	}

	callback := newRagChatStreamCallback(task, sink)
	handle, err := s.chatService.StreamChatWithRequest(request, callback)
	if err != nil {
		s.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}
	s.setTaskHandle(state.meta.TaskID, task, handle)

	result := <-task.doneCh
	if result.cancelled {
		return s.handleCancelledResult(ctx, input, state, result, sink)
	}
	if result.err != nil {
		return s.handleFailedResult(ctx, state, result, sink)
	}
	return s.handleSucceededResult(ctx, input, state, result, sink)
}

// CancelTask 取消一个正在执行的聊天任务。
func (s *RagChatService) CancelTask(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}

	s.taskMu.Lock()
	task := s.tasks[taskID]
	s.taskMu.Unlock()
	if task == nil {
		return false
	}

	task.cancelOnce.Do(func() {
		close(task.cancelCh)
		if task.handle != nil {
			task.handle.Cancel()
		}
	})
	return true
}

// prepareChat 先完成会话、记忆、检索和 prompt 前准备。
func (s *RagChatService) prepareChat(ctx context.Context, input RagChatInput) (ragChatRuntimeState, []convention.ChatMessage, ragretrieve.Result, error) {
	conversationStage, err := s.runConversationStage(ctx, input)
	if err != nil {
		return ragChatRuntimeState{}, nil, ragretrieve.Result{}, err
	}

	memoryStage, err := s.runMemoryStage(ctx, conversationStage.conversationID, strings.TrimSpace(input.UserID))
	if err != nil {
		return ragChatRuntimeState{}, nil, ragretrieve.Result{}, err
	}

	userMessageStage, err := s.runUserMessageStage(ctx, input, conversationStage.conversationID)
	if err != nil {
		return ragChatRuntimeState{}, nil, ragretrieve.Result{}, err
	}

	runtimeStage, err := s.runRuntimeStage(ctx, input, conversationStage, userMessageStage)
	if err != nil {
		return ragChatRuntimeState{}, nil, ragretrieve.Result{}, err
	}

	retrieveStage, err := s.runRetrieveStage(ctx, input, runtimeStage.state.traceID)
	if err != nil {
		return ragChatRuntimeState{}, nil, ragretrieve.Result{}, err
	}

	return runtimeStage.state, memoryStage.history, retrieveStage.result, nil
}

// persistAssistantMessage 在流式结束后保存 assistant 消息。
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
		LastTime:       timePointerValue(s.now()),
	}); err != nil {
		return RagChatFinishPayload{}, err
	}

	return RagChatFinishPayload{
		MessageID: created.ID,
		Title:     state.title,
	}, nil
}

// validateDependencies 确认聊天主链路依赖齐全。
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

// newTask 创建一个新的聊天任务状态。
func (s *RagChatService) newTask() *ragChatTask {
	return &ragChatTask{
		cancelCh: make(chan struct{}),
		doneCh:   make(chan ragChatTaskResult, 1),
	}
}

// setTaskHandle 把流式取消句柄挂到任务上。
func (s *RagChatService) setTaskHandle(taskID string, task *ragChatTask, handle aichat.StreamCancellationHandle) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	task.handle = handle
	s.tasks[taskID] = task
}

// deleteTask 移除已结束的聊天任务。
func (s *RagChatService) deleteTask(taskID string) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	delete(s.tasks, taskID)
}

// startTraceRun 写入 trace run 起始记录。
func (s *RagChatService) startTraceRun(ctx context.Context, traceID, conversationID, taskID, userID string) error {
	return s.startTraceRunAt(ctx, traceID, conversationID, taskID, userID, s.now())
}

// startTraceRunAt 写入 trace run 起始记录，并显式指定开始时间。
func (s *RagChatService) startTraceRunAt(ctx context.Context, traceID, conversationID, taskID, userID string, startedAt time.Time) error {
	if s.traceRunRepo == nil {
		return nil
	}
	_, err := s.traceRunRepo.Create(ctx, domain.RagTraceRun{
		ID:             traceID,
		TraceID:        traceID,
		TraceName:      "rag_chat",
		EntryMethod:    "rag.v3.chat",
		ConversationID: conversationID,
		TaskID:         taskID,
		UserID:         userID,
		Status:         ragTraceStatusRunning,
		StartTime:      timePointerValue(startedAt),
		CreateTime:     startedAt,
		UpdateTime:     startedAt,
	})
	return err
}

// finishTraceRun 回写 trace run 最终状态。
func (s *RagChatService) finishTraceRun(ctx context.Context, traceID string, status string, cause error) {
	if s.traceRunRepo == nil || strings.TrimSpace(traceID) == "" {
		return
	}
	now := s.now()
	duration := int64(0)
	run, err := s.traceRunRepo.GetByTraceID(ctx, traceID)
	if err == nil && run.StartTime != nil {
		duration = now.Sub(*run.StartTime).Milliseconds()
	}
	errorMessage := ""
	if cause != nil {
		errorMessage = cause.Error()
	}
	_ = s.traceRunRepo.UpdateByTraceID(ctx, traceID, domain.RagTraceRun{
		TraceID:      traceID,
		Status:       status,
		ErrorMessage: errorMessage,
		EndTime:      &now,
		DurationMs:   &duration,
		UpdateTime:   now,
	})
}

// recordTraceNode 写入一个简单的阶段节点。
func (s *RagChatService) recordTraceNode(ctx context.Context, traceID string, node ragChatTraceNode, status string, extra map[string]any) error {
	if s.traceNodeRepo == nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	nodeRecordID, err := nextRagTraceNodeID()
	if err != nil {
		return err
	}
	now := s.now()
	duration := int64(0)
	extraData := ""
	if len(extra) > 0 {
		raw, err := json.Marshal(extra)
		if err == nil {
			extraData = string(raw)
		}
	}
	_, err = s.traceNodeRepo.Create(ctx, domain.RagTraceNode{
		ID:         nodeRecordID,
		TraceID:    traceID,
		NodeID:     node.NodeID,
		Depth:      1,
		NodeType:   node.NodeType,
		NodeName:   node.NodeName,
		Status:     status,
		StartTime:  &now,
		EndTime:    &now,
		DurationMs: &duration,
		ExtraData:  extraData,
		CreateTime: now,
		UpdateTime: now,
	})
	return err
}

type ragChatStreamCallback struct {
	task *ragChatTask
	sink RagChatEventSink

	mu       sync.Mutex
	content  strings.Builder
	thinking strings.Builder
}

// newRagChatStreamCallback 创建模型流回调。
func newRagChatStreamCallback(task *ragChatTask, sink RagChatEventSink) *ragChatStreamCallback {
	callback := &ragChatStreamCallback{
		task: task,
		sink: sink,
	}
	go callback.watchCancel()
	return callback
}

// OnContent 转发回答增量并累积最终内容。
func (c *ragChatStreamCallback) OnContent(content string) {
	c.mu.Lock()
	c.content.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendMessage(content)
}

// OnThinking 转发思考增量并累积最终思考内容。
func (c *ragChatStreamCallback) OnThinking(content string) {
	c.mu.Lock()
	c.thinking.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendThinking(content)
}

// OnComplete 标记模型正常完成。
func (c *ragChatStreamCallback) OnComplete() {
	c.task.doneCh <- ragChatTaskResult{
		content:  c.currentContent(),
		thinking: c.currentThinking(),
	}
}

// OnError 标记模型执行失败。
func (c *ragChatStreamCallback) OnError(err error) {
	c.task.doneCh <- ragChatTaskResult{
		content:  c.currentContent(),
		thinking: c.currentThinking(),
		err:      err,
	}
}

// watchCancel 在收到停止信号后回传取消结果。
func (c *ragChatStreamCallback) watchCancel() {
	<-c.task.cancelCh
	c.task.doneCh <- ragChatTaskResult{
		cancelled: true,
		content:   c.currentContent(),
		thinking:  c.currentThinking(),
	}
}

// currentContent 读取当前已聚合的回答内容。
func (c *ragChatStreamCallback) currentContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.content.String()
}

// currentThinking 读取当前已聚合的思考内容。
func (c *ragChatStreamCallback) currentThinking() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.thinking.String()
}

// nextConversationExternalID 生成前端使用的会话 ID。
func nextConversationExternalID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate conversation external id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

// nextRagTaskID 生成新的聊天任务 ID。
func nextRagTaskID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag task id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

// nextRagTraceID 生成新的 trace ID。
func nextRagTraceID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag trace id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

// nextRagTraceNodeID 生成新的 trace node 主键。
func nextRagTraceNodeID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate rag trace node id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

// boolPointer 构造布尔指针。
func boolPointer(value bool) *bool {
	return &value
}

// timePointerValue 构造时间指针。
func timePointerValue(value time.Time) *time.Time {
	return &value
}

// firstNonEmptyString 返回第一个非空字符串。
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// runRagChatStage 统一执行一个阶段，并在可用时写入阶段 trace。
func runRagChatStage[T any](ctx context.Context, service *RagChatService, traceID string, stage ragChatStage[T]) (T, error) {
	var zero T
	result, err := stage.run(ctx)
	if err != nil {
		if strings.TrimSpace(traceID) != "" && stage.node.NodeID != "" {
			_ = service.recordTraceNode(ctx, traceID, stage.node, ragTraceStatusFailed, map[string]any{
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
		_ = service.recordTraceNode(ctx, traceID, stage.node, ragTraceStatusSuccess, extra)
	}
	return result, nil
}

// handleCancelledResult 处理流式任务被取消后的统一收口。
func (s *RagChatService) handleCancelledResult(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.recordChatTraceNode(ctx, state.traceID, ragTraceStatusCancelled, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		s.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	s.finishTraceRun(ctx, state.traceID, ragTraceStatusCancelled, nil)
	_ = sink.SendCancel(payload)
	_ = sink.SendDone()
	return nil
}

// handleFailedResult 处理流式任务失败后的统一收口。
func (s *RagChatService) handleFailedResult(
	ctx context.Context,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.recordChatTraceNode(ctx, state.traceID, ragTraceStatusFailed, result)
	s.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, result.err)
	_ = sink.SendError(result.err)
	_ = sink.SendDone()
	return result.err
}

// handleSucceededResult 处理流式任务成功后的统一收口。
func (s *RagChatService) handleSucceededResult(
	ctx context.Context,
	input RagChatInput,
	state ragChatRuntimeState,
	result ragChatTaskResult,
	sink RagChatEventSink,
) error {
	s.recordChatTraceNode(ctx, state.traceID, ragTraceStatusSuccess, result)

	payload, err := s.persistAssistantMessage(ctx, state, input, result.content, result.thinking)
	if err != nil {
		s.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	s.finishTraceRun(ctx, state.traceID, ragTraceStatusSuccess, nil)
	_ = sink.SendFinish(payload)
	_ = sink.SendDone()
	return nil
}

// recordChatTraceNode 统一记录 chat 阶段的结果节点。
func (s *RagChatService) recordChatTraceNode(ctx context.Context, traceID string, status string, result ragChatTaskResult) {
	extra := map[string]any{
		"contentLength":  len(strings.TrimSpace(result.content)),
		"thinkingLength": len(strings.TrimSpace(result.thinking)),
	}
	if result.err != nil {
		extra["error"] = result.err.Error()
	}
	_ = s.recordTraceNode(ctx, traceID, ragChatTraceNode{
		NodeID:   "chat",
		NodeType: "chat",
		NodeName: "stream_chat",
	}, status, extra)
}

// runConversationStage 负责准备会话实体和外部会话 ID。
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

// runMemoryStage 负责加载当前会话的历史上下文。
func (s *RagChatService) runMemoryStage(ctx context.Context, conversationID string, userID string) (ragChatMemoryStageResult, error) {
	history, err := s.memoryService.Load(ctx, conversationID, userID)
	if err != nil {
		return ragChatMemoryStageResult{}, exception.NewServiceException("failed to load rag memory", err)
	}
	return ragChatMemoryStageResult{history: history}, nil
}

// runUserMessageStage 负责保存当前轮的用户消息。
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

// runRuntimeStage 负责生成任务元数据并启动一条 trace run。
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
		startTime:     s.now(),
	}
	_ = s.startTraceRunAt(ctx, traceID, conversationStage.conversationID, taskID, strings.TrimSpace(input.UserID), state.startTime)

	return ragChatRuntimeStageResult{state: state}, nil
}

// runRetrieveStage 负责执行知识检索并记录检索节点。
func (s *RagChatService) runRetrieveStage(ctx context.Context, input RagChatInput, traceID string) (ragChatRetrieveStageResult, error) {
	return runRagChatStage(ctx, s, traceID, ragChatStage[ragChatRetrieveStageResult]{
		node: ragChatTraceNode{
			NodeID:   "retrieve",
			NodeType: "retrieve",
			NodeName: "vector_retrieve",
		},
		run: func(ctx context.Context) (ragChatRetrieveStageResult, error) {
			retrieveResult, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
				Query:            strings.TrimSpace(input.Question),
				KnowledgeBaseIDs: input.KnowledgeBaseIDs,
			})
			if err != nil {
				return ragChatRetrieveStageResult{}, exception.NewServiceException("failed to retrieve rag knowledge", err)
			}
			return ragChatRetrieveStageResult{result: retrieveResult}, nil
		},
		buildExtra: func(result ragChatRetrieveStageResult) map[string]any {
			return map[string]any{
				"chunkCount": len(result.result.Chunks),
			}
		},
	})
}

// runPromptStage 负责组装模型输入消息并记录 prompt 节点。
func (s *RagChatService) runPromptStage(
	ctx context.Context,
	question string,
	history []convention.ChatMessage,
	promptCtx ragretrieve.Result,
	traceID string,
) (ragChatPromptStageResult, error) {
	return runRagChatStage(ctx, s, traceID, ragChatStage[ragChatPromptStageResult]{
		node: ragChatTraceNode{
			NodeID:   "prompt",
			NodeType: "prompt",
			NodeName: "build_messages",
		},
		run: func(context.Context) (ragChatPromptStageResult, error) {
			messages, err := s.promptService.BuildMessages(ragprompt.Context{
				Question:         question,
				KnowledgeContext: promptCtx.KnowledgeContext,
				History:          history,
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
