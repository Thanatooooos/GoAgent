package planner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/tool"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type mockLLMService struct {
	response string
	err      error
}

func (m *mockLLMService) Chat(string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockLLMService) ChatWithRequest(convention.ChatRequest) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockLLMService) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, errors.New("not implemented")
}

var sampleDefs = []tool.Definition{
	{
		Name:        "document_query",
		Description: "查询知识库文档状态和详情",
		Parameters: []tool.ParameterDefinition{
			{Name: "documentId", Type: "string", Description: "文档ID", Required: true},
		},
	},
	{
		Name:        "ingestion_task_query",
		Description: "查询数据导入任务状态",
		Parameters: []tool.ParameterDefinition{
			{Name: "taskId", Type: "string", Description: "任务ID", Required: true},
			{Name: "includeNodes", Type: "boolean", Description: "是否包含节点详情", Required: false},
		},
	},
}

func TestLLMPlanner_Plan_SingleTool(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[{"name":"document_query","arguments":{"documentId":"doc-abc"}}]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "document-doc-abc 的状态怎么样？",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasTools() {
		t.Fatal("expected tools to be planned")
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	c := result.Calls[0]
	if c.Name != "document_query" {
		t.Errorf("expected document_query, got %s", c.Name)
	}
}

func TestLLMPlanner_Plan_MultipleTools(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[{"name":"document_query","arguments":{"documentId":"doc-abc"}},{"name":"ingestion_task_query","arguments":{"taskId":"task-xyz","includeNodes":true}}]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc 和 task-xyz",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(result.Calls))
	}
}

func TestLLMPlanner_Plan_NoTools(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "你好，今天天气怎么样？",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools, got some")
	}
}

func TestLLMPlanner_Plan_MarkdownJSONBlock(t *testing.T) {
	mock := &mockLLMService{
		response: "```json\n{\"tools\":[{\"name\":\"document_query\",\"arguments\":{\"documentId\":\"doc-abc\"}}]}\n```",
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
}

func TestLLMPlanner_Plan_EmptyQuestion(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for empty question")
	}
}

func TestLLMPlanner_Plan_NoDefinitions(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools when no definitions")
	}
}

func TestLLMPlanner_Plan_LLMError(t *testing.T) {
	mock := &mockLLMService{
		err: errors.New("llm unavailable"),
	}
	planner := NewLLMPlanner(mock)

	_, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLLMPlanner_Plan_MalformedJSON(t *testing.T) {
	mock := &mockLLMService{
		response: "not valid json at all",
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("malformed JSON should not error, got: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for malformed JSON")
	}
}

func TestLLMPlanner_Plan_NilPlanner(t *testing.T) {
	var p *LLMPlanner
	result, err := p.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for nil planner")
	}
}

func TestLLMPlanner_Plan_NilChatService(t *testing.T) {
	p := &LLMPlanner{chatService: nil}
	result, err := p.Plan(context.Background(), tool.PlanInput{
		Question:        "查一下 doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for nil chat service")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	p := NewLLMPlanner(&mockLLMService{response: "{}"})
	prompt := p.buildSystemPrompt(sampleDefs)

	if !strings.Contains(prompt, "document_query") {
		t.Error("prompt should contain document_query")
	}
	if !strings.Contains(prompt, "ingestion_task_query") {
		t.Error("prompt should contain ingestion_task_query")
	}
	if !strings.Contains(prompt, "必填") {
		t.Error("prompt should mark required params")
	}
}

func TestExtractJSONBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"json marker", "```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"plain marker", "```\n{\"a\":1}\n```", "{\"a\":1}"},
		{"no block", "plain text", ""},
		{"no closing", "```json\n{\"a\":1}", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONBlock(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
