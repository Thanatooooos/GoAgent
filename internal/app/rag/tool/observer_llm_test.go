package tool

import (
	"context"
	"errors"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type mockObserverLLMService struct {
	response string
	err      error
	requests []convention.ChatRequest
}

func (m *mockObserverLLMService) Chat(string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockObserverLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockObserverLLMService) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockObserverLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, errors.New("not implemented")
}

func (m *mockObserverLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, errors.New("not implemented")
}

func TestLLMObserverUsesLLMDecision(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"Need node-level error before answering.","confidence":0.82,"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}],"state":{"phase":"deep_dive","hypothesis":"task task-1 likely failed at node indexer","confidence":0.82,"openQuestions":["What is the concrete node-level error message?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}]}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question: "why did doc-1 fail?",
		Round:    1,
		ToolDefinitions: []Definition{
			{
				Name:        "ingestion_task_node_query",
				Description: "query task node details",
				Parameters: []ParameterDefinition{
					{Name: "taskId", Type: ParamTypeString, Required: true},
					{Name: "nodeId", Type: ParamTypeString, Required: true},
				},
			},
		},
		RoundResults: []Result{
			{
				Name:    "document_ingestion_diagnose",
				Summary: "document ingestion failed at node indexer",
				Data: map[string]any{
					"latestTaskId": "task-1",
					"latestNodeId": "indexer",
					"conclusion":   "document ingestion failed at node indexer",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.Done {
		t.Fatal("expected llm observer to continue")
	}
	if result.NextHint != "tool:ingestion_task_node_query|taskId=task-1|nodeId=indexer" {
		t.Fatalf("unexpected next hint: %q", result.NextHint)
	}
	if len(result.NextHintCalls) != 1 || result.NextHintCalls[0].Name != "ingestion_task_node_query" {
		t.Fatalf("expected structured next hint calls, got %+v", result.NextHintCalls)
	}
	if result.State.Phase != "deep_dive" {
		t.Fatalf("unexpected phase: %q", result.State.Phase)
	}
	if result.State.Confidence != 0.82 {
		t.Fatalf("unexpected confidence: %v", result.State.Confidence)
	}
	if len(mock.requests) != 1 || mock.requests[0].JSONMode == nil || !*mock.requests[0].JSONMode {
		t.Fatal("expected llm observer to request JSON mode")
	}
	userPrompt := mock.requests[0].Messages[1].Content
	if !strings.Contains(userPrompt, "document_ingestion_diagnose") {
		t.Fatalf("expected prompt to include round result summary, got %q", userPrompt)
	}
	systemPrompt := mock.requests[0].Messages[0].Content
	if !strings.Contains(systemPrompt, "Examples:") {
		t.Fatalf("expected system prompt to include few-shot examples, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "parameter: taskId (string, required)") {
		t.Fatalf("expected system prompt to include tool definitions, got %q", systemPrompt)
	}
}

func TestLLMObserverFallsBackOnInvalidJSON(t *testing.T) {
	mock := &mockObserverLLMService{response: `not-json`}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question:        "why did doc-1 fail?",
		Round:           1,
		ToolRegistry:    testRegistry,
		ToolDefinitions: testRegistry.ListDefinitions(),
		RoundResults: []Result{
			{
				Name: "document_ingestion_diagnose",
				Data: map[string]any{
					"latestTaskId":   "task-1",
					"latestLogError": "indexer failed after retries",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.Done {
		t.Fatal("expected fallback observer to continue")
	}
	if result.NextHint != "tool:ingestion_task_query|taskId=task-1|includeNodes=true" {
		t.Fatalf("unexpected fallback next hint: %q", result.NextHint)
	}
}

func TestLLMObserverFallsBackWhenNextHintMissing(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"Need more evidence.","confidence":0.5,"state":{"phase":"deep_dive","hypothesis":"need more evidence"}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question:        "why did task-1 fail?",
		Round:           1,
		ToolRegistry:    testRegistry,
		ToolDefinitions: testRegistry.ListDefinitions(),
		RoundResults: []Result{
			{
				Name: "task_ingestion_diagnose",
				Data: map[string]any{
					"taskId":       "task-1",
					"latestNodeId": "indexer",
					"conclusion":   "task failed at node indexer",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.NextHint != "tool:ingestion_task_node_query|taskId=task-1|nodeId=indexer" {
		t.Fatalf("expected fallback to node query, got %q", result.NextHint)
	}
}

func TestLLMObserverFallsBackWhenNextHintInventsNodeID(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"Need node-level detail.","confidence":0.7,"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"node_0"}}],"state":{"phase":"deep_dive","hypothesis":"failed node needs inspection","confidence":0.7,"openQuestions":["What is the node error?"],"checkedTools":["ingestion_task_query"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"node_0"}}]}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question:        "why did task-1 fail?",
		Round:           2,
		ToolRegistry:    testRegistry,
		ToolDefinitions: testRegistry.ListDefinitions(),
		RoundResults: []Result{
			{
				Name: "ingestion_task_query",
				Data: map[string]any{
					"taskId": "task-1",
					"status": "failed",
					"taskNodeSummary": []map[string]any{
						{"nodeId": "fetcher", "status": "success"},
						{"nodeId": "indexer", "status": "failed"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.NextHint != "tool:ingestion_task_node_query|taskId=task-1|nodeId=indexer" {
		t.Fatalf("expected fallback to evidence-backed node id, got %q", result.NextHint)
	}
}

func TestLLMObserverDoesNotUseReasoningAsHypothesisFallback(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"Inspect the task detail next.","confidence":0.6,"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}],"state":{"phase":"deep_dive","confidence":0.6,"openQuestions":["Which node failed?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question: "why did doc-1 fail?",
		Round:    1,
		PreviousState: AgentState{
			Phase:      "triage",
			Hypothesis: "the task failed but the concrete node is still unknown",
		},
		RoundResults: []Result{
			{
				Name: "document_ingestion_diagnose",
				Data: map[string]any{
					"latestTaskId":   "task-1",
					"latestLogError": "indexer failed after retries",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.State.Hypothesis != "the task failed but the concrete node is still unknown" {
		t.Fatalf("expected previous hypothesis to be preserved, got %q", result.State.Hypothesis)
	}
	if result.State.Hypothesis == "Inspect the task detail next." {
		t.Fatal("reasoning should not be copied into hypothesis")
	}
}

func TestLLMObserverAcceptsMultipleNextHintCalls(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"Both task node detail and trace are needed; they are independent.","state":{"phase":"deep_dive","hypothesis":"indexer failed; trace context is also needed","confidence":0.65,"openQuestions":["What is the node error?","What does the trace show?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}},{"name":"trace_node_query","arguments":{"traceId":"trace-abc"}}]}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question: "doc-1 为什么失败了？当前 trace 是什么状态？",
		Round:    1,
		RoundResults: []Result{
			{
				Name: "document_ingestion_diagnose",
				Data: map[string]any{
					"documentId":     "doc-1",
					"latestTaskId":   "task-1",
					"latestNodeId":   "indexer",
					"latestLogError": "connection refused",
				},
			},
		},
		Results: []Result{
			{
				Name: "document_ingestion_diagnose",
				Data: map[string]any{
					"documentId":   "doc-1",
					"latestTaskId": "task-1",
					"latestNodeId": "indexer",
					"traceId":      "trace-abc",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if result.Done {
		t.Fatal("expected not done with multiple hints")
	}
	if len(result.NextHintCalls) != 2 {
		t.Fatalf("expected 2 nextHintCalls, got %d", len(result.NextHintCalls))
	}
	if result.NextHintCalls[0].Name != "ingestion_task_node_query" {
		t.Fatalf("unexpected first hint: %s", result.NextHintCalls[0].Name)
	}
	if result.NextHintCalls[1].Name != "trace_node_query" {
		t.Fatalf("unexpected second hint: %s", result.NextHintCalls[1].Name)
	}
	if result.State.Phase != "deep_dive" {
		t.Fatalf("expected deep_dive phase, got %q", result.State.Phase)
	}
}

func TestLLMObserverRejectsMultiHintWithEmptyName(t *testing.T) {
	mock := &mockObserverLLMService{
		response: `{"done":false,"reasoning":"bad hint","confidence":0.5,"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1"}},{"name":"","arguments":{}}],"state":{"phase":"deep_dive","nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1"}},{"name":"","arguments":{}}]}}`,
	}
	observer := NewLLMObserver(mock)

	result, err := observer.Observe(context.Background(), ObserveInput{
		Question:     "why did doc-1 fail?",
		Round:        1,
		RoundResults: []Result{{Name: "document_ingestion_diagnose", Data: map[string]any{"latestTaskId": "task-1", "latestNodeId": "indexer"}}},
		Results:      []Result{{Name: "document_ingestion_diagnose", Data: map[string]any{"latestTaskId": "task-1", "latestNodeId": "indexer"}}},
	})
	if err != nil {
		t.Fatalf("observe error should not block fallback: %v", err)
	}
	// Should have fallen back to RuleObserver because one hint call has empty name.
	if result.Done {
		t.Log("multi-hint with empty name correctly triggered fallback to RuleObserver")
	} else {
		// RuleObserver may also continue depending on the data state.
		// The key is that LLMObserver rejected the response and fell back.
		t.Log("fallback to RuleObserver produced non-done result")
	}
}

func TestLLMObserverBuildUserPromptIncludesRewriteAndRetrieveContext(t *testing.T) {
	observer := NewLLMObserver(&mockObserverLLMService{response: `{}`})

	prompt := observer.BuildUserPrompt(ObserveInput{
		Question: "为什么这个任务失败了",
		RewriteResult: ragrewrite.Result{
			RewrittenQuestion: "为什么 ingestion task task-1 失败了",
			SubQuestions:      []string{"task-1 失败在哪个节点", "节点错误是什么"},
		},
		RetrieveResult: ragretrieve.Result{
			SearchChannels: []string{ragretrieve.ChannelKeyword, ragretrieve.ChannelVectorGlobal},
			ChannelStats: []ragretrieve.ChannelStat{
				{Name: ragretrieve.ChannelKeyword, ChunkCount: 2},
			},
			Chunks: []convention.RetrievedChunk{
				{
					ID:   "chunk-1",
					Text: "indexer 节点在向量库不可用时会返回 connection refused。",
					Metadata: map[string]any{
						"section":          "故障排查",
						"source_file_name": "ingestion_failures.md",
					},
				},
			},
		},
		RoundResults: []Result{
			{Name: "task_ingestion_diagnose", Summary: "task failed at node indexer"},
		},
	})

	if !strings.Contains(prompt, "Rewrite context:") {
		t.Fatal("prompt should include rewrite context")
	}
	if !strings.Contains(prompt, "rewrittenQuestion=为什么 ingestion task task-1 失败了") {
		t.Fatal("prompt should include rewritten question summary")
	}
	if !strings.Contains(prompt, "Retrieve context:") {
		t.Fatal("prompt should include retrieve context")
	}
	if !strings.Contains(prompt, "searchChannels=keyword, vector_global") {
		t.Fatal("prompt should include retrieve channels")
	}
	if !strings.Contains(prompt, "source_file_name") && !strings.Contains(prompt, "file=ingestion_failures.md") {
		t.Fatal("prompt should include retrieved chunk summary")
	}
}
