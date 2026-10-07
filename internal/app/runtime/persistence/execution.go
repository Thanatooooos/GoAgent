package persistence

import (
	"context"
	"errors"
	"time"
)

var ErrExecutionLeaseLost = errors.New("chat execution is no longer owned; reconnect to inspect its outcome")

type Execution struct {
	TaskID, ConversationID, UserMessageID, UserID, Owner string
	Epoch                                                int64
}

type ChatExecutionStore interface {
	ClaimChatExecution(context.Context, Session, string, time.Duration) (Execution, error)
	RenewChatExecution(context.Context, Execution, time.Duration) error
}

type executionContextKey struct{}

func WithExecution(ctx context.Context, execution Execution) context.Context {
	return context.WithValue(ctx, executionContextKey{}, execution)
}

func CurrentExecution(ctx context.Context) (Execution, bool) {
	execution, ok := ctx.Value(executionContextKey{}).(Execution)
	return execution, ok
}
