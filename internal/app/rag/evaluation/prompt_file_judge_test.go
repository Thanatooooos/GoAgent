package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type promptJudgeLLMStub struct {
	response  string
	responses []string
	requests  []convention.ChatRequest
}

func (s *promptJudgeLLMStub) nextResponse() string {
	if len(s.responses) == 0 {
		return s.response
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response
}

func (s *promptJudgeLLMStub) Chat(string) (string, error) { return s.nextResponse(), nil }
func (s *promptJudgeLLMStub) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.nextResponse(), nil
}
func (s *promptJudgeLLMStub) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	s.requests = append(s.requests, request)
	return s.nextResponse(), nil
}
func (s *promptJudgeLLMStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *promptJudgeLLMStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func TestPromptFileJudgeEvaluateLoadsAssetsAndParsesJSON(t *testing.T) {
	root := t.TempDir()
	promptPath := filepath.Join(root, "judge_prompts")
	rubricPath := filepath.Join(root, "rubrics")
	if err := os.MkdirAll(promptPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(promptPath) error = %v", err)
	}
	if err := os.MkdirAll(rubricPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(rubricPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptPath, "summary.field.v1.md"), []byte("Prompt body"), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rubricPath, "summary.field.v1.md"), []byte("Rubric body"), 0o644); err != nil {
		t.Fatalf("WriteFile(rubric) error = %v", err)
	}

	llm := &promptJudgeLLMStub{
		response: "```json\n{\"passed\":true,\"score\":0.9,\"reason\":\"ok\",\"details\":{\"fields\":{\"goal\":{\"fidelity\":1,\"usefulness\":1}}}}\n```",
	}
	judge := NewPromptFileJudge(llm, root)
	result, err := judge.Evaluate(context.Background(), JudgeRequest{
		PromptRef: "summary.field.v1",
		RubricRef: "summary.field.v1",
		Payload:   map[string]any{"sample_name": "sample-1"},
		Config: JudgeConfig{
			Temperature: 0,
			MaxTokens:   700,
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed {
		t.Fatal("result expected passed")
	}
	if result.Score != 0.9 {
		t.Fatalf("result.Score = %v, want 0.9", result.Score)
	}
	if len(llm.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(llm.requests))
	}
	request := llm.requests[0]
	if request.JSONMode == nil || !*request.JSONMode {
		t.Fatalf("JSONMode = %+v, want true", request.JSONMode)
	}
	if request.MaxTokens == nil || *request.MaxTokens != 700 {
		t.Fatalf("MaxTokens = %+v, want 700", request.MaxTokens)
	}
}

func TestPromptFileJudgeEvaluateRetriesTruncatedJSONWithoutJSONMode(t *testing.T) {
	root := t.TempDir()
	promptPath := filepath.Join(root, "judge_prompts")
	rubricPath := filepath.Join(root, "rubrics")
	if err := os.MkdirAll(promptPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(promptPath) error = %v", err)
	}
	if err := os.MkdirAll(rubricPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(rubricPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptPath, "summary.field.v1.md"), []byte("Prompt body"), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rubricPath, "summary.field.v1.md"), []byte("Rubric body"), 0o644); err != nil {
		t.Fatalf("WriteFile(rubric) error = %v", err)
	}

	llm := &promptJudgeLLMStub{
		responses: []string{
			"{\"passed\":false,\"score\":0.2",
			"{\"passed\":true,\"score\":1,\"reason\":\"retry-ok\"}",
		},
	}
	judge := NewPromptFileJudge(llm, root)
	result, err := judge.Evaluate(context.Background(), JudgeRequest{
		PromptRef: "summary.field.v1",
		RubricRef: "summary.field.v1",
		Payload:   map[string]any{"sample_name": "sample-1"},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed {
		t.Fatal("result expected passed after retry")
	}
	if len(llm.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(llm.requests))
	}
	if llm.requests[0].JSONMode == nil || !*llm.requests[0].JSONMode {
		t.Fatalf("first JSONMode = %+v, want true", llm.requests[0].JSONMode)
	}
	if llm.requests[1].JSONMode == nil || *llm.requests[1].JSONMode {
		t.Fatalf("second JSONMode = %+v, want false", llm.requests[1].JSONMode)
	}
}

func TestPromptFileJudgeEvaluateRetriesMultipleTruncatedJSONResponses(t *testing.T) {
	root := t.TempDir()
	promptPath := filepath.Join(root, "judge_prompts")
	rubricPath := filepath.Join(root, "rubrics")
	if err := os.MkdirAll(promptPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(promptPath) error = %v", err)
	}
	if err := os.MkdirAll(rubricPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(rubricPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptPath, "summary.field.v1.md"), []byte("Prompt body"), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rubricPath, "summary.field.v1.md"), []byte("Rubric body"), 0o644); err != nil {
		t.Fatalf("WriteFile(rubric) error = %v", err)
	}

	llm := &promptJudgeLLMStub{
		responses: []string{
			"{\"passed\":false,\"score\":0.2",
			"{\"passed\":false,\"score\":0.3",
			"{\"passed\":true,\"score\":1,\"reason\":\"retry-ok\"}",
		},
	}
	judge := NewPromptFileJudge(llm, root)
	result, err := judge.Evaluate(context.Background(), JudgeRequest{
		PromptRef: "summary.field.v1",
		RubricRef: "summary.field.v1",
		Payload:   map[string]any{"sample_name": "sample-1"},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed {
		t.Fatal("result expected passed after retry")
	}
	if len(llm.requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(llm.requests))
	}
	if llm.requests[0].JSONMode == nil || !*llm.requests[0].JSONMode {
		t.Fatalf("first JSONMode = %+v, want true", llm.requests[0].JSONMode)
	}
	if llm.requests[1].JSONMode == nil || *llm.requests[1].JSONMode {
		t.Fatalf("second JSONMode = %+v, want false", llm.requests[1].JSONMode)
	}
	if llm.requests[2].JSONMode == nil || *llm.requests[2].JSONMode {
		t.Fatalf("third JSONMode = %+v, want false", llm.requests[2].JSONMode)
	}
}

func TestPromptFileJudgeEvaluateRetriesEmptyResponseWithoutJSONMode(t *testing.T) {
	root := t.TempDir()
	promptPath := filepath.Join(root, "judge_prompts")
	rubricPath := filepath.Join(root, "rubrics")
	if err := os.MkdirAll(promptPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(promptPath) error = %v", err)
	}
	if err := os.MkdirAll(rubricPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(rubricPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptPath, "summary.field.v1.md"), []byte("Prompt body"), 0o644); err != nil {
		t.Fatalf("WriteFile(prompt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rubricPath, "summary.field.v1.md"), []byte("Rubric body"), 0o644); err != nil {
		t.Fatalf("WriteFile(rubric) error = %v", err)
	}

	llm := &promptJudgeLLMStub{
		responses: []string{
			"   ",
			"{\"passed\":true,\"score\":1,\"reason\":\"retry-ok\"}",
		},
	}
	judge := NewPromptFileJudge(llm, root)
	result, err := judge.Evaluate(context.Background(), JudgeRequest{
		PromptRef: "summary.field.v1",
		RubricRef: "summary.field.v1",
		Payload:   map[string]any{"sample_name": "sample-1"},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed {
		t.Fatal("result expected passed after retry")
	}
	if len(llm.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(llm.requests))
	}
	if llm.requests[0].JSONMode == nil || !*llm.requests[0].JSONMode {
		t.Fatalf("first JSONMode = %+v, want true", llm.requests[0].JSONMode)
	}
	if llm.requests[1].JSONMode == nil || *llm.requests[1].JSONMode {
		t.Fatalf("second JSONMode = %+v, want false", llm.requests[1].JSONMode)
	}
}
