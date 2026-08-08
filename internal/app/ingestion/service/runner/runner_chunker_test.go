package runner

import (
	"context"
	"testing"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
)

func TestChunkerNodeRunnerParentChildModePreservesParentReferences(t *testing.T) {
	runner := NewChunkerNodeRunner(nil)
	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Content: "abcdefghijklmno"},
	}, ingestiondomain.PipelineNode{Settings: map[string]any{
		"strategy":          "fixed_size",
		"enableParentChild": true,
		"parentChunkSize":   8,
		"childChunkSize":    4,
	}})
	if err != nil {
		t.Fatalf("run chunker: %v", err)
	}
	if output["chunkMode"] != "parent_child" {
		t.Fatalf("expected parent_child mode, got %#v", output["chunkMode"])
	}
	if len(state.ParentChunks) != 2 {
		t.Fatalf("expected two parent chunks, got %d", len(state.ParentChunks))
	}
	if len(state.Chunks) != 4 {
		t.Fatalf("expected four child chunks, got %d", len(state.Chunks))
	}
	if state.Chunks[2].ParentIndex == nil || *state.Chunks[2].ParentIndex != 1 {
		t.Fatalf("expected child 2 to reference parent 1, got %#v", state.Chunks[2].ParentIndex)
	}
}

func TestChunkerNodeRunnerDefaultsToFlatMode(t *testing.T) {
	runner := NewChunkerNodeRunner(nil)
	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Content: "abcdef"},
	}, ingestiondomain.PipelineNode{Settings: map[string]any{"chunkSize": 3}})
	if err != nil {
		t.Fatalf("run chunker: %v", err)
	}
	if output["chunkMode"] != "flat" {
		t.Fatalf("expected flat mode, got %#v", output["chunkMode"])
	}
	if len(state.ParentChunks) != 0 {
		t.Fatalf("expected no parent chunks, got %#v", state.ParentChunks)
	}
}
