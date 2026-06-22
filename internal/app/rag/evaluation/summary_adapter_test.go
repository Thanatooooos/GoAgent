package evaluation

import (
	"context"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type evaluationLLMStub struct {
	response string
}

func (s *evaluationLLMStub) Chat(_ string) (string, error) { return s.response, nil }
func (s *evaluationLLMStub) ChatWithRequest(_ convention.ChatRequest) (string, error) {
	return s.response, nil
}
func (s *evaluationLLMStub) ChatWithModel(_ convention.ChatRequest, _ string) (string, error) {
	return s.response, nil
}
func (s *evaluationLLMStub) StreamChat(_ string, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *evaluationLLMStub) StreamChatWithRequest(_ convention.ChatRequest, _ aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func TestHistorySummaryGeneratorGenerate(t *testing.T) {
	generator := NewHistorySummaryGenerator(&evaluationLLMStub{
		response: `{"schema_version":1,"goal":"当前主目标是先做 spec","constraints":["当前不进入实现"],"recent_progress":["已整理评测规范"]}`,
	}, raghistory.SummaryBudgetOptions{})

	output, err := generator.Generate(context.Background(), SummaryGenerationInput{
		SourceMessages: []SummaryMessage{
			{Role: "user", Content: "先做 spec，不进入实现。"},
			{Role: "assistant", Content: "收到，先整理评测规范。"},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if output.Structured.Goal != "当前主目标是先做 spec" {
		t.Fatalf("goal = %q, want 当前主目标是先做 spec", output.Structured.Goal)
	}
	if output.Rendered == "" {
		t.Fatal("rendered summary should not be empty")
	}
}
