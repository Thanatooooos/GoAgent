package service

import (
	"testing"

	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestTokenUsageTraceExtraIncludesEstimatedSource(t *testing.T) {
	t.Parallel()
	extra := tokenUsageTraceExtra(aichat.EstimatedTokenUsage(120, 45), "estimated")
	if extra["promptTokens"] != 120 || extra["completionTokens"] != 45 || extra["totalTokens"] != 165 {
		t.Fatalf("unexpected usage extra: %+v", extra)
	}
	if extra["tokenUsageSource"] != "estimated" {
		t.Fatalf("expected estimated source, got %+v", extra["tokenUsageSource"])
	}
}
