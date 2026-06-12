package service

import (
	"context"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	ragcache "local/rag-project/internal/app/rag/cache"
	raghistory "local/rag-project/internal/app/rag/core/history"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
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
	UseAgentRuntime  bool
	RequireApproval  bool
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
	SendAgentOutcome(payload RagChatAgentOutcomePayload) error
	SendApprovalPending(payload RagChatApprovalPendingPayload) error
	SendAgentServiceError(payload RagChatAgentServiceErrorPayload) error
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
	conversationService    *ConversationService
	messageService         *ConversationMessageService
	historyService         raghistory.Service
	longTermMemory         longtermmemory.RecallService
	rewriteService         ragrewrite.Service
	sessionRecall          SessionRecallService
	retrieveService        ragretrieve.Service
	promptService          *ragprompt.Service
	chatService            aichat.LLMService
	tracer                 *ChatTracer
	toolWorkflow           ragtool.Workflow
	parallelSubquestions   bool
	subquestionConcurrency int
	confidenceThreshold    float64
	requestCacheMaxEntries int
	taskRegistry           *TaskRegistry
	agentRuntime           AgentRuntimeService
	agentRuntimeMode       string
	chatContextBudget      ChatContextBudgetOptions
}

// RagChatDeps bundles required RagChatService dependencies for construction-time injection.
type RagChatDeps struct {
	ConversationService *ConversationService
	MessageService      *ConversationMessageService
	HistoryService      raghistory.Service
	RewriteService      ragrewrite.Service
	RetrieveService     ragretrieve.Service
	PromptService       *ragprompt.Service
	ChatService         aichat.LLMService
	Tracer              *ChatTracer
	AgentRuntime        AgentRuntimeService
}

// RagChatOptions configures optional RagChatService behavior at construction time.
type RagChatOptions struct {
	ConfidenceThreshold    float64
	ParallelSubquestions   bool
	SubquestionConcurrency int
	RequestCacheMaxEntries int
	AgentRuntimeMode       string
	SessionRecall          SessionRecallService
	LongTermMemoryRecall   longtermmemory.RecallService
	ToolWorkflow           ragtool.Workflow
	ChatContextBudget      ChatContextBudgetOptions
}

func NewRagChatServiceWithDeps(deps RagChatDeps, opts RagChatOptions) (*RagChatService, error) {
	if deps.ConversationService == nil {
		return nil, exception.NewServiceException("conversation service is required", nil)
	}
	if deps.MessageService == nil {
		return nil, exception.NewServiceException("conversation message service is required", nil)
	}
	if deps.HistoryService == nil {
		return nil, exception.NewServiceException("rag history service is required", nil)
	}
	if deps.RetrieveService == nil {
		return nil, exception.NewServiceException("rag retrieve service is required", nil)
	}
	if deps.PromptService == nil {
		return nil, exception.NewServiceException("rag prompt service is required", nil)
	}
	if deps.ChatService == nil {
		return nil, exception.NewServiceException("chat model service is required", nil)
	}
	if deps.Tracer == nil {
		return nil, exception.NewServiceException("rag chat tracer is required", nil)
	}

	opts = normalizeRagChatOptions(opts)
	service := newRagChatService(
		deps.ConversationService,
		deps.MessageService,
		deps.HistoryService,
		deps.RewriteService,
		deps.RetrieveService,
		deps.PromptService,
		deps.ChatService,
		deps.Tracer,
	)
	service.confidenceThreshold = opts.ConfidenceThreshold
	service.parallelSubquestions = opts.ParallelSubquestions
	service.subquestionConcurrency = opts.SubquestionConcurrency
	service.requestCacheMaxEntries = opts.RequestCacheMaxEntries
	service.agentRuntimeMode = opts.AgentRuntimeMode
	service.sessionRecall = opts.SessionRecall
	service.longTermMemory = opts.LongTermMemoryRecall
	service.toolWorkflow = opts.ToolWorkflow
	service.agentRuntime = deps.AgentRuntime
	service.chatContextBudget = opts.ChatContextBudget.normalized()
	return service, nil
}

func normalizeRagChatOptions(opts RagChatOptions) RagChatOptions {
	if opts.SubquestionConcurrency <= 0 {
		opts.SubquestionConcurrency = 2
	}
	if opts.RequestCacheMaxEntries <= 0 {
		opts.RequestCacheMaxEntries = 128
	}
	opts.AgentRuntimeMode = normalizeAgentRuntimeMode(opts.AgentRuntimeMode)
	return opts
}

func newRagChatService(
	conversationService *ConversationService,
	messageService *ConversationMessageService,
	historyService raghistory.Service,
	rewriteService ragrewrite.Service,
	retrieveService ragretrieve.Service,
	promptService *ragprompt.Service,
	chatService aichat.LLMService,
	tracer *ChatTracer,
) *RagChatService {
	return &RagChatService{
		conversationService:    conversationService,
		messageService:         messageService,
		historyService:         historyService,
		rewriteService:         rewriteService,
		retrieveService:        retrieveService,
		promptService:          promptService,
		chatService:            chatService,
		tracer:                 tracer,
		parallelSubquestions:   true,
		subquestionConcurrency: 2,
		requestCacheMaxEntries: 128,
		taskRegistry:           NewTaskRegistry(),
		agentRuntimeMode:       ragChatAgentModeOff,
	}
}

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
