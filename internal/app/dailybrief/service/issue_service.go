package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type IssueService struct {
	repo port.IssueRepository
}

func NewIssueService(repo port.IssueRepository) *IssueService {
	return &IssueService{repo: repo}
}

func (s *IssueService) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	if strings.TrimSpace(issue.ID) == "" {
		return domain.Issue{}, fmt.Errorf("issue id is required")
	}
	if strings.TrimSpace(issue.UserID) == "" {
		return domain.Issue{}, fmt.Errorf("issue user id is required")
	}
	if strings.TrimSpace(issue.BriefDate) == "" {
		return domain.Issue{}, fmt.Errorf("issue brief date is required")
	}
	if issue.Status == "" {
		issue.Status = domain.IssueStatusGenerating
	}
	if !domain.IsValidIssueStatus(issue.Status) {
		return domain.Issue{}, fmt.Errorf("issue status %q is invalid", issue.Status)
	}
	return s.repo.Create(ctx, issue)
}

func (s *IssueService) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	if strings.TrimSpace(issue.ID) == "" {
		return domain.Issue{}, fmt.Errorf("issue id is required")
	}
	if strings.TrimSpace(issue.UserID) == "" {
		return domain.Issue{}, fmt.Errorf("issue user id is required")
	}
	if strings.TrimSpace(issue.BriefDate) == "" {
		return domain.Issue{}, fmt.Errorf("issue brief date is required")
	}
	if !domain.IsValidIssueStatus(issue.Status) {
		return domain.Issue{}, fmt.Errorf("issue status %q is invalid", issue.Status)
	}
	return s.repo.Update(ctx, issue)
}

func (s *IssueService) MarkGenerating(ctx context.Context, id string, at time.Time) (domain.Issue, error) {
	issue, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Issue{}, err
	}
	if strings.TrimSpace(issue.ID) == "" {
		return domain.Issue{}, fmt.Errorf("issue %q not found", id)
	}
	if err := issue.MarkGenerating(at); err != nil {
		return domain.Issue{}, err
	}
	return s.repo.Update(ctx, issue)
}

func (s *IssueService) MarkReady(ctx context.Context, id string, generatedAt time.Time) (domain.Issue, error) {
	issue, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Issue{}, err
	}
	if strings.TrimSpace(issue.ID) == "" {
		return domain.Issue{}, fmt.Errorf("issue %q not found", id)
	}
	if err := issue.MarkReady(generatedAt); err != nil {
		return domain.Issue{}, err
	}
	return s.repo.Update(ctx, issue)
}

func (s *IssueService) MarkFailed(ctx context.Context, id string, failedAt time.Time) (domain.Issue, error) {
	issue, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.Issue{}, err
	}
	if strings.TrimSpace(issue.ID) == "" {
		return domain.Issue{}, fmt.Errorf("issue %q not found", id)
	}
	if err := issue.MarkFailed(failedAt); err != nil {
		return domain.Issue{}, err
	}
	return s.repo.Update(ctx, issue)
}

func (s *IssueService) GetByUserIDAndBriefDate(ctx context.Context, userID, briefDate string) (domain.Issue, error) {
	return s.repo.GetByUserIDAndBriefDate(ctx, strings.TrimSpace(userID), strings.TrimSpace(briefDate))
}
