package chat

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	fwlog "local/rag-project/internal/framework/log"
)

func TestEmitPreferenceOverrideObservabilityLogsOverride(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	ctx := fwlog.BindLogger(context.Background(), zap.New(core).Sugar())

	emitPreferenceOverrideObservability(ctx, "Please answer in English", "默认用中文回答")

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 override log entry, got %d", len(entries))
	}
	if entries[0].Message != "rag chat preference recall overridden by current turn input" {
		t.Fatalf("unexpected override log: %+v", entries[0])
	}
	contextMap := entries[0].ContextMap()
	if contextMap["canonical_key"] != "response.language" {
		t.Fatalf("unexpected override context: %+v", contextMap)
	}
	if contextMap["subsystem"] != "long_term_memory" {
		t.Fatalf("expected long_term_memory subsystem, got %+v", contextMap)
	}
}
