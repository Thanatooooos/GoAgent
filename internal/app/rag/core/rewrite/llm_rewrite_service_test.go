package rewrite

import (
	"fmt"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestParseRewriteResponseValidJSON(t *testing.T) {
	result := parseRewriteResponse(`{"rewritten": "什么是RAG", "sub_questions": ["RAG定义", "RAG原理"], "preferred_search_mode": "semantic"}`)

	if result.RewrittenQuestion != "什么是RAG" {
		t.Fatalf("expected rewritten '什么是RAG', got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 3 {
		t.Fatalf("expected 3 sub_questions (rewritten + 2 subs), got %d: %v", len(result.SubQuestions), result.SubQuestions)
	}
	if result.PreferredSearchMode != ragretrieve.SearchModeSemantic {
		t.Fatalf("expected semantic, got %q", result.PreferredSearchMode)
	}
}

func TestParseRewriteResponseMarkdownJSONBlock(t *testing.T) {
	raw := "```json\n{\"rewritten\": \"hello world\", \"sub_questions\": [\"q1\"]}\n```"
	result := parseRewriteResponse(raw)

	if result.RewrittenQuestion != "hello world" {
		t.Fatalf("expected rewritten 'hello world', got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 2 {
		t.Fatalf("expected 2 sub_questions, got %d: %v", len(result.SubQuestions), result.SubQuestions)
	}
}

func TestParseRewriteResponseFallbackOnInvalidJSON(t *testing.T) {
	result := parseRewriteResponse("这不是 JSON 格式的返回")

	if result.RewrittenQuestion != "这不是 JSON 格式的返回" {
		t.Fatalf("expected fallback to raw text, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 1 || result.SubQuestions[0] != "这不是 JSON 格式的返回" {
		t.Fatalf("expected single sub_question from fallback, got %v", result.SubQuestions)
	}
}

func TestParseRewriteResponseEmpty(t *testing.T) {
	result := parseRewriteResponse("")

	if result.RewrittenQuestion != "" {
		t.Fatalf("expected empty rewritten question, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 0 {
		t.Fatalf("expected empty sub_questions, got %v", result.SubQuestions)
	}
}

func TestParseRewriteResponseWhitespaceOnly(t *testing.T) {
	result := parseRewriteResponse("   ")

	if result.RewrittenQuestion != "" {
		t.Fatalf("expected empty, got %q", result.RewrittenQuestion)
	}
}

func TestNormalizeSubQuestionsDedup(t *testing.T) {
	subs := normalizeSubQuestions([]string{"a", "b", "a", "c"}, "a")
	if len(subs) != 3 {
		t.Fatalf("expected 3 unique subs, got %d: %v", len(subs), subs)
	}
}

func TestNormalizeSubQuestionsEmptyInput(t *testing.T) {
	// 两个输入都为空时，不产生有效子问题。
	subs := normalizeSubQuestions(nil, "")
	if len(subs) != 0 {
		t.Fatalf("expected empty, got %v", subs)
	}
}

func TestNormalizeSubQuestionsEmptyRewrittenWithRaw(t *testing.T) {
	// rewritten 为空但 raw 有值时，只返回去重后的 raw。
	subs := normalizeSubQuestions([]string{"a", "b", "a"}, "")
	if len(subs) != 2 || subs[0] != "a" || subs[1] != "b" {
		t.Fatalf("expected [a b], got %v", subs)
	}
}

func TestExtractJSONBlockWithTag(t *testing.T) {
	raw := "some text\n```json\n{\"key\": \"value\"}\n```\nmore text"
	extracted := extractJSONBlock(raw)
	if extracted != `{"key": "value"}` {
		t.Fatalf("expected extracted JSON, got %q", extracted)
	}
}

func TestExtractJSONBlockWithoutTag(t *testing.T) {
	raw := "text\n```\nplain content\n```\nend"
	extracted := extractJSONBlock(raw)
	if extracted != "plain content" {
		t.Fatalf("expected plain content, got %q", extracted)
	}
}

func TestExtractJSONBlockNoMarkers(t *testing.T) {
	raw := `{"key": "value"}`
	extracted := extractJSONBlock(raw)
	if extracted != "" {
		t.Fatalf("expected empty on no markers, got %q", extracted)
	}
}

func TestFallbackResultNormal(t *testing.T) {
	result := fallbackResult(" hello ")
	if result.RewrittenQuestion != "hello" {
		t.Fatalf("expected trimmed 'hello', got %q", result.RewrittenQuestion)
	}
	if result.PreferredSearchMode == "" {
		t.Fatal("expected preferred search mode")
	}
}

func TestFallbackResultEmpty(t *testing.T) {
	result := fallbackResult("")
	if result.RewrittenQuestion != "" || len(result.SubQuestions) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestBuildRewriteHistoryPrompt(t *testing.T) {
	history := []convention.ChatMessage{
		convention.UserMessage("什么是向量检索"),
		convention.AssistantMessage("向量检索是一种..."),
	}
	prompt := buildRewriteHistoryPrompt("base", history, "它有什么优点")
	if !strings.Contains(prompt, "什么是向量检索") {
		t.Fatal("expected history in prompt")
	}
	if !strings.Contains(prompt, "用户：") || !strings.Contains(prompt, "助手：") {
		t.Fatal("expected role labels in prompt")
	}
	if !strings.Contains(prompt, "指代消解") {
		t.Fatal("expected co-reference instruction in prompt")
	}
}

func TestBuildRewriteHistoryPromptEmpty(t *testing.T) {
	prompt := buildRewriteHistoryPrompt("base", nil, "q")
	if prompt != "base" {
		t.Fatalf("expected unchanged base, got %q", prompt)
	}
}

// mockLLMService 用于测试的 LLM 服务桩。
type mockLLMService struct {
	response string
	err      error
}

func (m *mockLLMService) Chat(prompt string) (string, error) {
	return m.response, m.err
}

func (m *mockLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	return m.response, m.err
}

func (m *mockLLMService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	return m.response, m.err
}

func (m *mockLLMService) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (m *mockLLMService) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*mockLLMService)(nil)

func TestLLMServiceRewriteWithSplitSuccess(t *testing.T) {
	mock := &mockLLMService{
		response: `{"rewritten": "RAG技术原理", "sub_questions": ["RAG定义", "RAG架构"], "preferred_search_mode": "semantic"}`,
	}
	svc := NewLLMService(mock)

	result := svc.RewriteWithSplit("什么是RAG")
	if result.RewrittenQuestion != "RAG技术原理" {
		t.Fatalf("expected rewritten 'RAG技术原理', got %q", result.RewrittenQuestion)
	}
	if result.PreferredSearchMode != ragretrieve.SearchModeSemantic {
		t.Fatalf("expected semantic, got %q", result.PreferredSearchMode)
	}
}

func TestLLMServiceRewriteWithSplitLLMFailure(t *testing.T) {
	mock := &mockLLMService{
		response: "unused",
		err:      fmt.Errorf("llm service unavailable"),
	}
	svc := NewLLMService(mock)

	// LLM 调用失败时应该降级返回原问题。
	result := svc.RewriteWithSplit("hello")
	if result.RewrittenQuestion != "hello" {
		t.Fatalf("expected fallback to original on LLM failure, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 1 || result.SubQuestions[0] != "hello" {
		t.Fatalf("expected single sub_question from fallback, got %v", result.SubQuestions)
	}
}

func TestLLMServiceRewriteNilService(t *testing.T) {
	result := (&LLMService{}).RewriteWithSplit("hello")
	if result.RewrittenQuestion != "hello" {
		t.Fatalf("expected fallback on nil, got %q", result.RewrittenQuestion)
	}
}

func TestLLMServiceRewriteEmptyQuestion(t *testing.T) {
	mock := &mockLLMService{response: "should not be called"}
	svc := NewLLMService(mock)

	result := svc.RewriteWithSplit("")
	if result.RewrittenQuestion != "" || len(result.SubQuestions) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestLLMServiceRewrite(t *testing.T) {
	mock := &mockLLMService{
		response: `{"rewritten": "优化后的问题", "sub_questions": ["q1"]}`,
	}
	svc := NewLLMService(mock)

	rewritten := svc.Rewrite("原始问题")
	if rewritten != "优化后的问题" {
		t.Fatalf("expected '优化后的问题', got %q", rewritten)
	}
}

func TestLLMServiceRewriteWithHistorySuccess(t *testing.T) {
	mock := &mockLLMService{
		response: `{"rewritten": "向量检索的优点", "sub_questions": ["向量检索优势", "为什么用向量检索"], "preferred_search_mode": "semantic"}`,
	}
	svc := NewLLMService(mock)

	history := []convention.ChatMessage{
		convention.UserMessage("什么是向量检索"),
		convention.AssistantMessage("向量检索是一种基于embedding的检索方式"),
	}
	result := svc.RewriteWithHistory("它有什么优点", history)
	if result.RewrittenQuestion != "向量检索的优点" {
		t.Fatalf("expected rewritten with history context, got %q", result.RewrittenQuestion)
	}
}

func TestNormalizePreferredSearchModeFallback(t *testing.T) {
	mode := normalizePreferredSearchMode("", "nginx 配置报错 404")
	if mode != ragretrieve.SearchModeHybrid {
		t.Fatalf("expected hybrid, got %q", mode)
	}
}
