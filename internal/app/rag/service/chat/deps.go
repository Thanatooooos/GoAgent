package chat

import (
	"context"

	raghistory "local/rag-project/internal/app/rag/core/history"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type RagChatService struct {
	conversationService     *ConversationService
	messageService          *ConversationMessageService
	historyService          raghistory.Service
	longTermMemory          longtermmemory.RecallService
	longTermMemoryWriteback LongTermMemoryWriteback
	rewriteService          ragrewrite.Service
	sessionRecall           SessionRecallService
	retrieveService         ragretrieve.Service
	promptService           *ragprompt.Service
	chatService             aichat.LLMService
	tracer                  *ChatTracer
	toolWorkflow            ragtool.Workflow
	parallelSubquestions    bool
	subquestionConcurrency  int
	confidenceThreshold     float64
	requestCacheMaxEntries  int
	taskRegistry            *TaskRegistry
	agentRuntime            AgentRuntimeService
	agentRuntimeMode        string
	chatContextBudget       ChatContextBudgetOptions
}

type LongTermMemoryWritebackInput struct {
	UserID          string
	Message         string
	SourceMessageID string
}

type LongTermMemoryWriteback interface {
	CapturePreferenceCandidate(ctx context.Context, input LongTermMemoryWritebackInput)
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
	ConfidenceThreshold     float64
	ParallelSubquestions    bool
	SubquestionConcurrency  int
	RequestCacheMaxEntries  int
	AgentRuntimeMode        string
	SessionRecall           SessionRecallService
	LongTermMemoryRecall    longtermmemory.RecallService
	LongTermMemoryWriteback LongTermMemoryWriteback
	ToolWorkflow            ragtool.Workflow
	ChatContextBudget       ChatContextBudgetOptions
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
	service.longTermMemoryWriteback = opts.LongTermMemoryWriteback
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
