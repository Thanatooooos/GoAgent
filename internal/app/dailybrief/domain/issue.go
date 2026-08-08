package domain

import (
	"fmt"
	"time"
)

type Issue struct {
	ID             string
	UserID         string
	BriefDate      string
	Status         string
	Headline       string
	TopSummary     string
	SectionsJSON   string
	ItemCount      int
	PublishedRunID string
	GeneratedAt    *time.Time
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewIssue(id, userID, briefDate string) Issue {
	now := time.Now()
	return Issue{
		ID:        id,
		UserID:    userID,
		BriefDate: briefDate,
		Status:    IssueStatusGenerating,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (i *Issue) MarkGenerating(at time.Time) error {
	switch i.Status {
	case IssueStatusGenerating:
		i.UpdatedAt = at
		return nil
	case IssueStatusFailed:
		i.Status = IssueStatusGenerating
		i.UpdatedAt = at
		return nil
	default:
		return fmt.Errorf("cannot transition issue from %q to %q", i.Status, IssueStatusGenerating)
	}
}

func (i *Issue) MarkReady(publishedAt time.Time) error {
	if i.Status != IssueStatusGenerating {
		return fmt.Errorf("cannot transition issue from %q to %q", i.Status, IssueStatusReady)
	}

	i.Status = IssueStatusReady
	i.GeneratedAt = &publishedAt
	i.PublishedAt = &publishedAt
	i.UpdatedAt = publishedAt
	return nil
}

func (i *Issue) MarkFailed(failedAt time.Time) error {
	if i.Status != IssueStatusGenerating {
		return fmt.Errorf("cannot transition issue from %q to %q", i.Status, IssueStatusFailed)
	}

	i.Status = IssueStatusFailed
	i.UpdatedAt = failedAt
	return nil
}
