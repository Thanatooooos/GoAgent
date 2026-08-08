package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	postgresdailybrief "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/framework/distributedid"
)

type PublishInput struct {
	Issue         domain.Issue
	Artifact      domain.BriefArtifact
	PublishedRunID string
	PublishedAt   time.Time
}

type Publisher struct {
	publishTx postgresdailybrief.PublishTransaction
}

func NewPublisher(publishTx postgresdailybrief.PublishTransaction) *Publisher {
	return &Publisher{publishTx: publishTx}
}

func (p *Publisher) Publish(ctx context.Context, input PublishInput) (domain.Issue, []domain.Item, error) {
	if p == nil || p.publishTx == nil {
		return domain.Issue{}, nil, fmt.Errorf("publish transaction is not configured")
	}
	if strings.TrimSpace(input.Issue.ID) == "" {
		return domain.Issue{}, nil, fmt.Errorf("issue id is required for publish")
	}
	if strings.TrimSpace(input.PublishedRunID) == "" {
		return domain.Issue{}, nil, fmt.Errorf("published run id is required for publish")
	}

	sectionsJSON, err := MarshalBriefSections(input.Artifact.Sections)
	if err != nil {
		return domain.Issue{}, nil, err
	}
	items, err := buildPublishedItems(input.Issue.ID, input.Artifact, input.PublishedAt)
	if err != nil {
		return domain.Issue{}, nil, err
	}

	issue := input.Issue
	issue.Headline = input.Artifact.Headline
	issue.TopSummary = input.Artifact.TopSummary
	issue.SectionsJSON = sectionsJSON
	issue.ItemCount = len(items)
	issue.PublishedRunID = input.PublishedRunID
	if err := issue.MarkReady(input.PublishedAt); err != nil {
		return domain.Issue{}, nil, err
	}

	err = p.publishTx(ctx, func(ctx context.Context, issueRepo port.IssueRepository, itemRepo port.ItemRepository) error {
		if _, err := issueRepo.Update(ctx, issue); err != nil {
			return err
		}
		return itemRepo.ReplaceIssueItems(ctx, issue.ID, items)
	})
	if err != nil {
		return domain.Issue{}, nil, fmt.Errorf("publish daily brief issue: %w", err)
	}
	return issue, items, nil
}

func buildPublishedItems(issueID string, artifact domain.BriefArtifact, publishedAt time.Time) ([]domain.Item, error) {
	items := make([]domain.Item, 0)
	rank := 1
	for _, section := range artifact.Sections {
		for _, draft := range section.Items {
			id, err := distributedid.NextID()
			if err != nil {
				return nil, fmt.Errorf("generate daily brief item id: %w", err)
			}
			item := domain.NewItem(fmt.Sprintf("%d", id), issueID, section.Key, rank, draft.Title)
			item.Summary = draft.Summary
			item.WhyItMatters = draft.WhyItMatters
			item.URL = draft.URL
			item.Source = draft.Source
			item.Topic = draft.Topic
			item.PublishedAt = &publishedAt
			items = append(items, item)
			rank++
		}
	}
	return items, nil
}
