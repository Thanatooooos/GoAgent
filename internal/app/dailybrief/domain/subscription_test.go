package domain_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestNewSubscriptionInitializesDefaultsAndCopiesSlices(t *testing.T) {
	topics := []string{"tech.ai.models", "tech.ai.research"}
	sources := []string{"hacker-news", "openai-blog"}

	before := time.Now()
	subscription := domain.NewSubscription("user-1", "Asia/Shanghai", "08:30", topics, sources)
	after := time.Now()

	if subscription.UserID != "user-1" {
		t.Fatalf("expected user id to be preserved, got %q", subscription.UserID)
	}
	if !subscription.Enabled {
		t.Fatal("expected subscription to be enabled by default")
	}
	if subscription.Timezone != "Asia/Shanghai" {
		t.Fatalf("expected timezone to be preserved, got %q", subscription.Timezone)
	}
	if subscription.DeliveryTimeLocal != "08:30" {
		t.Fatalf("expected delivery time to be preserved, got %q", subscription.DeliveryTimeLocal)
	}
	assertStringSliceEqual(t, subscription.Topics, []string{"tech.ai.models", "tech.ai.research"}, "topics")
	assertStringSliceEqual(t, subscription.Sources, []string{"hacker-news", "openai-blog"}, "sources")
	if subscription.LockOwner != "" {
		t.Fatalf("expected empty lock owner, got %q", subscription.LockOwner)
	}
	if subscription.LockUntil != nil {
		t.Fatal("expected lock until to be nil by default")
	}
	assertTimeBetween(t, subscription.CreatedAt, before, after, "created at")
	assertTimeBetween(t, subscription.UpdatedAt, before, after, "updated at")
	if !subscription.CreatedAt.Equal(subscription.UpdatedAt) {
		t.Fatalf("expected created at and updated at to match, got %v and %v", subscription.CreatedAt, subscription.UpdatedAt)
	}

	topics[0] = "mutated-topic"
	sources[0] = "mutated-source"

	assertStringSliceEqual(t, subscription.Topics, []string{"tech.ai.models", "tech.ai.research"}, "topics after caller mutation")
	assertStringSliceEqual(t, subscription.Sources, []string{"hacker-news", "openai-blog"}, "sources after caller mutation")
}

func assertStringSliceEqual(t *testing.T, got []string, want []string, name string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected %s length: got %d want %d (%#v vs %#v)", name, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected %s at index %d: got %q want %q (%#v vs %#v)", name, i, got[i], want[i], got, want)
		}
	}
}

func assertTimeBetween(t *testing.T, got time.Time, start time.Time, end time.Time, name string) {
	t.Helper()

	if got.Before(start) || got.After(end) {
		t.Fatalf("expected %s between %v and %v, got %v", name, start, end, got)
	}
}
