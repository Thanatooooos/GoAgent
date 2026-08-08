package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func TestSubscriptionServiceUpsertDerivesSourcesFromTopics(t *testing.T) {
	repo := &stubSubscriptionRepo{}
	service := NewSubscriptionService(repo)

	subscription := domain.Subscription{
		UserID:            "user-1",
		Enabled:           true,
		Timezone:          "UTC",
		DeliveryTimeLocal: "08:00",
		Topics:            []string{domain.TopicKeyTechAIModels, domain.TopicKeyTechDev},
		Sources:           []string{"user-supplied-value"},
	}

	_, err := service.Upsert(context.Background(), subscription)
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	want := []string{
		domain.SourceKeyAnthropicBlog,
		domain.SourceKeyGitHubTrending,
		domain.SourceKeyGoogleDeepMind,
		domain.SourceKeyHackerNews,
		domain.SourceKeyMetaAIBlog,
		domain.SourceKeyOpenAIBlog,
	}
	if !reflect.DeepEqual(repo.lastUpsert.Sources, want) {
		t.Fatalf("unexpected derived sources: got %v want %v", repo.lastUpsert.Sources, want)
	}
}

func TestSubscriptionServiceRejectsDisabledOrNonLeafTopics(t *testing.T) {
	service := NewSubscriptionService(&stubSubscriptionRepo{})

	_, err := service.Upsert(context.Background(), domain.Subscription{
		UserID:            "user-1",
		Enabled:           true,
		Timezone:          "UTC",
		DeliveryTimeLocal: "08:00",
		Topics:            []string{"tech"},
	})
	if err == nil || !strings.Contains(err.Error(), "selectable leaf") {
		t.Fatalf("expected non-leaf topic error, got %v", err)
	}
}

func TestSubscriptionServiceUpsertValidatesTimezoneAndCatalogs(t *testing.T) {
	t.Parallel()

	service := NewSubscriptionService(&stubSubscriptionRepo{})
	subscription := domain.NewSubscription("user-1", "Asia/Shanghai", "08:30", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyHackerNews})

	if _, err := service.Upsert(context.Background(), subscription); err != nil {
		t.Fatalf("expected valid subscription to pass, got %v", err)
	}

	subscription.Timezone = "Invalid/Timezone"
	if _, err := service.Upsert(context.Background(), subscription); err == nil || !strings.Contains(err.Error(), "timezone is invalid") {
		t.Fatalf("expected invalid timezone error, got %v", err)
	}

	subscription.Timezone = "UTC"
	subscription.Topics = []string{"unknown"}
	if _, err := service.Upsert(context.Background(), subscription); err == nil || !strings.Contains(err.Error(), "selectable leaf") {
		t.Fatalf("expected unsupported topic error, got %v", err)
	}
}

func TestSubscriptionServiceUpsertNormalizesDuplicates(t *testing.T) {
	t.Parallel()

	repo := &stubSubscriptionRepo{}
	service := NewSubscriptionService(repo)
	subscription := domain.NewSubscription(
		"user-1",
		" UTC ",
		" 08:30 ",
		[]string{domain.TopicKeyTechAIModels, domain.TopicKeyTechAIModels, " "},
		nil,
	)
	_, err := service.Upsert(context.Background(), subscription)
	if err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if repo.lastUpsert.UserID != "user-1" || repo.lastUpsert.Timezone != "UTC" || repo.lastUpsert.DeliveryTimeLocal != "08:30" {
		t.Fatalf("expected normalized identifiers, got %+v", repo.lastUpsert)
	}
	if len(repo.lastUpsert.Topics) != 1 || repo.lastUpsert.Topics[0] != domain.TopicKeyTechAIModels {
		t.Fatalf("expected deduplicated topics, got %+v", repo.lastUpsert.Topics)
	}
	if len(repo.lastUpsert.Sources) != 4 {
		t.Fatalf("expected derived sources for tech.ai.models, got %+v", repo.lastUpsert.Sources)
	}
}

type stubSubscriptionRepo struct {
	lastUpsert domain.Subscription
	getResult  domain.Subscription
}

func (s *stubSubscriptionRepo) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	s.lastUpsert = subscription
	if subscription.CreatedAt.IsZero() {
		subscription.CreatedAt = time.Now()
	}
	subscription.UpdatedAt = time.Now()
	return subscription, nil
}

func (s *stubSubscriptionRepo) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	return s.getResult, nil
}

func (s *stubSubscriptionRepo) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	return nil, nil
}

func (s *stubSubscriptionRepo) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (s *stubSubscriptionRepo) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	return true, nil
}

func (s *stubSubscriptionRepo) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	return true, nil
}
