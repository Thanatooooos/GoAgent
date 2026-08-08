package schedule_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/schedule"
)

func TestResolveBriefDateUsesSubscriptionTimezone(t *testing.T) {
	t.Parallel()

	got, err := schedule.ResolveBriefDate(time.Date(2026, 6, 29, 20, 30, 0, 0, time.UTC), "Asia/Shanghai")
	if err != nil {
		t.Fatalf("ResolveBriefDate returned error: %v", err)
	}
	if got != "2026-06-30" {
		t.Fatalf("expected 2026-06-30, got %q", got)
	}
}

func TestDeliveryWindowOpenRespectsLocalDeliveryTime(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})
	before := time.Date(2026, 6, 29, 7, 30, 0, 0, time.UTC)
	open, err := schedule.DeliveryWindowOpen(subscription, before)
	if err != nil {
		t.Fatalf("DeliveryWindowOpen returned error: %v", err)
	}
	if open {
		t.Fatal("expected delivery window to be closed before 08:00")
	}

	after := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	open, err = schedule.DeliveryWindowOpen(subscription, after)
	if err != nil {
		t.Fatalf("DeliveryWindowOpen returned error: %v", err)
	}
	if !open {
		t.Fatal("expected delivery window to be open at 08:00")
	}
}

func TestShouldScheduleGenerationSkipsReadyIssue(t *testing.T) {
	t.Parallel()

	subscription := domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})
	issue := domain.Issue{ID: "issue-1", Status: domain.IssueStatusReady}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	shouldSchedule, err := schedule.ShouldScheduleGeneration(subscription, issue, now)
	if err != nil {
		t.Fatalf("ShouldScheduleGeneration returned error: %v", err)
	}
	if shouldSchedule {
		t.Fatal("expected ready issue to skip scheduling")
	}
}
