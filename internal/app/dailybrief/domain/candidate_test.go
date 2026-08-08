package domain_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestNewCandidateInitializesCoreFields(t *testing.T) {
	before := time.Now()
	candidate := domain.NewCandidate("cand-1", domain.SourceKeyHackerNews, "Launch post", "https://example.com/item")
	after := time.Now()

	if candidate.ID != "cand-1" {
		t.Fatalf("expected id to be preserved, got %q", candidate.ID)
	}
	if candidate.Source != domain.SourceKeyHackerNews {
		t.Fatalf("expected source to be preserved, got %q", candidate.Source)
	}
	if candidate.Title != "Launch post" {
		t.Fatalf("expected title to be preserved, got %q", candidate.Title)
	}
	if candidate.URL != "https://example.com/item" {
		t.Fatalf("expected url to be preserved, got %q", candidate.URL)
	}
	if candidate.ExternalID != "" || candidate.SummarySnippet != "" {
		t.Fatalf("expected optional fields to default empty, got %+v", candidate)
	}
	if !candidate.PublishedAt.IsZero() {
		t.Fatal("expected published_at to default zero time")
	}
	if candidate.Metadata != nil {
		t.Fatalf("expected metadata to default nil, got %+v", candidate.Metadata)
	}
	if candidate.CollectedAt.Before(before) || candidate.CollectedAt.After(after) {
		t.Fatalf("expected collected_at between %v and %v, got %v", before, after, candidate.CollectedAt)
	}
}
