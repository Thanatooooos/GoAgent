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
