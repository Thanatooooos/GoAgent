package evaluation

import "testing"

func TestEvaluateChannelMetricsComputesHitOverlapAndUnique(t *testing.T) {
	t.Parallel()
	summary, err := Evaluate([]Sample{
		{
			Name:        "hybrid overlap",
			Query:       "pg timeout",
			Target:      TargetChunk,
			ExpectedIDs: []string{"chunk-pg-conn-1"},
			Retrieved: []RetrievedItem{
				{ChunkID: "chunk-pg-conn-1"},
			},
			ChannelRetrieved: map[string][]RetrievedItem{
				"vector_global": {
					{ChunkID: "chunk-noise-1"},
					{ChunkID: "chunk-pg-conn-1"},
				},
				"keyword": {
					{ChunkID: "chunk-pg-conn-1"},
				},
			},
		},
		{
			Name:        "keyword unique",
			Query:       "es health",
			Target:      TargetChunk,
			ExpectedIDs: []string{"chunk-es-health-1"},
			Retrieved: []RetrievedItem{
				{ChunkID: "chunk-es-health-1"},
			},
			ChannelRetrieved: map[string][]RetrievedItem{
				"vector_global": {
					{ChunkID: "chunk-noise-2"},
				},
				"keyword": {
					{ChunkID: "chunk-es-health-1"},
				},
			},
		},
	}, []int{1, 3})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(summary.Channels) != 2 {
		t.Fatalf("expected 2 channel aggregates, got %d", len(summary.Channels))
	}

	channelByName := map[string]ChannelAggregateMetrics{}
	for _, channel := range summary.Channels {
		channelByName[channel.ChannelName] = channel
	}
	vector := channelByName["vector_global"]
	keyword := channelByName["keyword"]
	if vector.HitRateAtK[3] != 0.5 {
		t.Fatalf("expected vector channel_hit@3=0.5, got %v", vector.HitRateAtK[3])
	}
	if keyword.HitRateAtK[1] != 1 {
		t.Fatalf("expected keyword channel_hit@1=1, got %v", keyword.HitRateAtK[1])
	}
	if vector.UniqueHitCount != 0 || vector.OverlapHitCount != 1 {
		t.Fatalf("expected vector overlap only, got unique=%d overlap=%d", vector.UniqueHitCount, vector.OverlapHitCount)
	}
	if keyword.UniqueHitCount != 1 || keyword.OverlapHitCount != 1 {
		t.Fatalf("expected keyword unique+overlap mix, got unique=%d overlap=%d", keyword.UniqueHitCount, keyword.OverlapHitCount)
	}

	if len(summary.Samples[0].Channels) != 2 {
		t.Fatalf("expected per-sample channel metrics, got %+v", summary.Samples[0].Channels)
	}
	if summary.Samples[0].Channels[0].FirstRelevantRank == 0 && summary.Samples[0].Channels[1].FirstRelevantRank == 0 {
		t.Fatal("expected at least one channel firstRelevantRank")
	}
}

func TestEvaluateChannelMetricsSkippedWithoutChannelRetrieved(t *testing.T) {
	t.Parallel()
	summary, err := Evaluate([]Sample{
		{
			Name:        "offline only",
			Query:       "test",
			Target:      TargetChunk,
			ExpectedIDs: []string{"c1"},
			Retrieved:   []RetrievedItem{{ChunkID: "c1"}},
		},
	}, []int{1})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(summary.Channels) != 0 {
		t.Fatalf("expected no channel metrics without channelRetrieved, got %+v", summary.Channels)
	}
}
