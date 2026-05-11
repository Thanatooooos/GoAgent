package planner

import (
	"context"
	"errors"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
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
		Description: "query document status",
		Parameters: []tool.ParameterDefinition{
			{Name: "documentId", Type: "string", Description: "document id", Required: true},
		},
	},
	{
		Name:        "ingestion_task_query",
		Description: "query ingestion task",
		Parameters: []tool.ParameterDefinition{
			{Name: "taskId", Type: "string", Description: "task id", Required: true},
			{Name: "includeNodes", Type: "boolean", Description: "include node details", Required: false},
		},
	},
}

func TestLLMPlannerPlanSingleTool(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[{"name":"document_query","arguments":{"documentId":"doc-abc"}}]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "what is the state of doc-abc?",
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
	if result.Calls[0].Name != "document_query" {
		t.Fatalf("expected document_query, got %s", result.Calls[0].Name)
	}
}

func TestLLMPlannerPlanMultipleTools(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[{"name":"document_query","arguments":{"documentId":"doc-abc"}},{"name":"ingestion_task_query","arguments":{"taskId":"task-xyz","includeNodes":true}}]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc and task-xyz",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(result.Calls))
	}
}

func TestLLMPlannerPlanNoTools(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "hello there",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools")
	}
}

func TestLLMPlannerPlanMarkdownJSONBlock(t *testing.T) {
	mock := &mockLLMService{
		response: "```json\n{\"tools\":[{\"name\":\"document_query\",\"arguments\":{\"documentId\":\"doc-abc\"}}]}\n```",
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
}

func TestLLMPlannerPlanEmptyQuestion(t *testing.T) {
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

func TestLLMPlannerPlanNoDefinitions(t *testing.T) {
	mock := &mockLLMService{
		response: `{"tools":[]}`,
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
		ToolDefinitions: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools when no definitions")
	}
}

func TestLLMPlannerPlanLLMError(t *testing.T) {
	mock := &mockLLMService{
		err: errors.New("llm unavailable"),
	}
	planner := NewLLMPlanner(mock)

	_, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLLMPlannerPlanMalformedJSON(t *testing.T) {
	mock := &mockLLMService{
		response: "not valid json at all",
	}
	planner := NewLLMPlanner(mock)

	result, err := planner.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("malformed JSON should not error, got: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for malformed JSON")
	}
}

func TestLLMPlannerPlanNilPlanner(t *testing.T) {
	var p *LLMPlanner
	result, err := p.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
		ToolDefinitions: sampleDefs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasTools() {
		t.Fatal("expected no tools for nil planner")
	}
}

func TestLLMPlannerPlanNilChatService(t *testing.T) {
	p := &LLMPlanner{chatService: nil}
	result, err := p.Plan(context.Background(), tool.PlanInput{
		Question:        "check doc-abc",
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
	if !strings.Contains(prompt, "required") {
		t.Error("prompt should mark required params")
	}
	if !strings.Contains(prompt, "Examples:") {
		t.Error("prompt should contain examples for incremental planning")
	}
	if !strings.Contains(prompt, "Only plan multiple tool calls in the same round when they are independent") {
		t.Error("prompt should include explicit parallel planning constraint")
	}
	if !strings.Contains(prompt, "avoid combinations like document_query + document_ingestion_diagnose") {
		t.Error("prompt should include same-entity serial drill-down examples")
	}
}

func TestBuildUserPromptIncludesHintAndPreviousResults(t *testing.T) {
	p := NewLLMPlanner(&mockLLMService{response: "{}"})
	prompt := p.buildUserPrompt(tool.PlanInput{
		Question: "why did doc-1 fail",
		AgentState: tool.AgentState{
			Phase:      "deep_dive",
			Hypothesis: "document ingestion failed at node indexer",
			Confidence: 0.72,
			NextHintCalls: []tool.HintCall{{
				Name: "ingestion_task_node_query",
				Arguments: map[string]any{
					"taskId": "task-1",
					"nodeId": "indexer",
				},
			}},
		},
		PreviousResults: []tool.Result{
			{
				Name:    "document_ingestion_diagnose",
				Summary: "document ingestion failed at node indexer",
				Data: map[string]any{
					"latestTaskId": "task-1",
					"latestNodeId": "indexer",
				},
			},
		},
	})

	if !strings.Contains(prompt, "Current agent state") {
		t.Fatal("prompt should include the structured agent state section")
	}
	if !strings.Contains(prompt, "tool:ingestion_task_node_query|taskId=task-1|nodeId=indexer") {
		t.Fatal("prompt should include the structured hint content")
	}
	if !strings.Contains(prompt, "\"nextHintCalls\"") {
		t.Fatal("prompt should include structured nextHintCalls content")
	}
	if !strings.Contains(prompt, "\"phase\":\"deep_dive\"") {
		t.Fatal("prompt should include the normalized agent phase")
	}
	if !strings.Contains(prompt, "document_ingestion_diagnose: document ingestion failed at node indexer") {
		t.Fatal("prompt should include previous result summaries")
	}
	if !strings.Contains(prompt, "latestNodeId=indexer") {
		t.Fatal("prompt should include structured result data")
	}
	if !strings.Contains(prompt, "Parallel planning guidance:") {
		t.Fatal("prompt should include parallel planning guidance")
	}
	if !strings.Contains(prompt, "For the same entity, prefer one incremental lookup per round") {
		t.Fatal("prompt should include same-entity serial planning guidance")
	}
}

func TestBuildUserPromptIncludesRewriteAndRetrieveContext(t *testing.T) {
	p := NewLLMPlanner(&mockLLMService{response: "{}"})
	prompt := p.buildUserPrompt(tool.PlanInput{
		Question: "why did task-1 fail?",
		RewriteResult: ragrewrite.Result{
			RewrittenQuestion:   "why did ingestion task task-1 fail?",
			SubQuestions:        []string{"which node failed?", "what error was returned?"},
			PreferredSearchMode: ragretrieve.SearchModeHybrid,
		},
		RetrieveResult: ragretrieve.Result{
			SearchChannels: []string{ragretrieve.ChannelKeyword, ragretrieve.ChannelVectorGlobal},
			ChannelStats: []ragretrieve.ChannelStat{
				{Name: ragretrieve.ChannelKeyword, ChunkCount: 2},
			},
			Chunks: []convention.RetrievedChunk{
				{
					ID:   "chunk-1",
					Text: "The indexer node can fail with connection refused when the vector store is unavailable.",
					Metadata: map[string]any{
						"section":          "Failure troubleshooting",
						"source_file_name": "ingestion_failures.md",
					},
				},
			},
		},
	})

	if !strings.Contains(prompt, "Rewrite context:") {
		t.Fatal("prompt should include rewrite context")
	}
	if !strings.Contains(prompt, "rewrittenQuestion=why did ingestion task task-1 fail?") {
		t.Fatal("prompt should include rewritten question summary")
	}
	if !strings.Contains(prompt, "Retrieve context:") {
		t.Fatal("prompt should include retrieve context")
	}
	if !strings.Contains(prompt, "searchChannels=keyword, vector_global") {
		t.Fatal("prompt should include retrieve channels")
	}
	if !strings.Contains(prompt, "file=ingestion_failures.md") {
		t.Fatal("prompt should include retrieved chunk summary")
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
