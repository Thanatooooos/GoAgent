package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
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
	result ragretrieve.Result
	used   bool
}

type ragChatSessionRecallStageResult struct {
	result SessionRecallResult
}

type ragChatToolStageResult struct {
	result ragtool.WorkflowResult
}

type ragChatPromptStageResult struct {
	messages []convention.ChatMessage
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
	node       ragChatTraceNode
	run        func(context.Context) (T, error)
	buildExtra func(T) map[string]any
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
