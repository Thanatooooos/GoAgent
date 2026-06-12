package log

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFromContextNilDoesNotPanic(t *testing.T) {
	t.Parallel()
	logger := FromContext(nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	logger.Info("from nil context")
}

func TestFromContextUsesContextLogger(t *testing.T) {
	t.Parallel()
	core, observed := observer.New(zap.InfoLevel)
	contextLogger := zap.New(core).Sugar()

	ctx := context.WithValue(context.Background(), contextKey{}, contextLogger)
	FromContext(ctx).Infow("ctx bound", "request_id", "rid-1")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "ctx bound" {
		t.Fatalf("unexpected message: %q", entries[0].Message)
	}
}

func TestNewContextMergesFields(t *testing.T) {
	t.Parallel()
	core, observed := observer.New(zap.InfoLevel)
	contextLogger := zap.New(core).Sugar()

	ctx := context.WithValue(context.Background(), contextKey{}, contextLogger)
	ctx = NewContext(ctx, "trace_id", "trace-1", "conversation_id", "conv-1")
	FromContext(ctx).Infow("stage done", "stage", "rewrite")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	contextMap := entries[0].ContextMap()
	if contextMap["trace_id"] != "trace-1" {
		t.Fatalf("expected trace_id trace-1, got %v", contextMap["trace_id"])
	}
	if contextMap["conversation_id"] != "conv-1" {
		t.Fatalf("expected conversation_id conv-1, got %v", contextMap["conversation_id"])
	}
	if contextMap["stage"] != "rewrite" {
		t.Fatalf("expected stage rewrite, got %v", contextMap["stage"])
	}
}

func TestWithFieldsUsesGlobalLogger(t *testing.T) {
	t.Parallel()
	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	logger := WithFields("request_id", "rid-2")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	logger.Infow("with fields")
}
