package service

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestResolvePageState(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})
	now := time.Date(2026, 6, 29, 7, 0, 0, 0, time.UTC)

	state, err := ResolvePageState(subscription, domain.Issue{}, "2026-06-29", now)
	if err != nil {
		t.Fatalf("ResolvePageState returned error: %v", err)
	}
	if state != PageStateEmpty {
		t.Fatalf("expected empty before delivery window, got %q", state)
	}

	readyIssue := domain.Issue{ID: "issue-1", Status: domain.IssueStatusReady}
	state, err = ResolvePageState(subscription, readyIssue, "2026-06-29", now)
	if err != nil {
		t.Fatalf("ResolvePageState returned error: %v", err)
	}
	if state != PageStateReady {
		t.Fatalf("expected ready, got %q", state)
	}
	subscription.Enabled = false
	state, err = ResolvePageState(subscription, readyIssue, "2026-06-29", now)
	if err != nil || state != PageStateReady {
		t.Fatalf("paused subscription hid published history: %s %v", state, err)
	}
}

func TestReadServiceGetByDateSetsPageState(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	readService := NewReadService(
		&stubSubscriptionRepo{getResult: domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		&stubReadIssueRepo{issue: domain.Issue{
			ID:          "issue-1",
			UserID:      "user-1",
			BriefDate:   "2026-06-29",
			Status:      domain.IssueStatusReady,
			GeneratedAt: &generatedAt,
		}},
		&stubReadItemRepo{},
	)

	model, err := readService.GetByDate(context.Background(), "user-1", "2026-06-29")
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if model.PageState != PageStateReady || model.BriefDate != "2026-06-29" {
		t.Fatalf("unexpected read model: %+v", model)
	}
}
