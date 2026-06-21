package history

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestBuildStructuredSummaryPromptUsesChineseInstructions(t *testing.T) {
	tier := SummaryBudgetTier{MaxChars: 320}
	latest := domain.ConversationSummary{
		StructuredSummaryJSON: `{"schema_version":1,"goal":"排查导入失败"}`,
	}
	historyMessages := []domain.ConversationMessage{
		{Role: "user", Content: "为什么 doc_fail_01 失败了？"},
		{Role: "assistant", Content: "目前看到 indexer 阶段报错。"},
	}

	prompt := buildStructuredSummaryPrompt(tier, latest, historyMessages)

	requiredPhrases := []string{
		"你正在将一段对话压缩为结构化工作记忆。",
		"只返回 JSON。",
		"允许字段：",
		"schema_version 必须是数字 1",
		"上一次结构化摘要 JSON：",
		"最近消息：",
		"用户：为什么 doc_fail_01 失败了？",
		"助手：目前看到 indexer 阶段报错。",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", phrase, prompt)
		}
	}

	unexpectedPhrases := []string{
		"You are compressing a conversation into structured work memory.",
		"Return JSON only.",
		"Previous structured summary JSON:",
		"Recent messages:",
		"user: ",
		"assistant: ",
	}
	for _, phrase := range unexpectedPhrases {
		if strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt not to contain %q, got:\n%s", phrase, prompt)
		}
	}
}

func TestBuildStructuredSummaryPromptIncludesRepairOrientedRules(t *testing.T) {
	tier := SummaryBudgetTier{MaxChars: 320}
	latest := domain.ConversationSummary{
		StructuredSummaryJSON: `{"schema_version":1,"goal":"先完成 spec"}`,
	}
	historyMessages := []domain.ConversationMessage{
		{Role: "user", Content: "当前先不要进入实现。"},
		{Role: "assistant", Content: "先完成 spec、design、tasks。"},
		{Role: "user", Content: "根因还没确认，先不要下结论。"},
	}

	prompt := buildStructuredSummaryPrompt(tier, latest, historyMessages)

	requiredPhrases := []string{
		"只保留当前仍然有效的目标和约束，保持当前边界",
		"当前不做什么也属于 constraints",
		"未确认、待验证、候选信息放进 open_questions",
		"不要把猜测写成 established_facts",
		"只保留当前边界内仍然有效的信息",
		"最近刚确认或刚变化的状态优先写入 recent_progress",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", phrase, prompt)
		}
	}
}
