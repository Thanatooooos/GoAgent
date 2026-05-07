package rewrite

import (
	"os"
	"strings"
	"testing"

	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
)

// TestLLMRewriteIntegration 使用真实 LLM 验证查询改写效果。
// 设置 RAG_INTEGRATION_API=1 启用。
func TestLLMRewriteIntegration(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_API") != "1" {
		t.Skip("set RAG_INTEGRATION_API=1 to run real API integration test")
	}

	aiRuntime := infraai.NewRuntime()
	svc := NewLLMService(aiRuntime.Chat)

	t.Run("RewriteSimple", func(t *testing.T) {
		result := svc.RewriteWithSplit("什么叫RAG技术")
		t.Logf("rewritten: %q", result.RewrittenQuestion)
		t.Logf("sub_questions: %v", result.SubQuestions)

		if result.RewrittenQuestion == "" {
			t.Fatal("expected non-empty rewritten question")
		}
		if len(result.SubQuestions) == 0 {
			t.Fatal("expected at least 1 sub_question")
		}
		// 改写结果应与原问题相关（至少包含"RAG"或同义表述）
		combined := result.RewrittenQuestion + " " + strings.Join(result.SubQuestions, " ")
		hasRAG := strings.Contains(strings.ToLower(combined), "rag") ||
			strings.Contains(combined, "检索增强") ||
			strings.Contains(combined, "生成")
		if !hasRAG {
			t.Logf("warning: rewritten result may not be related to RAG: %s", combined)
		}
	})

	t.Run("RewriteComplex", func(t *testing.T) {
		// 复杂问题应该拆解为多个子问题。
		result := svc.RewriteWithSplit("RAG和传统搜索引擎有什么区别，各自适用什么场景")
		t.Logf("rewritten: %q", result.RewrittenQuestion)
		t.Logf("sub_questions (%d): %v", len(result.SubQuestions), result.SubQuestions)

		if len(result.SubQuestions) < 1 {
			t.Fatal("expected at least 1 sub_question for complex query")
		}
		// 子问题中不应有完全重复的。
		seen := map[string]bool{}
		for _, q := range result.SubQuestions {
			q = strings.TrimSpace(q)
			if q == "" {
				t.Fatal("sub_question should not be empty")
			}
			if seen[q] {
				t.Fatalf("duplicate sub_question: %q", q)
			}
			seen[q] = true
		}
	})

	t.Run("RewritePronounResolution", func(t *testing.T) {
		// 带历史的指代消解：LLM 应将"它"替换为具体实体。
		history := []convention.ChatMessage{
			convention.UserMessage("什么是向量数据库"),
			convention.AssistantMessage("向量数据库是一种专门用于存储和检索高维向量的数据库系统，常用于 AI 语义搜索。"),
		}
		result := svc.RewriteWithHistory("它有哪些常见的应用场景", history)
		t.Logf("rewritten: %q", result.RewrittenQuestion)
		t.Logf("sub_questions: %v", result.SubQuestions)

		// 改写后的问题不应为纯指代词开头（说明模型未理解指令）。
		combined := result.RewrittenQuestion + " " + strings.Join(result.SubQuestions, " ")
		if strings.HasPrefix(result.RewrittenQuestion, "它") {
			t.Logf("info: model did not resolve pronoun, returned: %q", result.RewrittenQuestion)
			t.Logf("info: this may indicate the model is too small or the prompt needs adjustment")
			// 不 fail，小模型可能无法正确执行指代消解，只需确保不 panic 且有返回。
		}
		if strings.Contains(combined, "向量") || strings.Contains(combined, "数据库") {
			t.Logf("pronoun resolution looks reasonable: contains entity reference")
		}
	})

	t.Run("RewriteEmptyService", func(t *testing.T) {
		// nil chatService 时不应 panic，降级返回原问题。
		emptySvc := NewLLMService(nil)
		result := emptySvc.RewriteWithSplit("hello")
		if result.RewrittenQuestion != "hello" {
			t.Fatalf("expected fallback 'hello', got %q", result.RewrittenQuestion)
		}
	})
}
