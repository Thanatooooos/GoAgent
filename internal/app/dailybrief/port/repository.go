package port

import (
	"context"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

type ListOptions struct {
	Offset int
	Limit  int
}

type SubscriptionListFilter struct {
	Enabled  *bool
	Timezone string
	ListOptions
}

type IssueListFilter struct {
	UserID    string
	BriefDate string
	Status    string
	ListOptions
}

type ItemListFilter struct {
	IssueID    string
	SectionKey string
	ListOptions
}

type GenerationRunRetryEligibleFilter struct {
	FailedBefore time.Time
	Limit        int
}

type SubscriptionRepository interface {
	Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error)
	GetByUserID(ctx context.Context, userID string) (domain.Subscription, error)
	List(ctx context.Context, filter SubscriptionListFilter) ([]domain.Subscription, error)
	TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error)
	RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error)
	ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error)
}

type IssueRepository interface {
	Create(ctx context.Context, issue domain.Issue) (domain.Issue, error)
	Update(ctx context.Context, issue domain.Issue) (domain.Issue, error)
	GetByID(ctx context.Context, id string) (domain.Issue, error)
	GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error)
	List(ctx context.Context, filter IssueListFilter) ([]domain.Issue, error)
}

type ItemRepository interface {
	Create(ctx context.Context, item domain.Item) (domain.Item, error)
	ReplaceIssueItems(ctx context.Context, issueID string, items []domain.Item) error
	List(ctx context.Context, filter ItemListFilter) ([]domain.Item, error)
}

type GenerationRunRepository interface {
	Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error)
	Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error)
	GetByID(ctx context.Context, id string) (domain.GenerationRun, error)
	GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error)
	ListRetryEligible(ctx context.Context, filter GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error)
	CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error)
}
