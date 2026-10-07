package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"gorm.io/gorm"

	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/app/scheduledtask/service"
	"local/rag-project/internal/framework/config"
)

type Runtime struct {
	Store           *storepkg.Store
	Executor        service.Executor
	claimOptions    storepkg.ClaimOptions
	interval        time.Duration
	retryDelay      time.Duration
	feedbackOptions storepkg.FeedbackOptions
	slots           chan struct{}
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func NewRuntime(db *gorm.DB, taskRuntime runtime.TaskRuntime, cfg config.ScheduledTaskConfig) (*Runtime, error) {
	if db == nil || taskRuntime == nil {
		return nil, fmt.Errorf("scheduled tasks require a database and task runtime")
	}
	if cfg.ScanIntervalSeconds < 1 || cfg.ClaimBatchSize < 1 || cfg.MaxConcurrentRuns < 1 || cfg.LeaseSeconds < 1 ||
		cfg.OnceDeadlineSeconds < 1 || cfg.RecurringDeadlineSeconds < 1 || cfg.RetryMaxAttempts < 1 ||
		cfg.RetryDelaySeconds < 1 || cfg.DailyFeedbackDays < 1 || cfg.WeeklyFeedbackDays < 1 || cfg.MonthlyFeedbackDays < 1 || cfg.FailureThreshold < 1 {
		return nil, fmt.Errorf("scheduled task configuration requires positive scan, lease, deadlines, retries, and feedback intervals")
	}
	return &Runtime{
		Store:    storepkg.NewStore(db),
		Executor: service.Executor{Runtime: taskRuntime, Access: runtimeadapter.NewGlobalKnowledgeBaseAccess(db)},
		claimOptions: storepkg.ClaimOptions{Limit: cfg.ClaimBatchSize, WorkerID: fmt.Sprintf("%s-%d", hostname(), os.Getpid()),
			Lease:             time.Duration(cfg.LeaseSeconds) * time.Second,
			OnceDeadline:      time.Duration(cfg.OnceDeadlineSeconds) * time.Second,
			RecurringDeadline: time.Duration(cfg.RecurringDeadlineSeconds) * time.Second,
			MaxAttempts:       cfg.RetryMaxAttempts},
		interval:   time.Duration(cfg.ScanIntervalSeconds) * time.Second,
		retryDelay: time.Duration(cfg.RetryDelaySeconds) * time.Second,
		feedbackOptions: storepkg.FeedbackOptions{Daily: time.Duration(cfg.DailyFeedbackDays) * 24 * time.Hour,
			Weekly:  time.Duration(cfg.WeeklyFeedbackDays) * 24 * time.Hour,
			Monthly: time.Duration(cfg.MonthlyFeedbackDays) * 24 * time.Hour, FailureThreshold: cfg.FailureThreshold},
		slots: make(chan struct{}, cfg.MaxConcurrentRuns),
	}, nil
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "server"
	}
	return name
}

func (r *Runtime) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			if err := r.scanAndDispatch(ctx, time.Now()); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "scheduled task worker: %v\n", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	return nil
}

func (r *Runtime) scanAndDispatch(ctx context.Context, now time.Time) error {
	available := cap(r.slots) - len(r.slots)
	if available > r.claimOptions.Limit {
		available = r.claimOptions.Limit
	}
	if available > 0 {
		opts := r.claimOptions
		opts.Limit = available
		pending, err := r.Store.ClaimPending(ctx, now, opts)
		if err != nil {
			return err
		}
		opts.Limit -= len(pending)
		var due []storepkg.Claim
		if opts.Limit > 0 {
			due, err = r.Store.ClaimDue(ctx, now, opts)
			if err != nil {
				return err
			}
		}
		for _, claim := range append(pending, due...) {
			r.slots <- struct{}{}
			r.wg.Add(1)
			go func(claim storepkg.Claim) {
				defer r.wg.Done()
				defer func() { <-r.slots }()
				if err := r.runClaim(ctx, claim); err != nil && ctx.Err() == nil {
					fmt.Fprintf(os.Stderr, "scheduled occurrence %s: %v\n", claim.OccurrenceID, err)
				}
			}(claim)
		}
	}
	_, err := r.Store.PublishDueFeedback(ctx, now, r.claimOptions.Limit, r.feedbackOptions)
	return err
}

func (r *Runtime) RunOnce(ctx context.Context, now time.Time) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("scheduled task runtime is not initialized")
	}
	pending, err := r.Store.ClaimPending(ctx, now, r.claimOptions)
	if err != nil {
		return err
	}
	due, err := r.Store.ClaimDue(ctx, now, r.claimOptions)
	if err != nil {
		return err
	}
	var all error
	for _, claim := range append(pending, due...) {
		if err := r.runClaim(ctx, claim); err != nil {
			all = errors.Join(all, fmt.Errorf("occurrence %s: %w", claim.OccurrenceID, err))
		}
	}
	if _, err := r.Store.PublishDueFeedback(ctx, time.Now(), r.claimOptions.Limit, r.feedbackOptions); err != nil {
		all = errors.Join(all, fmt.Errorf("scheduled status feedback: %w", err))
	}
	return all
}

func (r *Runtime) runClaim(ctx context.Context, claim storepkg.Claim) error {
	if claim.ReadyOutcome != nil {
		return r.publish(ctx, claim, *claim.ReadyOutcome)
	}
	now := time.Now()
	attemptID, err := r.Store.StartAttempt(ctx, claim, now)
	if err != nil {
		return err
	}
	history, historyErr := r.Store.History(ctx, claim.Task.ID, claim.Version.Number, 10)
	var result service.ExecutionResult
	if historyErr == nil {
		runCtx, cancel := context.WithDeadline(ctx, claim.DeadlineAt)
		go func() {
			interval := r.claimOptions.Lease / 3
			if interval < time.Second {
				interval = time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					if err := r.Store.RenewLease(runCtx, claim, time.Now(), r.claimOptions.Lease); err != nil {
						cancel()
						return
					}
				}
			}
		}()
		result, err = r.Executor.Run(runCtx, service.ExecutionInput{Task: claim.Task, Version: claim.Version,
			AttemptID: attemptID, OccurrenceID: claim.OccurrenceID, ScheduledAt: claim.ScheduledAt, History: history})
		cancel()
	} else {
		err = historyErr
	}
	finished := time.Now()
	input := storepkg.FinishInput{Claim: claim, AttemptID: attemptID, RuntimeSessionID: result.RuntimeSessionID,
		Now: finished, MaxAttempts: r.claimOptions.MaxAttempts, RetryDelay: r.retryDelay}
	if err != nil {
		input.TechnicalError = err.Error()
		if len(input.TechnicalError) > 2000 {
			input.TechnicalError = input.TechnicalError[:2000]
		}
	} else {
		input.Outcome = &result.Outcome
	}
	if saveErr := r.Store.FinishAttempt(ctx, input); saveErr != nil {
		return errors.Join(err, saveErr)
	}
	if err != nil {
		return err
	}
	if result.Outcome.Signal == domain.SignalReport && !finished.After(claim.DeadlineAt) {
		return r.publish(ctx, claim, result.Outcome)
	}
	return nil
}

func (r *Runtime) publish(ctx context.Context, claim storepkg.Claim, outcome domain.Outcome) error {
	if err := r.Executor.CheckAccess(ctx, claim.Task.UserID, claim.Version); err != nil {
		return err
	}
	_, err := r.Store.PublishReport(ctx, storepkg.PublishReportInput{TaskID: claim.Task.ID,
		UserID: claim.Task.UserID, Version: claim.Version.Number, OccurrenceID: claim.OccurrenceID,
		Body: outcome.Body, Sources: outcome.Sources, Now: time.Now()})
	return err
}
