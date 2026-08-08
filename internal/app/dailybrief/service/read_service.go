package service

import (
	"context"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type IssueReadModel struct {
	Issue             domain.Issue
	Items             []domain.Item
	Status            string
	PageState         string
	BriefDate         string
	LastGeneratedTime *time.Time
}

type ReadService struct {
	subscriptionRepo port.SubscriptionRepository
	issueRepo        port.IssueRepository
	itemRepo         port.ItemRepository
}

func NewReadService(subscriptionRepo port.SubscriptionRepository, issueRepo port.IssueRepository, itemRepo port.ItemRepository) *ReadService {
	return &ReadService{
		subscriptionRepo: subscriptionRepo,
		issueRepo:        issueRepo,
		itemRepo:         itemRepo,
	}
}

func (s *ReadService) GetToday(ctx context.Context, userID string, now time.Time) (IssueReadModel, error) {
	subscription, err := s.subscriptionRepo.GetByUserID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return IssueReadModel{}, err
	}
	briefDate, err := domain.ResolveBriefDate(now, subscription.Timezone)
	if err != nil {
		return IssueReadModel{}, err
	}
	return s.getByDateWithSubscription(ctx, userID, briefDate, subscription, now)
}

func (s *ReadService) GetByDate(ctx context.Context, userID string, briefDate string) (IssueReadModel, error) {
	subscription, err := s.subscriptionRepo.GetByUserID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return IssueReadModel{}, err
	}
	return s.getByDateWithSubscription(ctx, userID, briefDate, subscription, time.Now())
}

func (s *ReadService) getByDateWithSubscription(
	ctx context.Context,
	userID string,
	briefDate string,
	subscription domain.Subscription,
	now time.Time,
) (IssueReadModel, error) {
	briefDate = strings.TrimSpace(briefDate)
	issue, err := s.issueRepo.GetByUserIDAndBriefDate(ctx, strings.TrimSpace(userID), briefDate)
	if err != nil {
		return IssueReadModel{}, err
	}
	pageState, err := ResolvePageState(subscription, issue, briefDate, now)
	if err != nil {
		return IssueReadModel{}, err
	}

	model := IssueReadModel{
		BriefDate: briefDate,
		PageState: pageState,
	}
	if strings.TrimSpace(issue.ID) == "" {
		return model, nil
	}

	items, err := s.itemRepo.List(ctx, port.ItemListFilter{
		IssueID: issue.ID,
	})
	if err != nil {
		return IssueReadModel{}, err
	}
	model.Issue = issue
	model.Items = items
	model.Status = issue.Status
	model.LastGeneratedTime = issue.GeneratedAt
	return model, nil
}

func (s *ReadService) GetSubscription(ctx context.Context, userID string) (domain.Subscription, error) {
	return s.subscriptionRepo.GetByUserID(ctx, strings.TrimSpace(userID))
}
