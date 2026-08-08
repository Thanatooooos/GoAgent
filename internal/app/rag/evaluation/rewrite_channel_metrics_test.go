package evaluation

import "testing"

func TestChannelMRRUplift(t *testing.T) {
	t.Parallel()
	uplift := channelMRRUplift(
		[]ChannelAggregateMetrics{
			{ChannelName: "keyword", MRR: 0.5, HitRateAtK: map[int]float64{1: 0.4, 3: 0.6}},
			{ChannelName: "vector_global", MRR: 0.8, HitRateAtK: map[int]float64{1: 0.7, 3: 0.9}},
		},
		[]ChannelAggregateMetrics{
			{ChannelName: "keyword", MRR: 0.6, HitRateAtK: map[int]float64{1: 0.5, 3: 0.7}},
			{ChannelName: "vector_global", MRR: 0.75, HitRateAtK: map[int]float64{1: 0.8, 3: 0.85}},
		},
	)
	if uplift["keyword"] != 0.1 {
		t.Fatalf("keyword mrr uplift = %v, want 0.1", uplift["keyword"])
	}
	if uplift["vector_global"] != -0.05 {
		t.Fatalf("vector_global mrr uplift = %v, want -0.05", uplift["vector_global"])
	}

	hitUplift := channelHitAtKUplift(
		[]ChannelAggregateMetrics{{ChannelName: "keyword", HitRateAtK: map[int]float64{1: 0.4, 3: 0.6}}},
		[]ChannelAggregateMetrics{{ChannelName: "keyword", HitRateAtK: map[int]float64{1: 0.5, 3: 0.7}}},
		[]int{1, 3},
	)
	if hitUplift["keyword"][1] != 0.1 || hitUplift["keyword"][3] != 0.1 {
		t.Fatalf("keyword hit uplift = %#v, want +0.1 for k=1 and k=3", hitUplift["keyword"])
	}
}

func TestAggregateChannelMetricsComputesMRR(t *testing.T) {
	t.Parallel()
	channels := aggregateChannelMetrics([]SampleResult{
		{
			Channels: []ChannelSampleResult{
				{ChannelName: "keyword", FirstRelevantRank: 2, HitAtK: map[int]bool{1: false, 3: true}},
			},
		},
		{
			Channels: []ChannelSampleResult{
				{ChannelName: "keyword", FirstRelevantRank: 1, HitAtK: map[int]bool{1: true, 3: true}},
			},
		},
	}, []int{1, 3})
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel aggregate, got %d", len(channels))
	}
	if channels[0].MRR != 0.75 {
		t.Fatalf("keyword channel MRR = %v, want 0.75", channels[0].MRR)
	}
}
