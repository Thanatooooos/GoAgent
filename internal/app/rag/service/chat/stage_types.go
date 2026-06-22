package chat

import (
	"context"
	"time"

	agentapp "local/rag-project/internal/app/agent"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
)

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

type ragChatLongTermMemoryStageResult struct {
	result longtermmemory.RecallMemoriesResult
}

type ragChatRetrieveStageResult struct {
	result              ragretrieve.Result
	used                bool
	executionMode       string
	wallClockDurationMs int64
	subQuestions        []subQuestionRetrieveResult
}

type ragChatSessionRecallStageResult struct {
	result SessionRecallResult
}

type ragChatToolStageResult struct {
	result         ragtool.WorkflowResult
	backend        string
	agentRun       *agentapp.RunResponse
	agentError     *RagChatAgentServiceErrorPayload
	fallbackFrom   string
	fallbackReason string
}

type ragChatPromptStageResult struct {
	messages []convention.ChatMessage
	budget   ChatContextBudgetResult
	override *preferenceOverrideSignal
}

type ragChatPreparedState struct {
	state          ragChatRuntimeState
	history        []convention.ChatMessage
	userMessage    domain.ConversationMessage
	rewriteResult  ragrewrite.Result
	memoryContext  string
	sessionRecall  SessionRecallResult
	sessionContext string
	retrieveResult ragretrieve.Result
	retrievalUsed  bool
}

type ragChatStage[T any] struct {
	node            ragChatTraceNode
	run             func(context.Context) (T, error)
	buildExtra      func(T) map[string]any
	buildErrorExtra func(error) map[string]any
}
