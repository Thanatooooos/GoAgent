package executor

import (
	"testing"

	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
)

func TestEinoTaskRuntimeCommitCarriesParentChunks(t *testing.T) {
	runtime := newEinoTaskRuntime(ingestionworkflow.ExecutionState{})
	next := ingestionworkflow.ExecutionState{
		ParentChunks: []ingestionworkflow.ParentChunkPayload{
			{Index: 0, Content: "parent-0"},
			{Index: 1, Content: "parent-1"},
		},
	}
	runtime.Commit(ingestionworkflow.WorkflowNodeSpec{}, next, nil)
	snapshot := runtime.Snapshot()
	if len(snapshot.ParentChunks) != 2 || snapshot.ParentChunks[1].Content != "parent-1" {
		t.Fatalf("expected parent chunks committed, got %+v", snapshot.ParentChunks)
	}
}
