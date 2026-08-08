package service

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestDedupCollapsesByCanonicalURL(t *testing.T) {
	candidates := []domain.Candidate{
		{
			Title:       "Agent Release",
			URL:         "https://example.com/post?id=1&utm_source=hn",
			Source:      domain.SourceKeyHackerNews,
			Topic:       domain.TopicKeyTechDev,
			PublishedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		},
		{
			Title:       "Agent Release",
			URL:         "https://example.com/post?id=1#comments",
			Source:      domain.SourceKeyHackerNews,
			Topic:       domain.TopicKeyTechDev,
			PublishedAt: time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC),
		},
	}

	deduped := DeduplicateCandidates(candidates)
	if len(deduped) != 1 {
		t.Fatalf("expected URL duplicates to collapse to one candidate, got %d", len(deduped))
	}
	if !deduped[0].PublishedAt.Equal(time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("expected newest candidate to be kept, got %v", deduped[0].PublishedAt)
	}
}

func TestDedupCollapsesBySourceExternalID(t *testing.T) {
	candidates := []domain.Candidate{
		{
			Title:       "Run A",
			URL:         "https://example.com/a",
			Source:      domain.SourceKeyOpenAIBlog,
			Topic:       domain.TopicKeyTechAIModels,
			ExternalID:  "same-id",
			PublishedAt: time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC),
		},
		{
			Title:          "Run A update",
			URL:            "https://example.com/b",
			Source:         domain.SourceKeyOpenAIBlog,
			Topic:          domain.TopicKeyTechAIModels,
			ExternalID:     "same-id",
			SummarySnippet: "longer summary for tie break",
			PublishedAt:    time.Date(2026, 6, 29, 2, 0, 0, 0, time.UTC),
		},
	}

	deduped := DeduplicateCandidates(candidates)
	if len(deduped) != 1 {
		t.Fatalf("expected external id duplicates to collapse, got %d", len(deduped))
	}
	if deduped[0].Title != "Run A update" {
		t.Fatalf("expected preferred candidate to be kept, got %#v", deduped[0])
	}
}

func TestDedupConservativeTitleFallback(t *testing.T) {
	candidates := []domain.Candidate{
		{
			Title:       "Deterministic Candidate Pipeline For Daily Brief",
			URL:         "https://example.com/one",
			Source:      domain.SourceKeyHackerNews,
			Topic:       domain.TopicKeyTechDev,
			PublishedAt: time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC),
		},
		{
			Title:       "Deterministic Candidate Pipeline for Daily Brief!",
			URL:         "https://example.com/two",
			Source:      domain.SourceKeyHackerNews,
			Topic:       domain.TopicKeyTechDev,
			PublishedAt: time.Date(2026, 6, 29, 2, 0, 0, 0, time.UTC),
		},
	}

	deduped := DeduplicateCandidates(candidates)
	if len(deduped) != 1 {
		t.Fatalf("expected similar titles under same source/topic to deduplicate, got %d", len(deduped))
	}
	if deduped[0].URL != "https://example.com/two" {
		t.Fatalf("expected latest title duplicate to be kept, got %#v", deduped[0])
	}
}

func TestNormalizeURLRemovesTrackingAndFragment(t *testing.T) {
	got := NormalizeURL("HTTPS://Example.com:443/post/?utm_source=x&id=2#section")
	want := "https://example.com/post?id=2"
	if got != want {
		t.Fatalf("unexpected normalized URL: got %q want %q", got, want)
	}
}
