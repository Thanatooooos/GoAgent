package service

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func TestReadServiceGetByDateBuildsPageReadModel(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	issueRepo := &stubReadIssueRepo{
		issue: domain.Issue{
			ID:          "issue-1",
			UserID:      "user-1",
			BriefDate:   "2026-06-29",
			Status:      domain.IssueStatusReady,
			GeneratedAt: &generatedAt,
		},
	}
	itemRepo := &stubReadItemRepo{
		items: []domain.Item{{ID: "item-1", IssueID: "issue-1", Title: "headline"}},
	}
	readService := NewReadService(
		&stubSubscriptionRepo{getResult: domain.NewSubscription("user-1", "UTC", "08:00", []string{"tech.ai.models"}, []string{"hacker-news"})},
		issueRepo,
		itemRepo,
	)

	model, err := readService.GetByDate(context.Background(), "user-1", "2026-06-29")
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if model.Issue.ID != "issue-1" || model.Status != domain.IssueStatusReady {
		t.Fatalf("unexpected issue read model: %+v", model)
	}
	if model.PageState != PageStateReady {
		t.Fatalf("unexpected page state: %q", model.PageState)
	}
	if len(model.Items) != 1 || model.Items[0].ID != "item-1" {
		t.Fatalf("unexpected item list: %+v", model.Items)
	}
	if model.LastGeneratedTime == nil || !model.LastGeneratedTime.Equal(generatedAt) {
		t.Fatalf("unexpected last generated time: %+v", model.LastGeneratedTime)
	}
}

func TestReadServiceGetTodayUsesSubscriptionTimezone(t *testing.T) {
	t.Parallel()

	subscriptionRepo := &stubSubscriptionRepo{
		getResult: domain.Subscription{
			UserID:   "user-1",
			Timezone: "Asia/Shanghai",
		},
	}
	issueRepo := &stubReadIssueRepo{
		issue: domain.Issue{
			ID:        "issue-2",
			UserID:    "user-1",
			BriefDate: "2026-06-30",
			Status:    domain.IssueStatusGenerating,
		},
	}
	readService := NewReadService(subscriptionRepo, issueRepo, &stubReadItemRepo{})

	// 2026-06-29 20:30 UTC = 2026-06-30 04:30 Asia/Shanghai.
	model, err := readService.GetToday(context.Background(), "user-1", time.Date(2026, 6, 29, 20, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetToday returned error: %v", err)
	}
	if model.Issue.BriefDate != "2026-06-30" {
		t.Fatalf("expected timezone-resolved brief date, got %q", model.Issue.BriefDate)
	}
}

type stubReadIssueRepo struct {
	issue domain.Issue
}

func (s *stubReadIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *stubReadIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	s.issue = issue
	return issue, nil
}

func (s *stubReadIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	if s.issue.ID == id {
		return s.issue, nil
	}
	return domain.Issue{}, nil
}

func (s *stubReadIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	if s.issue.UserID == userID && s.issue.BriefDate == briefDate {
		return s.issue, nil
	}
	return domain.Issue{}, nil
}

func (s *stubReadIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}

type stubReadItemRepo struct {
	items []domain.Item
}

func (s *stubReadItemRepo) Create(ctx context.Context, item domain.Item) (domain.Item, error) {
	return item, nil
}

func (s *stubReadItemRepo) ReplaceIssueItems(ctx context.Context, issueID string, items []domain.Item) error {
	s.items = append([]domain.Item(nil), items...)
	return nil
}

func (s *stubReadItemRepo) List(ctx context.Context, filter port.ItemListFilter) ([]domain.Item, error) {
	return append([]domain.Item(nil), s.items...), nil
}
