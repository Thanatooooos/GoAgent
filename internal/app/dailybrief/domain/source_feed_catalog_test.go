package domain_test

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestSourceKeysForTopicsExpandsAndSortsUniqueSources(t *testing.T) {
	got := domain.SourceKeysForTopics([]string{"tech.ai.models", "tech.ai.research"})
	want := []string{
		"anthropic-blog",
		"arxiv-cs-ai",
		"arxiv-cs-cl",
		"arxiv-cs-lg",
		"google-deepmind-blog",
		"meta-ai-blog",
		"openai-blog",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected source count: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected source at %d: got %q want %q (%v)", i, got[i], want[i], got)
		}
	}
	if !domain.TopicHasSources("tech.ai.models") {
		t.Fatal("expected tech.ai.models to report sources")
	}
	if !domain.TopicHasSources("art.design") {
		t.Fatal("expected art.design to report sources")
	}
}
