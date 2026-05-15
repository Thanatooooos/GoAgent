package core

import "context"

// ToolHandler is the function signature that middleware wraps.
type ToolHandler func(ctx context.Context, call Call) (Result, error)

// ToolMiddleware wraps a handler with cross-cutting behavior (timeout, retry, circuit breaker, logging).
// Implementations should call next(ctx, call) to proceed down the chain.
type ToolMiddleware interface {
	Wrap(next ToolHandler) ToolHandler
}

// ToolMiddlewareFunc adapts a plain function into ToolMiddleware.
type ToolMiddlewareFunc func(next ToolHandler) ToolHandler

func (f ToolMiddlewareFunc) Wrap(next ToolHandler) ToolHandler { return f(next) }

// ApplyMiddleware builds a handler chain from a slice of middleware.
// The first middleware in the slice is the outermost (executes first on entry, last on exit).
func ApplyMiddleware(handler ToolHandler, middlewares ...ToolMiddleware) ToolHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i].Wrap(handler)
	}
	return handler
}
