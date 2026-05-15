package rewrite

import (
	"encoding/json"
	"fmt"
	"testing"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type stubLLMService struct {
	response string
	err      error
	requests []convention.ChatRequest
}

func (s *stubLLMService) Chat(prompt string) (string, error) {
	return s.response, s.err
}

func (s *stubLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

func (s *stubLLMService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

func (s *stubLLMService) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubLLMService) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func TestLLMRewriteServiceParsesNeedRetrieval(t *testing.T) {
	llm := &stubLLMService{
		response: `{"rewritten":"排查 nginx 404 配置错误","sub_questions":["nginx 404 原因","nginx location 配置"],"need_retrieval":true}`,
	}
	service := NewLLMService(llm)

	result := service.RewriteWithSplit("nginx 配置报错 404")
	if result.RewrittenQuestion != "排查 nginx 404 配置错误" {
		t.Fatalf("unexpected rewritten question: %q", result.RewrittenQuestion)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected retrieval to be required")
	}
	if len(result.SubQuestions) != 3 {
		t.Fatalf("expected rewritten question plus 2 sub questions, got %v", result.SubQuestions)
	}
}

func TestLLMRewriteServiceFallsBackWhenJSONInvalid(t *testing.T) {
	llm := &stubLLMService{response: "not json"}
	service := NewLLMService(llm)

	result := service.RewriteWithSplit("hello")
	if result.RewrittenQuestion != "not json" {
		t.Fatalf("expected fallback to raw model text, got %q", result.RewrittenQuestion)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected fallback text to infer retrieval")
	}
}

func TestLLMRewriteServiceFallsBackWhenLLMErrors(t *testing.T) {
	llm := &stubLLMService{err: fmt.Errorf("boom")}
	service := NewLLMService(llm)

	result := service.RewriteWithSplit("你好")
	if result.RewrittenQuestion != "你好" {
		t.Fatalf("expected original question on error, got %q", result.RewrittenQuestion)
	}
	if result.NeedRetrieval {
		t.Fatal("expected greeting fallback to skip retrieval")
	}
}

func TestLLMRewriteServiceIncludesHistoryInPrompt(t *testing.T) {
	llm := &stubLLMService{
		response: `{"rewritten":"向量检索的优点","sub_questions":["向量检索优点"],"need_retrieval":true}`,
	}
	service := NewLLMService(llm)

	history := []convention.ChatMessage{
		convention.UserMessage("什么是向量检索"),
		convention.AssistantMessage("向量检索是一种基于 embedding 的检索方式。"),
	}

	_ = service.RewriteWithHistory("它有什么优点", history)

	if len(llm.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(llm.requests))
	}
	if len(llm.requests[0].Messages) != 2 {
		t.Fatalf("expected system and user messages, got %d", len(llm.requests[0].Messages))
	}
	if llm.requests[0].Messages[0].Role != convention.SystemRole {
		t.Fatalf("expected system prompt, got %q", llm.requests[0].Messages[0].Role)
	}
}

func TestParseRewriteResponseIgnoresUnknownFields(t *testing.T) {
	payload := map[string]any{
		"rewritten":       "解释 RAG 工作流",
		"sub_questions":   []string{"RAG 定义"},
		"need_retrieval":  true,
		"preferred_mode":  "hybrid",
		"unexpectedField": 123,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result := parseRewriteResponse(string(raw))
	if result.RewrittenQuestion != "解释 RAG 工作流" {
		t.Fatalf("unexpected rewritten question: %q", result.RewrittenQuestion)
	}
	if !result.NeedRetrieval {
		t.Fatal("expected retrieval to remain true")
	}
}
