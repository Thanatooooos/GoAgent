package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func TestIssueServiceMarkReadyTransitionsIssue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	repo := &stubIssueRepo{
		issueByID: domain.NewIssue("issue-1", "user-1", "2026-06-29"),
	}
	service := NewIssueService(repo)
	updated, err := service.MarkReady(context.Background(), "issue-1", now)
	if err != nil {
		t.Fatalf("MarkReady returned error: %v", err)
	}
	if updated.Status != domain.IssueStatusReady {
		t.Fatalf("expected issue status ready, got %q", updated.Status)
	}
	if updated.GeneratedAt == nil || !updated.GeneratedAt.Equal(now) {
		t.Fatalf("expected generated_at to be set, got %+v", updated.GeneratedAt)
	}
	if updated.PublishedAt == nil || !updated.PublishedAt.Equal(now) {
		t.Fatalf("expected published_at to be set, got %+v", updated.PublishedAt)
	}
}

func TestIssueServiceMarkGeneratingSupportsRetry(t *testing.T) {
	t.Parallel()

	repo := &stubIssueRepo{
		issueByID: domain.Issue{
			ID:        "issue-2",
			UserID:    "user-1",
			BriefDate: "2026-06-29",
			Status:    domain.IssueStatusFailed,
		},
	}
	service := NewIssueService(repo)
	retryAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	updated, err := service.MarkGenerating(context.Background(), "issue-2", retryAt)
	if err != nil {
		t.Fatalf("MarkGenerating returned error: %v", err)
	}
	if updated.Status != domain.IssueStatusGenerating {
		t.Fatalf("expected issue status generating, got %q", updated.Status)
	}
}

func TestIssueServiceRejectsInvalidTransitions(t *testing.T) {
	t.Parallel()

	repo := &stubIssueRepo{
		issueByID: domain.Issue{
			ID:        "issue-2",
			UserID:    "user-1",
			BriefDate: "2026-06-29",
			Status:    domain.IssueStatusFailed,
		},
	}
	service := NewIssueService(repo)
	if _, err := service.MarkReady(context.Background(), "issue-2", time.Now()); err == nil || !strings.Contains(err.Error(), "cannot transition") {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
}

type stubIssueRepo struct {
	issueByID domain.Issue
}

func (s *stubIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	return issue, nil
}

func (s *stubIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	s.issueByID = issue
	return issue, nil
}

func (s *stubIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	if s.issueByID.ID == id {
		return s.issueByID, nil
	}
	return domain.Issue{}, nil
}

func (s *stubIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	if s.issueByID.UserID == userID && s.issueByID.BriefDate == briefDate {
		return s.issueByID, nil
	}
	return domain.Issue{}, nil
}

func (s *stubIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}
