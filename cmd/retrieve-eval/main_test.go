package main

import (
	"context"
	"path/filepath"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/convention"
)

type captureRetrieveService struct {
	requests []ragretrieve.Request
	result   ragretrieve.Result
}

func (s *captureRetrieveService) Retrieve(_ context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.requests = append(s.requests, request)
	result := s.result
	if len(result.ChannelRetrieved) == 0 && len(result.Chunks) > 0 {
		result.ChannelRetrieved = map[string][]convention.RetrievedChunk{
			ragretrieve.ChannelKeyword: append([]convention.RetrievedChunk(nil), result.Chunks...),
		}
	}
	return result, nil
}

func (s *captureRetrieveService) RetrieveByVector(_ context.Context, _ []float32, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.requests = append(s.requests, request)
	return s.result, nil
}

func TestLoadSamplesRetrieveEvalFixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "retrieve_eval_samples.json")
	samples, err := loadSamples(path)
	if err != nil {
		t.Fatalf("loadSamples returned error: %v", err)
	}
	if len(samples) < 20 {
		t.Fatalf("expected at least 20 retrieve eval samples, got %d", len(samples))
	}

	tagCounts := map[string]int{}
	requiredTags := []string{"alias", "diagnosis", "metadata", "coreference", "multi_condition", "keyword", "semantic"}
	for _, sample := range samples {
		for _, tag := range sample.Tags {
			tagCounts[tag]++
		}
		if len(sample.ExpectedIDs) == 0 {
			t.Fatalf("sample %q missing expectedIds", sample.Name)
		}
	}
	for _, tag := range requiredTags {
		minCount := 2
		if tag == "alias" || tag == "diagnosis" || tag == "metadata" {
			minCount = 4
		}
		if tagCounts[tag] < minCount {
			t.Fatalf("expected at least %d samples tagged %q, got %d", minCount, tag, tagCounts[tag])
		}
	}

	summary, err := rageval.Evaluate(samples, []int{1, 3, 5})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if summary.Overall.SampleCount != len(samples) {
		t.Fatalf("expected %d evaluated samples, got %d", len(samples), summary.Overall.SampleCount)
	}
	if summary.Overall.MRR <= 0 {
		t.Fatalf("expected positive MRR for offline fixture, got %v", summary.Overall.MRR)
	}
}

func TestLoadSamplesMemoryFactPhase3Fixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "memory_fact_phase3_samples.json")
	samples, err := loadSamples(path)
	if err != nil {
		t.Fatalf("loadSamples returned error: %v", err)
	}
	if len(samples) != 6 {
		t.Fatalf("expected 6 memory fact samples, got %d", len(samples))
	}
	if samples[0].UserID != "user-1" {
		t.Fatalf("expected first sample userID=user-1, got %q", samples[0].UserID)
	}

	summary, err := rageval.Evaluate(samples, []int{1, 3, 5})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if summary.Overall.SampleCount != 6 {
		t.Fatalf("expected 6 evaluated samples, got %d", summary.Overall.SampleCount)
	}
	if summary.Overall.MRR != 1 {
		t.Fatalf("expected perfect offline MRR for memory fact fixture, got %v", summary.Overall.MRR)
	}
}

func TestExecuteSamplesForwardsUserID(t *testing.T) {
	retrieve := &captureRetrieveService{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{
					ID:         "memory_fact:mem-kb-1",
					DocumentID: "mem-kb-1",
					Score:      0.91,
				},
			},
		},
	}
	runtime := &ragbootstrap.Runtime{Retrieve: retrieve}
	samples := []rageval.Sample{
		{
			Name:             "memory_fact_execute_forward_user",
			Query:            "Can this service access the public internet directly?",
			UserID:           "user-1",
			KnowledgeBaseIDs: []string{"kb-ops"},
			SearchMode:       "hybrid",
			TopK:             5,
		},
	}

	if err := executeSamples(context.Background(), runtime, samples, ""); err != nil {
		t.Fatalf("executeSamples returned error: %v", err)
	}
	if len(retrieve.requests) != 1 {
		t.Fatalf("expected exactly one retrieve request, got %d", len(retrieve.requests))
	}
	if retrieve.requests[0].UserID != "user-1" {
		t.Fatalf("expected userID to be forwarded, got %q", retrieve.requests[0].UserID)
	}
	if len(samples[0].Retrieved) != 1 || samples[0].Retrieved[0].ChunkID != "memory_fact:mem-kb-1" {
		t.Fatalf("expected executeSamples to backfill retrieved chunks, got %+v", samples[0].Retrieved)
	}
	if len(samples[0].ChannelRetrieved) != 1 {
		t.Fatalf("expected executeSamples to backfill channel retrieved, got %+v", samples[0].ChannelRetrieved)
	}
	if len(samples[0].ChannelRetrieved[ragretrieve.ChannelKeyword]) != 1 {
		t.Fatalf("expected keyword channel retrieved chunk, got %+v", samples[0].ChannelRetrieved)
	}
}
