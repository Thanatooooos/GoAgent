package history

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestBuildStructuredSummaryPromptIncludesPriorityHierarchyRules(t *testing.T) {
	tier := SummaryBudgetTier{MaxChars: 400}
	prompt := buildStructuredSummaryPrompt(tier, domain.ConversationSummary{}, []domain.ConversationMessage{
		{Role: "user", Content: "CI flaky 不是当前重点。"},
		{Role: "assistant", Content: "先完成 spec、design、tasks。"},
	})

	required := []string{
		"active_priorities",
		"background_issues",
		"如果对话明确说某事项“不是当前重点/只是背景问题/暂不处理”，不要写进 active_priorities",
		"active_priorities 按执行优先级排序",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", phrase, prompt)
		}
	}
}
