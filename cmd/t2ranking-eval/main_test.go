package main

import "testing"

func TestBuildSamplesMapsPositivePassagesToDocumentTargets(t *testing.T) {
	samples := buildSamples(
		[]query{{ID: "q1", Text: "question"}, {ID: "q2", Text: "uncovered"}},
		[]qrel{{QueryID: "q1", PassageID: "p1", Relevance: 2}, {QueryID: "q1", PassageID: "p2", Relevance: 3}, {QueryID: "q2", PassageID: "p3", Relevance: 1}},
		map[string]string{"p1": "doc-1", "p2": "doc-2"},
		"kb-1",
	)
	if len(samples) != 1 {
		t.Fatalf("sample count = %d, want 1", len(samples))
	}
	sample := samples[0]
	if sample.Target != "document" || len(sample.ExpectedIDs) != 2 || sample.ExpectedRelevance["doc-2"] != 3 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	if len(sample.KnowledgeBaseIDs) != 1 || sample.KnowledgeBaseIDs[0] != "kb-1" {
		t.Fatalf("knowledge base scope = %v", sample.KnowledgeBaseIDs)
	}
}
