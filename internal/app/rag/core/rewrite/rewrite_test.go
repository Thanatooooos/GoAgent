package rewrite

import (
	"fmt"
	"strings"
	"testing"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestParseRewriteResponseValidJSON(t *testing.T) {
	result := parseRewriteResponse(`{"rewritten":"什么是RAG","sub_questions":["RAG定义","RAG原理"],"need_retrieval":true}`)

	if result.RewrittenQuestion != "什么是RAG" {
		t.Fatalf("expected rewritten question, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 3 {
		t.Fatalf("expected 3 sub questions, got %d: %v", len(result.SubQuestions), result.SubQuestions)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected retrieval to be required")
	}
}

func TestParseRewriteResponseMarkdownJSONBlock(t *testing.T) {
	raw := "```json\n{\"rewritten\":\"hello world\",\"sub_questions\":[\"q1\"],\"need_retrieval\":false}\n```"
	result := parseRewriteResponse(raw)

	if result.RewrittenQuestion != "hello world" {
		t.Fatalf("expected rewritten question, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 2 {
		t.Fatalf("expected 2 sub questions, got %d: %v", len(result.SubQuestions), result.SubQuestions)
	}
	if result.NeedRetrieval {
		t.Fatal("expected retrieval to be skipped")
	}
}

func TestParseRewriteResponseFallbackOnInvalidJSON(t *testing.T) {
	result := parseRewriteResponse("这不是 JSON 格式的返回")

	if result.RewrittenQuestion != "这不是 JSON 格式的返回" {
		t.Fatalf("expected fallback to raw text, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 1 || result.SubQuestions[0] != "这不是 JSON 格式的返回" {
		t.Fatalf("expected single fallback sub question, got %v", result.SubQuestions)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected fallback text to require retrieval by default")
	}
}

func TestParseRewriteResponseEmpty(t *testing.T) {
	result := parseRewriteResponse("")
	if result.RewrittenQuestion != "" || len(result.SubQuestions) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestNormalizeSubQuestionsDedup(t *testing.T) {
	subs := normalizeSubQuestions([]string{"a", "b", "a", "c"}, "a")
	if len(subs) != 3 {
		t.Fatalf("expected 3 unique subs, got %d: %v", len(subs), subs)
	}
}

func TestExtractJSONBlockWithTag(t *testing.T) {
	raw := "some text\n```json\n{\"key\":\"value\"}\n```\nmore text"
	extracted := extractJSONBlock(raw)
	if extracted != `{"key":"value"}` {
		t.Fatalf("expected extracted JSON, got %q", extracted)
	}
}

func TestFallbackResultNormal(t *testing.T) {
	result := fallbackResult(" 解释一下RAG原理 ")
	if result.RewrittenQuestion != "解释一下RAG原理" {
		t.Fatalf("expected trimmed question, got %q", result.RewrittenQuestion)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected retrieval for normal question")
	}
}

func TestFallbackResultSmallTalk(t *testing.T) {
	result := fallbackResult("你好")
	if result.NeedRetrieval {
		t.Fatal("expected small talk to skip retrieval")
	}
}

func TestBuildRewriteHistoryPrompt(t *testing.T) {
	history := []convention.ChatMessage{
		convention.UserMessage("什么是向量检索"),
		convention.AssistantMessage("向量检索是一种基于 embedding 的检索方式。"),
	}
	prompt := buildRewriteHistoryPrompt("base", history, "它有什么优点")
	if !strings.Contains(prompt, "什么是向量检索") {
		t.Fatal("expected history in prompt")
	}
	if !strings.Contains(prompt, "用户：") || !strings.Contains(prompt, "助手：") {
		t.Fatal("expected role labels in prompt")
	}
}

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
		response: `{"rewritten":"RAG技术原理","sub_questions":["RAG定义","RAG架构"],"need_retrieval":true}`,
	}
	svc := NewLLMService(mock)

	result := svc.RewriteWithSplit("什么是RAG")
	if result.RewrittenQuestion != "RAG技术原理" {
		t.Fatalf("expected rewritten question, got %q", result.RewrittenQuestion)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected retrieval to be required")
	}
}

func TestLLMServiceRewriteWithSplitLLMFailure(t *testing.T) {
	mock := &mockLLMService{
		response: "unused",
		err:      fmt.Errorf("llm service unavailable"),
	}
	svc := NewLLMService(mock)

	result := svc.RewriteWithSplit("hello")
	if result.RewrittenQuestion != "hello" {
		t.Fatalf("expected fallback to original on LLM failure, got %q", result.RewrittenQuestion)
	}
}

func TestLLMServiceRewriteWithHistorySuccess(t *testing.T) {
	mock := &mockLLMService{
		response: `{"rewritten":"向量检索的优点","sub_questions":["向量检索优点"],"need_retrieval":true}`,
	}
	svc := NewLLMService(mock)

	history := []convention.ChatMessage{
		convention.UserMessage("什么是向量检索"),
		convention.AssistantMessage("向量检索是一种基于 embedding 的检索方式。"),
	}
	result := svc.RewriteWithHistory("它有什么优点", history)
	if result.RewrittenQuestion != "向量检索的优点" {
		t.Fatalf("expected rewritten with history, got %q", result.RewrittenQuestion)
	}
}

func TestNormalizeNeedRetrievalFallback(t *testing.T) {
	if normalizeNeedRetrieval(nil, "你好") {
		t.Fatal("expected greeting to skip retrieval")
	}
	if !normalizeNeedRetrieval(nil, "解释一下RAG原理") {
		t.Fatal("expected knowledge question to require retrieval")
	}
}
