package main

import (
	"testing"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

func TestGroupSectionsOrdersByChunkCount(t *testing.T) {
	sections := groupSections([]manifestChunk{
		{ChunkID: "c1", Index: 1, Metadata: map[string]any{"section": "B"}},
		{ChunkID: "c2", Index: 0, Metadata: map[string]any{"section": "A"}},
		{ChunkID: "c3", Index: 2, Metadata: map[string]any{"section": "A"}},
	})
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Name != "A" {
		t.Fatalf("expected section A first, got %q", sections[0].Name)
	}
	if sections[0].Chunks[0].ChunkID != "c2" || sections[0].Chunks[1].ChunkID != "c3" {
		t.Fatalf("expected section chunks ordered by index, got %+v", sections[0].Chunks)
	}
}

func TestGenerateSamplesIncludesFileAndSectionQueries(t *testing.T) {
	manifest := manifestFile{
		KnowledgeBase: markdownKnowledgeBaseRef{ID: "kb-1", Name: "kb"},
		ChunkStrategy: "markdown",
		Documents: []manifestDocument{
			{
				DocumentID:   "doc-1",
				DocumentName: "guide__intro.md",
				Chunks: []manifestChunk{
					{ChunkID: "c1", Index: 0, Content: "# Intro\nhello", Metadata: map[string]any{"section": "Intro"}},
					{ChunkID: "c2", Index: 1, Content: "world", Metadata: map[string]any{"section": "Intro"}},
				},
			},
		},
	}
	samples := generateSamples(manifest, 10, 2)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	if samples[0].Target != rageval.TargetSourceFileName {
		t.Fatalf("expected first sample to target source_file_name, got %s", samples[0].Target)
	}
	if samples[1].Target != rageval.TargetSection {
		t.Fatalf("expected second sample to target section, got %s", samples[1].Target)
	}
	if samples[2].Target != rageval.TargetChunk {
		t.Fatalf("expected third sample to target chunk, got %s", samples[2].Target)
	}
	if samples[2].ExpectedRelevance["c1"] != 3 {
		t.Fatalf("expected first section chunk relevance 3, got %+v", samples[2].ExpectedRelevance)
	}
}
