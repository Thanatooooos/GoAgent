package service

import (
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

const (
	PageStateEmpty      = "empty"
	PageStateGenerating = "generating"
	PageStateFailed     = "failed"
	PageStateReady      = "ready"
)

func ResolvePageState(subscription domain.Subscription, issue domain.Issue, briefDate string, now time.Time) (string, error) {
	if !subscription.Enabled || len(subscription.Topics) == 0 || len(subscription.Sources) == 0 {
		return PageStateEmpty, nil
	}
	if strings.TrimSpace(issue.ID) != "" {
		switch issue.Status {
		case domain.IssueStatusReady:
			return PageStateReady, nil
		case domain.IssueStatusGenerating:
			return PageStateGenerating, nil
		case domain.IssueStatusFailed:
			return PageStateFailed, nil
		}
	}

	todayBriefDate, err := domain.ResolveBriefDate(now, subscription.Timezone)
	if err != nil {
		return "", err
	}
	if briefDate != todayBriefDate {
		return PageStateEmpty, nil
	}
	open, err := domain.DeliveryWindowOpen(subscription, now)
	if err != nil {
		return "", fmt.Errorf("resolve delivery window: %w", err)
	}
	if !open {
		return PageStateEmpty, nil
	}
	return PageStateEmpty, nil
}
