package service

import (
	"context"
	"strings"

	ragmemory "local/rag-project/internal/app/rag/core/memory"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	ragtool "local/rag-project/internal/app/rag/tool/core"
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

type RagChatMemoryStoredPayload struct {
	ConversationID   string `json:"conversationId"`
	MessageID        string `json:"messageId"`
	IsSummarized     bool   `json:"isSummarized"`
	ContentSummary   string `json:"contentSummary,omitempty"`
	RawContentLength int    `json:"rawContentLength,omitempty"`
}

type RagChatSessionRecallHitPayload struct {
	MessageID     string  `json:"messageId"`
	ChunkIndex    int     `json:"chunkIndex"`
	Score         float32 `json:"score"`
	Summary       string  `json:"summary,omitempty"`
	Excerpt       string  `json:"excerpt,omitempty"`
	SourceChunkID string  `json:"sourceChunkId,omitempty"`
}

type RagChatSessionRecallPayload struct {
	Query          string                           `json:"query,omitempty"`
	Used           bool                             `json:"used"`
	HitCount       int                              `json:"hitCount"`
	TopScore       float32                          `json:"topScore"`
	TruncatedBy    string                           `json:"truncatedBy,omitempty"`
	CandidateCount int                              `json:"candidateCount,omitempty"`
	Hits           []RagChatSessionRecallHitPayload `json:"hits,omitempty"`
}

type RagChatEventSink interface {
	SendMeta(meta RagChatMeta) error
	SendFallback(reason string) error
	SendAgentThink(message string) error
	SendMemoryStored(payload RagChatMemoryStoredPayload) error
	SendSessionRecall(payload RagChatSessionRecallPayload) error
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
	explicitMemory      ExplicitMemoryRecallService
	rewriteService      ragrewrite.Service
	sessionRecall       SessionRecallService
	retrieveService     ragretrieve.Service
	promptService       *ragprompt.Service
	chatService         aichat.LLMService
	tracer              *ChatTracer
	toolWorkflow        ragtool.Workflow
	confidenceThreshold float64
	taskRegistry        *TaskRegistry
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

func (s *RagChatService) SetSessionRecallService(service SessionRecallService) {
	if s == nil {
		return
	}
	s.sessionRecall = service
}

func (s *RagChatService) SetExplicitMemoryService(service *MemoryService) {
	if s == nil {
		return
	}
	s.explicitMemory = service
}

func (s *RagChatService) SetExplicitMemoryRecallService(service ExplicitMemoryRecallService) {
	if s == nil {
		return
	}
	s.explicitMemory = service
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
	s.emitPreparedObservabilityEvents(prepared, question, sink)

	retrieveResult, fallbackPrompt := s.applyFallbackGuard(ctx, prepared, question, sink)

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
		s.tracer.finishTraceRun(ctx, prepared.state.traceID, ragTraceStatusFailed, err)
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	result, err := s.runStreamingAnswer(ctx, prepared.state, promptStage.messages, input.DeepThinking, sink)
	if err != nil {
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
