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

func TestChunkerNodeRunnerDefaultsToParentChild(t *testing.T) {
	runner := NewChunkerNodeRunner(nil)
	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Content: "abcdefghijklmno"},
	}, ingestiondomain.PipelineNode{Settings: map[string]any{}})
	if err != nil {
		t.Fatalf("run chunker: %v", err)
	}
	if output["chunkMode"] != "parent_child" {
		t.Fatalf("expected parent_child default mode, got %#v", output["chunkMode"])
	}
	if len(state.ParentChunks) == 0 {
		t.Fatalf("expected parent chunks in default mode, got %#v", state.ParentChunks)
	}
	if len(state.Chunks) == 0 {
		t.Fatalf("expected child chunks in default mode")
	}
	if state.Chunks[0].ParentIndex == nil || *state.Chunks[0].ParentIndex != 0 {
		t.Fatalf("expected child to reference parent 0, got %#v", state.Chunks[0].ParentIndex)
	}
}

func TestChunkerNodeRunnerExplicitFlatOptOut(t *testing.T) {
	runner := NewChunkerNodeRunner(nil)
	state, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Content: "abcdef"},
	}, ingestiondomain.PipelineNode{Settings: map[string]any{
		"chunkSize":         3,
		"enableParentChild": false,
	}})
	if err != nil {
		t.Fatalf("run chunker: %v", err)
	}
	if output["chunkMode"] != "flat" {
		t.Fatalf("expected flat mode when explicitly disabled, got %#v", output["chunkMode"])
	}
	if len(state.ParentChunks) != 0 {
		t.Fatalf("expected no parent chunks, got %#v", state.ParentChunks)
	}
}
