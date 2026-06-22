package history

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestValidateStructuredSummaryRequiresActivePriorityWhenConversationSetsCurrentFocus(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    2,
		Goal:             "起草 summary 样本",
		EstablishedFacts: []string{"数据库最终选型为 PostgreSQL"},
		OpenQuestions:    []string{"ERR_POOL_TIMEOUT 根因是否已确认"},
	}

	source := []domain.ConversationMessage{
		{Role: "user", Content: "当前真正活跃的目标还是把 summary 样本起草出来，并把 must_cover、critical_contract 的边界写清楚。"},
	}

	result := ValidateStructuredSummary(summary, source)
	if result.Accepted {
		t.Fatalf("Validation.Accepted = true, want false when active priority is missing")
	}
	if result.Reason == "" {
		t.Fatalf("Validation.Reason = empty, want a concrete rejection reason")
	}
}
