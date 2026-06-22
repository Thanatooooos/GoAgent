package history

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestValidateStructuredSummaryAcceptsConfigEntityBeforeChinesePunctuation(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "排查错误码",
		Constraints:      []string{"pool.max_active=50", "pool.max_idle=5"},
		EstablishedFacts: []string{"客户端日志里反复出现 ERR_POOL_TIMEOUT"},
		OpenQuestions:    []string{"根因尚未定论"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "明白，已知 ERR_POOL_TIMEOUT 以及 pool.max_active=50、pool.max_idle=5，根因尚未定论。"},
	}

	result := ValidateStructuredSummary(summary, source)
	if !result.Accepted {
		t.Fatalf("expected validator to accept config entity before Chinese punctuation, got %+v", result)
	}
}
