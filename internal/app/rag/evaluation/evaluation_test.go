package evaluation

import "testing"

func TestEvaluateComputesMetrics(t *testing.T) {
	summary, err := Evaluate([]Sample{
		{
			Name:        "chunk sample",
			Query:       "find chunk",
			Tags:        []string{"chunk", "lookup"},
			Target:      TargetChunk,
			ExpectedIDs: []string{"c2", "c4"},
			Retrieved: []RetrievedItem{
				{ChunkID: "c1"},
				{ChunkID: "c2"},
				{ChunkID: "c3"},
			},
		},
	}, []int{1, 3, 5})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if summary.Overall.SampleCount != 1 {
		t.Fatalf("expected sample count 1, got %d", summary.Overall.SampleCount)
	}
	if summary.Overall.HitRateAtK[1] != 0 {
		t.Fatalf("expected Hit@1=0, got %v", summary.Overall.HitRateAtK[1])
	}
	if summary.Overall.HitRateAtK[3] != 1 {
		t.Fatalf("expected Hit@3=1, got %v", summary.Overall.HitRateAtK[3])
	}
	if summary.Overall.AverageRecallAtK[3] != 0.5 {
		t.Fatalf("expected Recall@3=0.5, got %v", summary.Overall.AverageRecallAtK[3])
	}
	if summary.Overall.MRR != 0.5 {
		t.Fatalf("expected MRR=0.5, got %v", summary.Overall.MRR)
	}
	if len(summary.ByTag) != 2 {
		t.Fatalf("expected 2 tag summaries, got %d", len(summary.ByTag))
	}
}

func TestEvaluateUsesDocumentTarget(t *testing.T) {
	summary, err := Evaluate([]Sample{
		{
			Name:        "document sample",
			Query:       "find document",
			Target:      TargetDocument,
			ExpectedIDs: []string{"doc-2"},
			Retrieved: []RetrievedItem{
				{ChunkID: "c1", DocumentID: "doc-1"},
				{ChunkID: "c2", DocumentID: "doc-2"},
			},
		},
	}, []int{1, 2})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if summary.Overall.HitRateAtK[1] != 0 {
		t.Fatalf("expected document Hit@1=0, got %v", summary.Overall.HitRateAtK[1])
	}
	if summary.Overall.HitRateAtK[2] != 1 {
		t.Fatalf("expected document Hit@2=1, got %v", summary.Overall.HitRateAtK[2])
	}
	if summary.Samples[0].FirstRelevantRank != 2 {
		t.Fatalf("expected first relevant rank 2, got %d", summary.Samples[0].FirstRelevantRank)
	}
}

func TestEvaluateUsesMetadataTarget(t *testing.T) {
	summary, err := Evaluate([]Sample{
		{
			Name:        "metadata sample",
			Query:       "find by file name",
			Target:      TargetSourceFileName,
			ExpectedIDs: []string{"trace_handlers.go"},
			Retrieved: []RetrievedItem{
				{ChunkID: "c1", Metadata: map[string]any{"source_file_name": "other.go"}},
				{ChunkID: "c2", Metadata: map[string]any{"source_file_name": "trace_handlers.go"}},
			},
		},
	}, []int{1, 2})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if summary.Overall.HitRateAtK[1] != 0 {
		t.Fatalf("expected metadata Hit@1=0, got %v", summary.Overall.HitRateAtK[1])
	}
	if summary.Overall.HitRateAtK[2] != 1 {
		t.Fatalf("expected metadata Hit@2=1, got %v", summary.Overall.HitRateAtK[2])
	}
	if summary.Samples[0].FirstRelevantRank != 2 {
		t.Fatalf("expected metadata first relevant rank 2, got %d", summary.Samples[0].FirstRelevantRank)
	}
}

func TestEvaluateRejectsInvalidSample(t *testing.T) {
	_, err := Evaluate([]Sample{{Name: "bad", Target: "invalid"}}, []int{1})
	if err == nil {
		t.Fatal("expected invalid sample error")
	}
}

func TestEvaluateNDCG(t *testing.T) {
	// binary relevance, c2 at rank 2
	summary, err := Evaluate([]Sample{
		{
			Name:        "ndcg sample",
			Query:       "find chunk",
			Target:      TargetChunk,
			ExpectedIDs: []string{"c2", "c4"},
			Retrieved: []RetrievedItem{
				{ChunkID: "c1"},
				{ChunkID: "c2"},
				{ChunkID: "c3"},
			},
		},
	}, []int{1, 3})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	ndcg1 := summary.Overall.AverageNDCGAtK[1]
	ndcg3 := summary.Overall.AverageNDCGAtK[3]
	if ndcg1 != 0 {
		t.Fatalf("expected NDCG@1=0 (first item not relevant), got %.4f", ndcg1)
	}
	// DCG@3 = 1/log2(3) ≈ 0.631, IDCG@3 = 1/log2(2) + 1/log2(3) ≈ 1.631 → NDCG@3 ≈ 0.387
	if ndcg3 < 0.38 || ndcg3 > 0.39 {
		t.Fatalf("expected NDCG@3 ≈ 0.387, got %.4f", ndcg3)
	}
}

func TestEvaluateNDCGWithGradedRelevance(t *testing.T) {
	summary, err := Evaluate([]Sample{
		{
			Name:        "graded sample",
			Query:       "find chunks",
			Target:      TargetChunk,
			ExpectedIDs: []string{"perf", "best", "good"},
			ExpectedRelevance: map[string]int{
				"perf": 3,
				"best": 2,
				"good": 1,
			},
			Retrieved: []RetrievedItem{
				{ChunkID: "perf"},   // rank 1, grade 3
				{ChunkID: "bad"},    // rank 2, grade 0
				{ChunkID: "good"},   // rank 3, grade 1
				{ChunkID: "best"},   // rank 4, grade 2
			},
		},
	}, []int{3})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	ndcg3 := summary.Overall.AverageNDCGAtK[3]
	// DCG@3: (2^3-1)/log2(2)=7, 0, (2^1-1)/log2(4)=1/2=0.5 → 7.5
	// IDCG@3: ideal=[3,2,1] → 7/log2(2)=7, 3/log2(3)=3/1.585=1.893, 1/log2(4)=0.5 → 9.393
	// NDCG@3 ≈ 0.798
	if ndcg3 < 0.79 || ndcg3 > 0.80 {
		t.Fatalf("expected NDCG@3 ≈ 0.798, got %.4f", ndcg3)
	}
}

func TestEvaluatePreservesChunkStrategyTag(t *testing.T) {
	summary, err := Evaluate([]Sample{
		{
			Name:          "fixed sample",
			Query:         "find chunk",
			Target:        TargetChunk,
			Tags:          []string{"fixed_size"},
			ExpectedIDs:   []string{"c1"},
			ChunkStrategy: "fixed_size",
			Retrieved: []RetrievedItem{
				{ChunkID: "c1"},
			},
		},
		{
			Name:          "markdown sample",
			Query:         "find chunk",
			Target:        TargetChunk,
			Tags:          []string{"markdown"},
			ExpectedIDs:   []string{"c1"},
			ChunkStrategy: "markdown",
			Retrieved: []RetrievedItem{
				{ChunkID: "c2"},
				{ChunkID: "c1"},
			},
		},
	}, []int{1, 3})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if summary.ByTag[0].Metrics.SampleCount != 1 {
		t.Fatalf("expected 1 sample per tag, got %d and %d", summary.ByTag[0].Metrics.SampleCount, summary.ByTag[1].Metrics.SampleCount)
	}
}
