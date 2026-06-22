package retrieve

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"local/rag-project/internal/framework/convention"
)

type countingRetrieveService struct {
	requests atomic.Int32
	result   Result
}

func (s *countingRetrieveService) Retrieve(_ context.Context, request Request) (Result, error) {
	s.requests.Add(1)
	time.Sleep(20 * time.Millisecond)
	result := s.result
	result.Chunks = []convention.RetrievedChunk{{
		ID:    request.Query,
		Score: 1,
	}}
	return result, nil
}

func (s *countingRetrieveService) RetrieveByVector(context.Context, []float32, Request) (Result, error) {
	return Result{}, nil
}

func TestSubQuestionExecutorParallelRunsConcurrently(t *testing.T) {
	retrieve := &countingRetrieveService{}
	executor := NewSubQuestionExecutor(retrieve, SubQuestionOptions{
		ParallelEnabled: true,
		MaxConcurrency:  2,
	})

	startedAt := time.Now()
	_, mode, subResults, err := executor.RetrieveMerged(context.Background(), Request{
		Query:            "original",
		KnowledgeBaseIDs: []string{"kb-1"},
		SearchMode:       SearchModeHybrid,
		TopK:             5,
	}, []string{"question one", "question two"}, 5)
	elapsed := time.Since(startedAt)

	if err != nil {
		t.Fatalf("RetrieveMerged returned error: %v", err)
	}
	if mode != ExecutionModeParallel {
		t.Fatalf("expected parallel mode, got %q", mode)
	}
	if len(subResults) != 2 {
		t.Fatalf("expected two sub results, got %d", len(subResults))
	}
	if retrieve.requests.Load() != 2 {
		t.Fatalf("expected two retrieve calls, got %d", retrieve.requests.Load())
	}
	if elapsed >= 35*time.Millisecond {
		t.Fatalf("expected parallel execution under 35ms, got %v", elapsed)
	}
}

func TestSubQuestionExecutorSerializesDependencyRisk(t *testing.T) {
	executor := NewSubQuestionExecutor(&countingRetrieveService{}, SubQuestionOptions{
		ParallelEnabled: true,
		MaxConcurrency:  2,
	})
	mode := executor.determineExecutionMode([]string{"first question", "这个节点错误是什么"})
	if mode != ExecutionModeSerialDependencyRisk {
		t.Fatalf("expected serial dependency risk mode, got %q", mode)
	}
}
