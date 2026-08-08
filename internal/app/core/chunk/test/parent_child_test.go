package chunk_test

import (
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestSplitParentChildKeepsParentReferenceAndGlobalChildIndex(t *testing.T) {
	result, err := chunk.SplitParentChild(
		"abcdefghijklmno",
		chunk.Options{Strategy: chunk.StrategyFixedSize, ChunkSize: 8},
		chunk.Options{Strategy: chunk.StrategyFixedSize, ChunkSize: 4},
	)
	if err != nil {
		t.Fatalf("split parent child: %v", err)
	}
	if len(result.Parents) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(result.Parents))
	}
	if len(result.Children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(result.Children))
	}

	for index, child := range result.Children {
		if child.Index != index {
			t.Fatalf("child %d has global index %d", index, child.Index)
		}
	}
	if result.Children[0].ParentIndex != 0 || result.Children[1].ParentIndex != 0 {
		t.Fatalf("expected first two children to reference first parent: %#v", result.Children[:2])
	}
	if result.Children[2].ParentIndex != 1 || result.Children[3].ParentIndex != 1 {
		t.Fatalf("expected last two children to reference second parent: %#v", result.Children[2:])
	}
}
