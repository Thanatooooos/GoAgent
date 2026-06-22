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

	chunks, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 2, Query: "test"}, channelResults)
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

	_, trace, err := engine.executeProcessors(context.Background(), SearchContext{TopK: 1, Query: "test"}, channelResults)
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
