package chunk

import "testing"

func TestParentChildOptionsDefaults(t *testing.T) {
	parent, child := ParentChildOptions(StrategyFixedSize, 0, 0, 0, 0)
	if parent.ChunkSize != 800 || parent.OverlapSize != 120 {
		t.Fatalf("parent defaults = %+v", parent)
	}
	if child.ChunkSize != 300 || child.OverlapSize != 60 {
		t.Fatalf("child defaults = %+v", child)
	}
	if parent.Strategy != StrategyFixedSize || child.Strategy != StrategyFixedSize {
		t.Fatalf("strategy not propagated: parent=%+v child=%+v", parent, child)
	}
}

func TestParentChildOptionsExplicitOverrides(t *testing.T) {
	parent, child := ParentChildOptions(StrategyMarkdown, 1200, 200, 400, 80)
	if parent.ChunkSize != 1200 || parent.OverlapSize != 200 {
		t.Fatalf("parent explicit = %+v", parent)
	}
	if child.ChunkSize != 400 || child.OverlapSize != 80 {
		t.Fatalf("child explicit = %+v", child)
	}
	if parent.Strategy != StrategyMarkdown || child.Strategy != StrategyMarkdown {
		t.Fatalf("strategy not propagated: parent=%+v child=%+v", parent, child)
	}
}
