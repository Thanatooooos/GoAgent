package evaluation

import (
	"context"
	"sync"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

type captureRetrieveService struct {
	mu       sync.Mutex
	requests []ragretrieve.Request
	result   ragretrieve.Result
}

func (s *captureRetrieveService) Retrieve(_ context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	result := s.result
	if len(result.ChannelRetrieved) == 0 && len(result.Chunks) > 0 {
		result.ChannelRetrieved = map[string][]convention.RetrievedChunk{
			ragretrieve.ChannelKeyword: append([]convention.RetrievedChunk(nil), result.Chunks...),
		}
	}
	return result, nil
}

func (s *captureRetrieveService) RetrieveByVector(_ context.Context, _ []float32, request ragretrieve.Request) (ragretrieve.Result, error) {
	return s.Retrieve(context.Background(), request)
}

type rewriteServiceStub struct {
	result ragrewrite.Result
}

func (s rewriteServiceStub) Rewrite(question string) string {
	if strings := s.result.RewrittenQuestion; strings != "" {
		return strings
	}
	return question
}

func (s rewriteServiceStub) RewriteWithSplit(_ string) ragrewrite.Result {
	return s.result
}

func (s rewriteServiceStub) RewriteWithHistory(_ string, _ []convention.ChatMessage) ragrewrite.Result {
	return s.result
}

func TestExecuteSampleDirectRetrieve(t *testing.T) {
	retrieve := &captureRetrieveService{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{{ID: "chunk-1", DocumentID: "doc-1", Score: 0.9}},
		},
	}
	sample := Sample{
		Name:             "direct",
		Query:            "hello",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
		SearchMode:       "hybrid",
		TopK:             5,
	}
	if err := ExecuteSample(context.Background(), &sample, ExecuteConfig{
		Retrieve: retrieve,
	}); err != nil {
		t.Fatalf("ExecuteSample returned error: %v", err)
	}
	if len(retrieve.requests) != 1 {
		t.Fatalf("expected one retrieve request, got %d", len(retrieve.requests))
	}
	if retrieve.requests[0].UserID != "user-1" {
		t.Fatalf("expected userID forwarded, got %q", retrieve.requests[0].UserID)
	}
	if len(sample.Retrieved) != 1 || sample.Retrieved[0].ChunkID != "chunk-1" {
		t.Fatalf("unexpected retrieved: %+v", sample.Retrieved)
	}
}

func TestExecuteSampleWithRewriteUsesSubQuestions(t *testing.T) {
	retrieve := &captureRetrieveService{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{{ID: "chunk-1", DocumentID: "doc-1", Score: 0.9}},
		},
	}
	sample := Sample{
		Name:             "rewrite_parallel",
		Query:            "SSE 长连接特别多，Go 怎么撑住",
		KnowledgeBaseIDs: []string{"kb-1"},
		SearchMode:       "hybrid",
		TopK:             5,
	}
	if err := ExecuteSample(context.Background(), &sample, ExecuteConfig{
		Retrieve:   retrieve,
		Rewrite: rewriteServiceStub{
			result: ragrewrite.Result{
				RewrittenQuestion: "Go 调度模型和 IO 多路复用",
				SubQuestions:      []string{"Go GMP 调度模型", "Go netpoller IO 多路复用"},
				NeedRetrieval:     true,
			},
		},
		UseRewrite: true,
		SubQuestionOptions: ragretrieve.SubQuestionOptions{
			ParallelEnabled: true,
			MaxConcurrency:  2,
		},
	}); err != nil {
		t.Fatalf("ExecuteSample returned error: %v", err)
	}
	if len(retrieve.requests) != 3 {
		t.Fatalf("expected original plus two sub-question retrieve requests, got %d", len(retrieve.requests))
	}
	if retrieve.requests[0].Query != "SSE 长连接特别多，Go 怎么撑住" {
		t.Fatalf("expected original query first, got %q", retrieve.requests[0].Query)
	}
	if sample.ExecutionMode != ragretrieve.ExecutionModeParallel {
		t.Fatalf("expected parallel execution mode, got %q", sample.ExecutionMode)
	}
	if len(sample.SubQuestions) != 2 {
		t.Fatalf("expected subQuestions recorded, got %+v", sample.SubQuestions)
	}
}
