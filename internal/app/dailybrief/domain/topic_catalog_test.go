package domain_test

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestTopicCatalogTreeExposesSelectableLeafTopics(t *testing.T) {
	tree := domain.TopicCatalogTree()
	if len(tree) == 0 {
		t.Fatal("expected non-empty topic catalog tree")
	}
	if !domain.IsTopicKeyKnown("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be known")
	}
	if !domain.IsLeafTopicKey("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be a leaf")
	}
	if !domain.IsTopicKeySelectable("tech.ai.models") {
		t.Fatal("expected tech.ai.models to be selectable")
	}
	if domain.IsTopicKeySelectable("tech") {
		t.Fatal("expected non-leaf tech to be non-selectable")
	}
}

func TestTopicBreadcrumbBuildsHumanReadableLabel(t *testing.T) {
	got := domain.TopicBreadcrumb("tech.ai.models")
	want := "科技 · AI · 模型发布"
	if got != want {
		t.Fatalf("unexpected breadcrumb: got %q want %q", got, want)
	}
}
