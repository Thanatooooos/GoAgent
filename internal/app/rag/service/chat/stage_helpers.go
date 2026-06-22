package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

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
				extra := map[string]any{
					"error": err.Error(),
				}
				if stage.buildErrorExtra != nil {
					for key, value := range stage.buildErrorExtra(err) {
						extra[key] = value
					}
				}
				_ = tracer.recordTraceNodeAt(ctx, traceID, stage.node, ragTraceStatusFailed, startedAt, endedAt, extra)
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
