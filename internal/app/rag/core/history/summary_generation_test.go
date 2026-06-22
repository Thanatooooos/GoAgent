package history

import (
	"context"
	"testing"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type llmServiceStub struct {
	response    string
	lastRequest convention.ChatRequest
}

func (s *llmServiceStub) Chat(_ string) (string, error) { return s.response, nil }

func (s *llmServiceStub) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.lastRequest = request
	return s.response, nil
}

func (s *llmServiceStub) ChatWithModel(_ convention.ChatRequest, _ string) (string, error) {
	return s.response, nil
}

func (s *llmServiceStub) StreamChat(_ string, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *llmServiceStub) StreamChatWithRequest(_ convention.ChatRequest, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func TestGenerateStructuredSummary(t *testing.T) {
	llm := &llmServiceStub{
		response: `{"schema_version":1,"goal":"当前主目标是先做 spec","constraints":["当前不进入实现"],"recent_progress":["已整理样本规范"]}`,
	}

	output, err := GenerateStructuredSummary(context.Background(), llm, GenerateStructuredSummaryInput{
		SourceMessages: []domain.ConversationMessage{
			{Role: "user", Content: "先做 spec，不进入实现。"},
			{Role: "assistant", Content: "收到，当前先整理 summary 样本规范。"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateStructuredSummary() error = %v", err)
	}
	if output.Structured.Goal != "当前主目标是先做 spec" {
		t.Fatalf("goal = %q, want 当前主目标是先做 spec", output.Structured.Goal)
	}
	if !output.Validation.Accepted {
		t.Fatalf("validation expected accepted, got %+v", output.Validation)
	}
	if output.Rendered == "" {
		t.Fatal("rendered summary should not be empty")
	}
	if len(llm.lastRequest.Messages) == 0 {
		t.Fatal("expected chat request to be sent")
	}
}
