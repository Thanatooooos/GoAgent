package retrieve

import (
	"context"
	"fmt"
	"testing"

	"local/rag-project/internal/framework/convention"
)

// makeChunkIDs creates N unique chunk IDs with scores for a simulated channel.
func makeChunkIDs(prefix string, n int) []convention.RetrievedChunk {
	chunks := make([]convention.RetrievedChunk, n)
	for i := 0; i < n; i++ {
		chunks[i] = convention.RetrievedChunk{
			ID:    fmt.Sprintf("%s_%02d", prefix, i),
			Score: float32(n - i),
		}
	}
	return chunks
}

// overlapChunks creates chunks where some overlap between channels.
func overlapChunks(prefix string, n int, overlapWith func(int) string) []convention.RetrievedChunk {
	chunks := make([]convention.RetrievedChunk, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s_%02d", prefix, i)
		if i < 3 && overlapWith != nil {
			id = overlapWith(i)
		}
		chunks[i] = convention.RetrievedChunk{
			ID:    id,
			Score: float32(n - i),
		}
	}
	return chunks
}

// countingReranker records how many candidates it received.
type countingReranker struct {
	candidateCount int
}

func (r *countingReranker) Rerank(_ string, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error) {
	r.candidateCount = len(candidates)
	// Return topN (identity, no reorder)
	if topN > 0 && topN < len(candidates) {
		return candidates[:topN], nil
	}
	return candidates, nil
}

func TestABCompare_RerankInput_TopK5_vs_TopK20(t *testing.T) {
	// Simulate a realistic multi-channel scenario:
	// - vector_global returns 10 chunks (when TopK=5, multiplier=2)
	// - keyword returns 10 chunks
	// - metadata_title returns 10 chunks
	// - 3-4 chunks overlap between channels (typical)
	//
	// After RRF fusion + dedup, we expect ~15-25 unique chunks.
	// With TopK=5, dedup truncates to 5 before rerank.
	// With TopK=20, dedup keeps 20, rerank has real work to do.

	type testCase struct {
		name       string
		topK       int
		rerankTopN int
	}

	cases := []testCase{
		{name: "TopK=5 (current default)", topK: 5, rerankTopN: 0},
		{name: "TopK=20 RerankTopN=5 (proposed)", topK: 20, rerankTopN: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build channels: each retrieves topK * 2 results
			perChannel := tc.topK * 2
			reranker := &countingReranker{}

			engine := &Engine{
				processors: []SearchResultPostProcessor{
					NewFusionPostProcessor(),
					NewDedupPostProcessor(),
					NewRerankPostProcessor(reranker),
				},
			}

			// Simulate 3 channels with realistic overlap (~3 shared chunks)
			vectorChunks := makeChunkIDs("vec", perChannel)
			keywordChunks := make([]convention.RetrievedChunk, perChannel)
			for i := 0; i < perChannel; i++ {
				if i < 3 {
					keywordChunks[i] = convention.RetrievedChunk{
						ID:    fmt.Sprintf("vec_%02d", i),
						Score: float32(perChannel - i),
					}
				} else {
					keywordChunks[i] = convention.RetrievedChunk{
						ID:    fmt.Sprintf("kw_%02d", i),
						Score: float32(perChannel - i),
					}
				}
			}
			titleChunks := make([]convention.RetrievedChunk, perChannel)
			for i := 0; i < perChannel; i++ {
				if i < 2 {
					titleChunks[i] = convention.RetrievedChunk{
						ID:    fmt.Sprintf("kw_%02d", i+3),
						Score: float32(perChannel - i),
					}
				} else {
					titleChunks[i] = convention.RetrievedChunk{
						ID:    fmt.Sprintf("title_%02d", i),
						Score: float32(perChannel - i),
					}
				}
			}

			channelResults := []SearchChannelResult{
				{ChannelName: ChannelVectorGlobal, Chunks: vectorChunks, Metadata: map[string]any{"rrfWeight": float32(1.0)}},
				{ChannelName: ChannelKeyword, Chunks: keywordChunks, Metadata: map[string]any{"rrfWeight": float32(0.85)}},
				{ChannelName: ChannelMetadataTitle, Chunks: titleChunks, Metadata: map[string]any{"rrfWeight": float32(0.8)}},
			}

			searchCtx := SearchContext{
				TopK:       tc.topK,
				RerankTopN: tc.rerankTopN,
				Query:      "test query",
			}

			totalRaw := 0
			for _, cr := range channelResults {
				totalRaw += len(cr.Chunks)
			}

			chunks, trace, err := engine.executeProcessors(context.Background(), searchCtx, channelResults)
			if err != nil {
				t.Fatalf("executeProcessors() error = %v", err)
			}

			preRerankCount := len(trace.PreRerankChunkIDs)
			rerankInputCount := reranker.candidateCount
			finalCount := len(chunks)

			t.Logf("Raw channel total: %d chunks", totalRaw)
			t.Logf("Pre-rerank (after dedup): %d chunks", preRerankCount)
			t.Logf("Rerank input count: %d chunks", rerankInputCount)
			t.Logf("Final output: %d chunks", finalCount)
			t.Logf("Rerank applied: %v", trace.RerankApplied)
			t.Logf("PreRerankIDs: %v", trace.PreRerankChunkIDs)
			t.Logf("FinalIDs:    %v", trace.FinalChunkIDs)
		})
	}
}
