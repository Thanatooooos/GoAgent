package service

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func TestSubscriptionSnapshotServiceRefreshesStoredSources(t *testing.T) {
	repo := &snapshotRepo{
		subscriptions: []domain.Subscription{
			domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechStartups}, nil),
		},
	}
	service := NewSubscriptionSnapshotService(repo)

	result, err := service.Recompute(context.Background(), SubscriptionSnapshotRecomputeInput{})
	if err != nil {
		t.Fatalf("unexpected recompute error: %v", err)
	}
	if result.UpdatedCount != 1 {
		t.Fatalf("expected one updated subscription, got %+v", result)
	}
	if len(repo.upserts) != 1 || len(repo.upserts[0].Sources) == 0 {
		t.Fatalf("expected derived sources to be persisted, got %+v", repo.upserts)
	}
}

type snapshotRepo struct {
	subscriptions []domain.Subscription
	upserts       []domain.Subscription
}

func (r *snapshotRepo) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	r.upserts = append(r.upserts, subscription)
	for i := range r.subscriptions {
		if r.subscriptions[i].UserID == subscription.UserID {
			r.subscriptions[i] = subscription
			break
		}
	}
	return subscription, nil
}

func (r *snapshotRepo) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	for _, sub := range r.subscriptions {
		if sub.UserID == userID {
			return sub, nil
		}
	}
	return domain.Subscription{}, nil
}

func (r *snapshotRepo) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	return append([]domain.Subscription(nil), r.subscriptions...), nil
}

func (r *snapshotRepo) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	return true, nil
}

func (r *snapshotRepo) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	return true, nil
}

func (r *snapshotRepo) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	return true, nil
}
