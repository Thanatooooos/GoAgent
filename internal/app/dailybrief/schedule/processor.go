package schedule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
)

type GenerationRunner interface {
	Run(ctx context.Context, request dailybriefservice.GenerationRequest) (dailybriefservice.GenerationOutcome, error)
}

type Processor struct {
	runner            GenerationRunner
	subscriptionRepo  port.SubscriptionRepository
	issueRepo         port.IssueRepository
	generationRunRepo port.GenerationRunRepository
	retryPolicy       *RetryPolicy
	metrics           *dailybriefservice.MetricsService
	now               func() time.Time
	StallCutoff       time.Duration
}

const defaultStalledRunCutoff = 30 * time.Minute

func NewProcessor(
	runner GenerationRunner,
	subscriptionRepo port.SubscriptionRepository,
	issueRepo port.IssueRepository,
	generationRunRepo port.GenerationRunRepository,
	retryPolicy *RetryPolicy,
	metrics *dailybriefservice.MetricsService,
	now func() time.Time,
) *Processor {
	if now == nil {
		now = time.Now
	}
	return &Processor{
		runner:            runner,
		subscriptionRepo:  subscriptionRepo,
		issueRepo:         issueRepo,
		generationRunRepo: generationRunRepo,
		retryPolicy:       retryPolicy,
		metrics:           metrics,
		now:               now,
	}
}

func (p *Processor) ProcessSubscription(ctx context.Context, subscription domain.Subscription, triggerType string) error {
	if p == nil || p.runner == nil {
		return fmt.Errorf("daily brief processor runner is not configured")
	}
	now := p.now()
	briefDate, err := ResolveBriefDate(now, subscription.Timezone)
	if err != nil {
		return err
	}
	issue, err := p.issueRepo.GetByUserIDAndBriefDate(ctx, subscription.UserID, briefDate)
	if err != nil {
		return err
	}
	if strings.TrimSpace(issue.ID) != "" {
		switch issue.Status {
		case domain.IssueStatusReady, domain.IssueStatusGenerating:
			return nil
		}
	}
	if triggerType == domain.GenerationRunTriggerTypeScheduled {
		shouldSchedule, err := ShouldScheduleGeneration(subscription, issue, now)
		if err != nil {
			return err
		}
		if !shouldSchedule {
			return nil
		}
	}
	_, err = p.runner.Run(ctx, dailybriefservice.GenerationRequest{
		Subscription: subscription,
		BriefDate:    briefDate,
		TriggerType:  triggerType,
		Now:          now,
	})
	return err
}

func (p *Processor) ProcessRetry(ctx context.Context, run domain.GenerationRun) error {
	if p == nil || p.runner == nil {
		return fmt.Errorf("daily brief processor runner is not configured")
	}
	if p.subscriptionRepo == nil || p.generationRunRepo == nil || p.retryPolicy == nil {
		return fmt.Errorf("daily brief processor retry dependencies are not configured")
	}
	if run.Status != domain.GenerationRunStatusFailed || run.FinishedAt == nil {
		return nil
	}
	latestFailedRun, err := p.generationRunRepo.GetLatestFailedByUserIDAndBriefDate(ctx, run.UserID, run.BriefDate)
	if err != nil {
		return err
	}
	retryAnchorRun := run
	if strings.TrimSpace(latestFailedRun.ID) != "" && latestFailedRun.FinishedAt != nil {
		retryAnchorRun = latestFailedRun
	}
	subscription, err := p.subscriptionRepo.GetByUserID(ctx, run.UserID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(subscription.UserID) == "" || !subscription.Enabled {
		return nil
	}
	now := p.now()
	currentBriefDate, err := ResolveBriefDate(now, subscription.Timezone)
	if err != nil {
		return err
	}
	if currentBriefDate != run.BriefDate {
		return nil
	}
	retryCount, err := p.generationRunRepo.CountRetryRunsByUserIDAndBriefDate(ctx, run.UserID, run.BriefDate)
	if err != nil {
		return err
	}
	if !p.retryPolicy.ShouldRetry(retryCount, *retryAnchorRun.FinishedAt, now) {
		return nil
	}
	if p.metrics != nil {
		p.metrics.RecordRetryAttempt()
	}
	return p.ProcessSubscription(ctx, subscription, domain.GenerationRunTriggerTypeRetry)
}

func (p *Processor) ProcessRetryBatch(ctx context.Context, now time.Time, limit int) error {
	if p == nil || p.generationRunRepo == nil {
		return nil
	}
	var batchErrors []error
	if err := p.recoverStalledRuns(ctx, now, limit); err != nil {
		batchErrors = append(batchErrors, err)
	}
	runs, err := p.generationRunRepo.ListRetryEligible(ctx, port.GenerationRunRetryEligibleFilter{
		FailedBefore: now,
		Limit:        limit,
	})
	if err != nil {
		return err
	}
	var retryErrors []error
	for _, run := range runs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := p.ProcessRetry(ctx, run); err != nil {
			retryErrors = append(retryErrors, err)
		}
	}
	return errors.Join(append(batchErrors, retryErrors...)...)
}

func (p *Processor) stalledRunCutoff() time.Duration {
	if p == nil || p.StallCutoff <= 0 {
		return defaultStalledRunCutoff
	}
	return p.StallCutoff
}

func (p *Processor) recoverStalledRuns(ctx context.Context, now time.Time, limit int) error {
	if p == nil || p.generationRunRepo == nil {
		return nil
	}
	stalled, err := p.generationRunRepo.ListStalledRunning(ctx, port.GenerationRunStalledFilter{
		StartedBefore: now.Add(-p.stalledRunCutoff()),
		Limit:         limit,
	})
	if err != nil {
		return err
	}
	var recoverErrors []error
	for _, run := range stalled {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := p.recoverStalledRun(ctx, run, now); err != nil {
			recoverErrors = append(recoverErrors, err)
		}
	}
	return errors.Join(recoverErrors...)
}

func (p *Processor) recoverStalledRun(ctx context.Context, run domain.GenerationRun, now time.Time) error {
	if p.issueRepo != nil {
		if err := p.markStalledIssueFailed(ctx, run, now); err != nil {
			return err
		}
	}
	if err := run.MarkFailed(now, "generation stalled"); err != nil {
		return err
	}
	if _, err := p.generationRunRepo.Update(ctx, run); err != nil {
		return err
	}
	return p.ProcessRetry(ctx, run)
}

func (p *Processor) markStalledIssueFailed(ctx context.Context, run domain.GenerationRun, now time.Time) error {
	issue, err := p.issueRepo.GetByUserIDAndBriefDate(ctx, run.UserID, run.BriefDate)
	if err != nil {
		return err
	}
	if strings.TrimSpace(issue.ID) == "" || issue.Status != domain.IssueStatusGenerating {
		return nil
	}
	if markErr := issue.MarkFailed(now); markErr != nil {
		return markErr
	}
	_, err = p.issueRepo.Update(ctx, issue)
	return err
}
