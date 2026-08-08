package service

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/framework/config"
)

func TestCandidatePipelineProducesStableRankedOutput(t *testing.T) {
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 2})
	input := []domain.Candidate{
		{
			Title:          "  Fresh arxiv  ",
			URL:            "https://example.com/arxiv?utm_source=x",
			Source:         domain.SourceKeyArxivCSAI,
			Topic:          domain.TopicKeyTechAIResearch,
			PublishedAt:    time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC),
			ExternalID:     "arxiv-1",
			SummarySnippet: "  long summary text for ranking richness points  ",
		},
		{
			Title:          "Fresh arxiv duplicate",
			URL:            "https://example.com/arxiv",
			Source:         domain.SourceKeyArxivCSAI,
			Topic:          domain.TopicKeyTechAIResearch,
			PublishedAt:    time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
			ExternalID:     "arxiv-1",
			SummarySnippet: "better summary",
		},
		{
			Title:          "Hacker update",
			URL:            "https://example.com/hn",
			Source:         domain.SourceKeyHackerNews,
			Topic:          domain.TopicKeyTechDev,
			PublishedAt:    time.Date(2026, 6, 29, 7, 30, 0, 0, time.UTC),
			SummarySnippet: "hn summary",
		},
		{
			Title:          "Ignore unknown topic",
			URL:            "https://example.com/other",
			Source:         domain.SourceKeyTechCrunchAI,
			Topic:          domain.TopicKeyTechStartups,
			PublishedAt:    time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC),
			SummarySnippet: "industry news",
		},
	}

	first := pipeline.Process(input, []string{domain.TopicKeyTechAIResearch, domain.TopicKeyTechDev})
	second := pipeline.Process(input, []string{domain.TopicKeyTechAIResearch, domain.TopicKeyTechDev})

	if len(first) != 2 {
		t.Fatalf("expected pipeline to respect max candidates and return 2, got %d", len(first))
	}
	if len(first) != len(second) {
		t.Fatalf("expected deterministic output size, got %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].URL != second[i].URL {
			t.Fatalf("expected deterministic order at index %d, got %q and %q", i, first[i].URL, second[i].URL)
		}
	}
	if first[0].ExternalID != "arxiv-1" {
		t.Fatalf("expected deduplicated arxiv candidate first, got %#v", first[0])
	}
}

func TestCandidatePipelineLimitsPerTopic(t *testing.T) {
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{
		MaxCandidates:    6,
		MaxItemsPerTopic: 2,
	})
	input := []domain.Candidate{
		{Title: "s1", URL: "https://example.com/s1", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
		{Title: "s2", URL: "https://example.com/s2", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
		{Title: "s3", URL: "https://example.com/s3", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
		{Title: "d1", URL: "https://example.com/d1", Source: domain.SourceKeyHackerNews, Topic: domain.TopicKeyTechDev},
		{Title: "d2", URL: "https://example.com/d2", Source: domain.SourceKeyHackerNews, Topic: domain.TopicKeyTechDev},
		{Title: "r1", URL: "https://example.com/r1", Source: domain.SourceKeyArxivCSAI, Topic: domain.TopicKeyTechAIResearch},
		{Title: "r2", URL: "https://example.com/r2", Source: domain.SourceKeyArxivCSAI, Topic: domain.TopicKeyTechAIResearch},
	}

	selected := pipeline.Process(input, []string{
		domain.TopicKeyTechAIResearch,
		domain.TopicKeyTechDev,
		domain.TopicKeyTechStartups,
	})
	if len(selected) != 6 {
		t.Fatalf("expected 6 candidates (2 per topic), got %d", len(selected))
	}
	counts := map[string]int{}
	for _, candidate := range selected {
		counts[candidate.Topic]++
	}
	if counts[domain.TopicKeyTechStartups] != 2 || counts[domain.TopicKeyTechDev] != 2 || counts[domain.TopicKeyTechAIResearch] != 2 {
		t.Fatalf("unexpected per-topic distribution: %#v", counts)
	}
}
