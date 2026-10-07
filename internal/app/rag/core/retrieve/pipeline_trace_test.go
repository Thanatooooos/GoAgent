package retrieve

import (
	"context"
	"errors"
	"testing"

	"local/rag-project/internal/framework/convention"
)

type stubReranker struct {
	applied bool
	err     error
}

func (s *stubReranker) Rerank(_ string, candidates []convention.RetrievedChunk, _ int) ([]convention.RetrievedChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.applied || len(candidates) < 2 {
		return candidates, nil
	}
	reordered := append([]convention.RetrievedChunk(nil), candidates[1], candidates[0])
	reordered = append(reordered, candidates[2:]...)
	return reordered, nil
}

func TestExecuteProcessorsRecordsPipelineTrace(t *testing.T) {
	engine := &Engine{
		processors: []SearchResultPostProcessor{
			NewFusionPostProcessor(),
			NewDedupPostProcessor(),
			NewRerankPostProcessor(&stubReranker{applied: true}),
		},
	}
	channelResults := []SearchChannelResult{
		{
			ChannelName: ChannelKeyword,
			Chunks: []convention.RetrievedChunk{
				{ID: "a", Score: 1},
				{ID: "b", Score: 0.9},
			},
			Metadata: map[string]any{"rrfWeight": float32(0.85)},
		},
		{
			ChannelName: ChannelVectorGlobal,
			Chunks: []convention.RetrievedChunk{
				{ID: "b", Score: 1},
				{ID: "c", Score: 0.8},
			},
			Metadata: map[string]any{"rrfWeight": float32(1.0)},
		},
	}

	chunks, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 2, RerankTopN: 2, Query: "test"}, channelResults)
	if err != nil {
		t.Fatalf("executeProcessors() error = %v", err)
	}
	if trace == nil {
		t.Fatal("expected pipeline trace")
	}
	if len(trace.PreRerankChunkIDs) != 2 {
		t.Fatalf("pre rerank ids = %v, want 2 entries", trace.PreRerankChunkIDs)
	}
	if !trace.RerankApplied {
		t.Fatal("expected rerank applied")
	}
	if len(chunks) != 2 || chunks[0].ID == trace.PreRerankChunkIDs[0] {
		t.Fatalf("expected rerank to reorder, pre=%v final=%v", trace.PreRerankChunkIDs, chunkIDs(chunks))
	}
}

func TestRerankFailureLeavesPreRerankOrder(t *testing.T) {
	engine := &Engine{
		processors: []SearchResultPostProcessor{
			NewFusionPostProcessor(),
			NewDedupPostProcessor(),
			NewRerankPostProcessor(&stubReranker{err: errors.New("rerank down")}),
		},
	}
	channelResults := []SearchChannelResult{
		{
			ChannelName: ChannelKeyword,
			Chunks:      []convention.RetrievedChunk{{ID: "a", Score: 1}},
			Metadata:    map[string]any{"rrfWeight": float32(0.85)},
		},
	}

	_, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 1, RerankTopN: 1, Query: "test"}, channelResults)
	if err != nil {
		t.Fatalf("executeProcessors() error = %v", err)
	}
	if trace.RerankApplied {
		t.Fatal("expected rerank not applied on error")
	}
	if trace.RerankError == "" {
		t.Fatal("expected rerank error recorded")
	}
}

func TestRerankProcessorSkipsWhenRerankTopNZero(t *testing.T) {
	engine := &Engine{
		processors: []SearchResultPostProcessor{
			NewFusionPostProcessor(),
			NewDedupPostProcessor(),
			NewRerankPostProcessor(&stubReranker{applied: true}),
		},
	}
	channelResults := []SearchChannelResult{
		{
			ChannelName: ChannelVectorGlobal,
			Chunks:      []convention.RetrievedChunk{{ID: "a", Score: 1}, {ID: "b", Score: 0.9}},
			Metadata:    map[string]any{"rrfWeight": float32(1.0)},
		},
	}

	chunks, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 2, RerankTopN: 0, Query: "test"}, channelResults)
	if err != nil {
		t.Fatalf("executeProcessors() error = %v", err)
	}
	if trace.RerankApplied {
		t.Fatal("expected rerank skipped when RerankTopN is zero")
	}
	if len(chunks) != 2 {
		t.Fatalf("expected unchanged chunks, got %d", len(chunks))
	}
}

func TestRerankProcessorReranksWhenTopNEqualsCandidateCount(t *testing.T) {
	engine := &Engine{
		processors: []SearchResultPostProcessor{
			NewFusionPostProcessor(),
			NewDedupPostProcessor(),
			NewRerankPostProcessor(&stubReranker{applied: true}),
		},
	}
	channelResults := []SearchChannelResult{
		{
			ChannelName: ChannelVectorGlobal,
			Chunks:      []convention.RetrievedChunk{{ID: "a", Score: 1}, {ID: "b", Score: 0.9}},
			Metadata:    map[string]any{"rrfWeight": float32(1.0)},
		},
	}

	chunks, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 2, RerankTopN: 2, Query: "test"}, channelResults)
	if err != nil {
		t.Fatalf("executeProcessors() error = %v", err)
	}
	if !trace.RerankApplied {
		t.Fatal("expected rerank applied when RerankTopN equals candidate count")
	}
	if len(chunks) != 2 || chunks[0].ID != "b" {
		t.Fatalf("expected rerank to reorder chunks, got %+v", chunkIDs(chunks))
	}
}

func TestStagedBudgetsKeepCandidatesAndLimitFinalResults(t *testing.T) {
	request := Request{Query: "test", TopK: 3, RecallBudget: 4, CandidateLimit: 6}
	searchCtx := buildSearchContext(request)
	if searchCtx.RerankTopN != 3 {
		t.Fatalf("rerank top N = %d, want 3", searchCtx.RerankTopN)
	}
	engine := &Engine{processors: []SearchResultPostProcessor{
		NewFusionPostProcessor(),
		NewDedupPostProcessor(),
		NewRerankPostProcessor(&stubReranker{applied: true}),
	}}
	channelResults := []SearchChannelResult{{ChannelName: ChannelKeyword, Chunks: []convention.RetrievedChunk{
		{ID: "a", Score: 8}, {ID: "b", Score: 7}, {ID: "c", Score: 6},
		{ID: "d", Score: 5}, {ID: "e", Score: 4}, {ID: "f", Score: 3}, {ID: "g", Score: 2},
	}}}
	chunks, trace, err := engine.executeProcessors(context.Background(), searchCtx, channelResults)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.PreRerankChunkIDs) != 6 || len(chunks) != 3 || chunks[0].ID != "b" {
		t.Fatalf("pre=%v final=%v", trace.PreRerankChunkIDs, chunkIDs(chunks))
	}
	if trace.RecallBudget != 4 || trace.CandidateLimit != 6 || trace.ContextTopK != 3 {
		t.Fatalf("unexpected budgets: %+v", trace)
	}
}

func TestStagedBudgetsFallBackToFusionOrderOnRerankFailure(t *testing.T) {
	searchCtx := buildSearchContext(Request{Query: "test", TopK: 2, CandidateLimit: 3})
	engine := &Engine{processors: []SearchResultPostProcessor{
		NewFusionPostProcessor(),
		NewDedupPostProcessor(),
		NewRerankPostProcessor(&stubReranker{err: errors.New("rerank down")}),
	}}
	results := []SearchChannelResult{{ChannelName: ChannelKeyword, Chunks: []convention.RetrievedChunk{
		{ID: "a", Score: 3}, {ID: "b", Score: 2}, {ID: "c", Score: 1},
	}}}
	chunks, trace, err := engine.executeProcessors(context.Background(), searchCtx, results)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.PreRerankChunkIDs) != 3 || len(chunks) != 2 || chunks[0].ID != "a" || chunks[1].ID != "b" {
		t.Fatalf("pre=%v final=%v", trace.PreRerankChunkIDs, chunkIDs(chunks))
	}
	if trace.RerankApplied || trace.RerankError == "" {
		t.Fatalf("expected recorded rerank failure: %+v", trace)
	}
}
