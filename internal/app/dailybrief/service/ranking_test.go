package service

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestRankingIsDeterministicForSameInput(t *testing.T) {
	base := []domain.Candidate{
		{
			Title:          "Older source",
			URL:            "https://example.com/older",
			Source:         domain.SourceKeyHackerNews,
			Topic:          domain.TopicKeyTechDev,
			PublishedAt:    time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC),
			SummarySnippet: "short",
		},
		{
			Title:          "Fresh arxiv",
			URL:            "https://example.com/arxiv",
			Source:         domain.SourceKeyArxivCSAI,
			Topic:          domain.TopicKeyTechAIResearch,
			PublishedAt:    time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
			SummarySnippet: "this is a more detailed summary that should add richness points",
		},
		{
			Title:          "Fresh but lower priority",
			URL:            "https://example.com/news",
			Source:         domain.SourceKeyTechCrunchAI,
			Topic:          domain.TopicKeyTechStartups,
			PublishedAt:    time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC),
			SummarySnippet: "medium summary",
		},
	}

	first := RankCandidates(base, []string{domain.TopicKeyTechAIResearch})
	second := RankCandidates(base, []string{domain.TopicKeyTechAIResearch})

	if len(first) != len(second) {
		t.Fatalf("expected same length, got %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].URL != second[i].URL {
			t.Fatalf("expected stable order at index %d, got %q and %q", i, first[i].URL, second[i].URL)
		}
	}
	if first[0].Source != domain.SourceKeyArxivCSAI {
		t.Fatalf("expected arxiv candidate to rank first, got %#v", first[0])
	}
}
