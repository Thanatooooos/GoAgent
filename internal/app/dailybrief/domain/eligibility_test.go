package domain_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestShouldScheduleGenerationRequiresEnabledSubscription(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyHackerNews})
	subscription.Enabled = false
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	shouldSchedule, err := domain.ShouldScheduleGeneration(subscription, domain.Issue{}, now)
	if err != nil {
		t.Fatalf("ShouldScheduleGeneration returned error: %v", err)
	}
	if shouldSchedule {
		t.Fatal("expected disabled subscription to skip scheduling")
	}
}

func TestShouldScheduleGenerationWaitsForDeliveryWindow(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyHackerNews})
	beforeWindow := time.Date(2026, 6, 29, 7, 59, 0, 0, time.UTC)

	shouldSchedule, err := domain.ShouldScheduleGeneration(subscription, domain.Issue{}, beforeWindow)
	if err != nil {
		t.Fatalf("ShouldScheduleGeneration returned error: %v", err)
	}
	if shouldSchedule {
		t.Fatal("expected delivery window to block scheduling before 08:00")
	}
}

func TestShouldScheduleGenerationSkipsFailedIssueForScheduledPath(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyHackerNews})
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	shouldSchedule, err := domain.ShouldScheduleGeneration(subscription, domain.Issue{
		ID:        "issue-1",
		UserID:    "user-1",
		BriefDate: "2026-06-29",
		Status:    domain.IssueStatusFailed,
	}, now)
	if err != nil {
		t.Fatalf("ShouldScheduleGeneration returned error: %v", err)
	}
	if shouldSchedule {
		t.Fatal("expected failed issue to wait for retry path instead of scheduled path")
	}
}
