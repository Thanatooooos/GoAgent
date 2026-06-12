package retrieve

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestCollectChannelRetrievedSkipsFailedChannels(t *testing.T) {
	t.Parallel()
	retrieved := collectChannelRetrieved([]SearchChannelResult{
		{
			ChannelName: ChannelVectorGlobal,
			Chunks:      []convention.RetrievedChunk{{ID: "v1"}},
		},
		{
			ChannelName: ChannelKeyword,
			Error:       "keyword down",
			Chunks:      []convention.RetrievedChunk{{ID: "k1"}},
		},
	})
	if len(retrieved) != 1 {
		t.Fatalf("expected one channel retrieved map entry, got %+v", retrieved)
	}
	if len(retrieved[ChannelVectorGlobal]) != 1 || retrieved[ChannelVectorGlobal][0].ID != "v1" {
		t.Fatalf("unexpected vector channel chunks: %+v", retrieved[ChannelVectorGlobal])
	}
}

func TestMergeChannelRetrievedDedupesByChunkID(t *testing.T) {
	t.Parallel()
	merged := mergeChannelRetrieved([]Result{
		{
			ChannelRetrieved: map[string][]convention.RetrievedChunk{
				ChannelKeyword: {
					{ID: "c1", Score: 0.4},
					{ID: "c2", Score: 0.9},
				},
			},
		},
		{
			ChannelRetrieved: map[string][]convention.RetrievedChunk{
				ChannelKeyword: {
					{ID: "c1", Score: 0.8},
				},
			},
		},
	})
	chunks := merged[ChannelKeyword]
	if len(chunks) != 2 {
		t.Fatalf("expected 2 merged keyword chunks, got %+v", chunks)
	}
	if chunks[0].ID != "c2" || chunks[1].ID != "c1" || chunks[1].Score != 0.8 {
		t.Fatalf("unexpected merged order/scores: %+v", chunks)
	}
}
