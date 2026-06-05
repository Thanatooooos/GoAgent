package planner

import (
	"context"
	"testing"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type stubLLMService struct {
	response string
	err      error
	requests []convention.ChatRequest
}

func (s *stubLLMService) Chat(string) (string, error) {
	return s.response, s.err
}

func (s *stubLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, s.err
}

func (s *stubLLMService) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	return s.ChatWithRequest(request)
}

func (s *stubLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*stubLLMService)(nil)

func TestLLMPlannerPlanReturnsStructuredResult(t *testing.T) {
	llm := &stubLLMService{
		response: `{"decision":"continue","reason":"need_more_sources","confidence":0.71,"next_query":"refined query","preferred_urls":["https://example.com/b"],"avoid_urls":["https://example.com/a"],"answer_plan":{"use_evidence_ids":["fetch_1"]},"notes":["refine the query"]}`,
	}
	planner := NewLLMPlanner(llm)

	result, err := planner.Plan(context.Background(), PlanInput{
		Session:          newPlannerTestSession(),
		BaselineDecision: "continue",
		BaselineReason:   "need_more_sources",
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if result.Decision != "continue" || result.Reason != "need_more_sources" {
		t.Fatalf("unexpected plan result: %+v", result)
	}
	if result.NextQuery != "refined query" {
		t.Fatalf("expected refined next query, got %+v", result)
	}
	if len(result.PreferredURLs) != 1 || result.PreferredURLs[0] != "https://example.com/b" {
		t.Fatalf("expected preferred url guidance, got %+v", result)
	}
	if len(llm.requests) != 1 || llm.requests[0].JSONMode == nil || !*llm.requests[0].JSONMode {
		t.Fatalf("expected planner request in json mode, got %+v", llm.requests)
	}
}

func TestLLMPlannerPlanRejectsInventedPreferredURL(t *testing.T) {
	llm := &stubLLMService{
		response: `{"decision":"continue","reason":"need_more_sources","confidence":0.6,"next_query":"refined query","preferred_urls":["https://invented.example.com"],"avoid_urls":[],"answer_plan":{"use_evidence_ids":[]}}`,
	}
	planner := NewLLMPlanner(llm)

	_, err := planner.Plan(context.Background(), PlanInput{
		Session:          newPlannerTestSession(),
		BaselineDecision: "continue",
		BaselineReason:   "need_more_sources",
	})
	if err == nil {
		t.Fatal("expected validation error for invented preferred url")
	}
}

func newPlannerTestSession() *agentruntime.RuntimeSession {
	return &agentruntime.RuntimeSession{
		SessionID: "sess-planner",
		Request: agentruntime.RequestEnvelope{
			Question: "refine me",
			Options: agentstate.RuntimeOptions{
				MaxIterations:  3,
				AllowWebSearch: true,
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "refine me",
				RuntimeOptions: agentstate.RuntimeOptions{
					MaxIterations:  3,
					AllowWebSearch: true,
				},
			},
			Context: agentstate.ContextState{
				RewrittenQuery: "refine me",
				SearchQuery:    "refine me",
				SearchResults: []agentstate.SearchResultRef{
					{ID: "search_1", URL: "https://example.com/a", Title: "A", Snippet: "first"},
					{ID: "search_2", URL: "https://example.com/b", Title: "B", Snippet: "second"},
				},
				FetchResults: []agentstate.FetchResultRef{
					{ID: "fetch_1", URL: "https://example.com/a", Degraded: true, ErrorReason: "temporary failure"},
				},
				SeenURLs: []string{"https://example.com/a"},
			},
			Evidence: agentstate.EvidenceState{
				Items: []agentstate.EvidenceItem{
					{ID: "fetch_1", Source: "fetch", Content: "partial evidence", SourceRef: "https://example.com/a"},
				},
				Sufficient: false,
			},
			Execution: agentstate.ExecutionState{
				Iteration:            1,
				MaxIterations:        3,
				LastNewURLCount:      1,
				LastNewEvidenceCount: 0,
				LastProgressKind:     "progress_new_sources_found",
			},
		},
	}
}
