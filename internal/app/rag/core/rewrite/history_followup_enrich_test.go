package rewrite

import (
	"strings"
	"testing"
)

func TestEnrichPersistenceFollowUpRewrite(t *testing.T) {
	result := Result{
		RewrittenQuestion: "Redis 的持久化机制是什么",
		SubQuestions:      []string{"Redis 的持久化机制是什么"},
		NeedRetrieval:     true,
	}
	enriched := enrichPersistenceFollowUpRewrite("那持久化呢", result)
	if !strings.Contains(enriched.RewrittenQuestion, "AOF") || !strings.Contains(enriched.RewrittenQuestion, "RDB") {
		t.Fatalf("expected AOF/RDB in rewrite, got %q", enriched.RewrittenQuestion)
	}
}
