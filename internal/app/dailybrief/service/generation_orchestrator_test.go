package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/framework/config"
)

type stubBriefGenerator struct {
	artifact domain.BriefArtifact
}

func (s *stubBriefGenerator) Generate(ctx context.Context, input port.BriefGenerationInput) (port.BriefGenerationResult, error) {
	return port.BriefGenerationResult{
		Output:        s.artifact,
		Model:         "test-model",
		PromptVersion: "v1",
	}, nil
}

type cancelingBriefGenerator struct {
	cancel func()
	err    error
}

func (s *cancelingBriefGenerator) Generate(ctx context.Context, input port.BriefGenerationInput) (port.BriefGenerationResult, error) {
	if s.cancel != nil {
		s.cancel()
	}
	return port.BriefGenerationResult{}, s.err
}

type orchestratorRunRepo struct {
	run domain.GenerationRun
}

func (s *orchestratorRunRepo) Create(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	s.run = run
	return run, nil
}

func (s *orchestratorRunRepo) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	s.run = run
	return run, nil
}

func (s *orchestratorRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	if s.run.ID == id {
		return s.run, nil
	}
	return domain.GenerationRun{}, nil
}

func (s *orchestratorRunRepo) GetLatestFailedByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.GenerationRun, error) {
	if s.run.UserID == userID && s.run.BriefDate == briefDate && s.run.Status == domain.GenerationRunStatusFailed {
		return s.run, nil
	}
	return domain.GenerationRun{}, nil
}

func (s *orchestratorRunRepo) ListRetryEligible(ctx context.Context, filter port.GenerationRunRetryEligibleFilter) ([]domain.GenerationRun, error) {
	return nil, nil
}

func (s *orchestratorRunRepo) CountRetryRunsByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (int, error) {
	if s.run.UserID == userID && s.run.BriefDate == briefDate && s.run.ID != "" {
		return 1, nil
	}
	return 0, nil
}

type cancelAwareOrchestratorRunRepo struct {
	orchestratorRunRepo
}

func (s *cancelAwareOrchestratorRunRepo) Update(ctx context.Context, run domain.GenerationRun) (domain.GenerationRun, error) {
	if err := ctx.Err(); err != nil {
		return domain.GenerationRun{}, err
	}
	return s.orchestratorRunRepo.Update(ctx, run)
}

func (s *cancelAwareOrchestratorRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	if err := ctx.Err(); err != nil {
		return domain.GenerationRun{}, err
	}
	return s.orchestratorRunRepo.GetByID(ctx, id)
}

type stubSourceProvider struct {
	sourceKey string
	items     []domain.Candidate
}

func (s *stubSourceProvider) SourceKey() string { return s.sourceKey }

func (s *stubSourceProvider) Fetch(ctx context.Context, client port.HTTPClient) ([]domain.Candidate, error) {
	return append([]domain.Candidate(nil), s.items...), nil
}

type failingSourceProvider struct {
	sourceKey string
	err       error
}

func (s *failingSourceProvider) SourceKey() string { return s.sourceKey }

func (s *failingSourceProvider) Fetch(ctx context.Context, client port.HTTPClient) ([]domain.Candidate, error) {
	return nil, s.err
}

type orchestratorIssueRepo struct {
	issueByDate domain.Issue
}

func (s *orchestratorIssueRepo) Create(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	s.issueByDate = issue
	return issue, nil
}

func (s *orchestratorIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	s.issueByDate = issue
	return issue, nil
}

func (s *orchestratorIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	if s.issueByDate.ID == id {
		return s.issueByDate, nil
	}
	return domain.Issue{}, nil
}

func (s *orchestratorIssueRepo) GetByUserIDAndBriefDate(ctx context.Context, userID string, briefDate string) (domain.Issue, error) {
	if s.issueByDate.UserID == userID && s.issueByDate.BriefDate == briefDate && s.issueByDate.ID != "" {
		return s.issueByDate, nil
	}
	return domain.Issue{}, nil
}

func (s *orchestratorIssueRepo) List(ctx context.Context, filter port.IssueListFilter) ([]domain.Issue, error) {
	return nil, nil
}

type cancelAwareOrchestratorIssueRepo struct {
	orchestratorIssueRepo
}

func (s *cancelAwareOrchestratorIssueRepo) Update(ctx context.Context, issue domain.Issue) (domain.Issue, error) {
	if err := ctx.Err(); err != nil {
		return domain.Issue{}, err
	}
	return s.orchestratorIssueRepo.Update(ctx, issue)
}

func (s *cancelAwareOrchestratorIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	if err := ctx.Err(); err != nil {
		return domain.Issue{}, err
	}
	return s.orchestratorIssueRepo.GetByID(ctx, id)
}

type updateOnlyOrchestratorRunRepo struct {
	orchestratorRunRepo
}

func (s *updateOnlyOrchestratorRunRepo) GetByID(ctx context.Context, id string) (domain.GenerationRun, error) {
	return domain.GenerationRun{}, context.DeadlineExceeded
}

type updateOnlyOrchestratorIssueRepo struct {
	orchestratorIssueRepo
}

func (s *updateOnlyOrchestratorIssueRepo) GetByID(ctx context.Context, id string) (domain.Issue, error) {
	return domain.Issue{}, context.DeadlineExceeded
}

func TestGenerationOrchestratorPublishesDegradedWhenPartialSourceFailure(t *testing.T) {
	t.Parallel()

	registry := NewSourceRegistry(
		&stubSourceProvider{
			sourceKey: domain.SourceKeyOpenAIBlog,
			items: []domain.Candidate{
				{
					Title:  "Story",
					URL:    "https://example.com/story",
					Source: domain.SourceKeyOpenAIBlog,
					Topic:  domain.TopicKeyTechAIModels,
				},
			},
		},
		&failingSourceProvider{sourceKey: domain.SourceKeyHackerNews, err: context.DeadlineExceeded},
	)
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	generator := &stubBriefGenerator{artifact: domain.BriefArtifact{
		Headline:   "Daily AI Brief",
		TopSummary: "Top stories.",
		Sections: []domain.BriefSection{{
			Key:   "tech.ai.models",
			Title: "AI Models",
			Items: []domain.BriefItemDraft{{
				Title: "Story", Summary: "Summary", WhyItMatters: "Impact",
				URL: "https://example.com/story", Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
			}},
		}},
	}}
	issueRepo := &orchestratorIssueRepo{}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})
	runRepo := &orchestratorRunRepo{}
	orchestrator := NewGenerationOrchestrator(
		collector,
		pipeline,
		generator,
		publisher,
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5, Model: "test-model", PromptVersion: "v1"},
		nil,
	)

	outcome, err := orchestrator.Run(context.Background(), GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog, domain.SourceKeyHackerNews}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		RunID:        "run-1",
		IssueID:      "issue-1",
		Now:          time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !outcome.Degraded || outcome.FailureClass != "source_partial_failure" {
		t.Fatalf("expected degraded partial source failure, got %+v", outcome)
	}
	if outcome.Issue.Status != domain.IssueStatusReady {
		t.Fatalf("expected ready issue, got %q", outcome.Issue.Status)
	}
	if outcome.Run.Status != domain.GenerationRunStatusDegraded {
		t.Fatalf("expected degraded run, got %q", outcome.Run.Status)
	}
}

func TestGenerationOrchestratorRetriesFailedIssue(t *testing.T) {
	t.Parallel()

	registry := NewSourceRegistry(&stubSourceProvider{
		sourceKey: domain.SourceKeyOpenAIBlog,
		items: []domain.Candidate{{
			Title: "Story", URL: "https://example.com/story",
			Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
		}},
	})
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	generator := &stubBriefGenerator{artifact: domain.BriefArtifact{
		Headline: "Brief", TopSummary: "Summary",
		Sections: []domain.BriefSection{{
			Key: "tech.ai.models", Title: "AI Models",
			Items: []domain.BriefItemDraft{{
				Title: "Story", Summary: "S", WhyItMatters: "W",
				URL: "https://example.com/story", Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
			}},
		}},
	}}
	issueRepo := &orchestratorIssueRepo{
		issueByDate: domain.Issue{
			ID: "issue-1", UserID: "user-1", BriefDate: "2026-06-29", Status: domain.IssueStatusFailed,
		},
	}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})
	runRepo := &orchestratorRunRepo{}
	orchestrator := NewGenerationOrchestrator(
		collector, pipeline, generator, publisher,
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5},
		nil,
	)

	outcome, err := orchestrator.Run(context.Background(), GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeRetry,
		RunID:        "run-2",
		IssueID:      "issue-1",
		Now:          time.Now(),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Issue.Status != domain.IssueStatusReady {
		t.Fatalf("expected retry to publish ready issue, got %q", outcome.Issue.Status)
	}
}

func TestGenerationOrchestratorFailsWhenNoCandidatesRemain(t *testing.T) {
	t.Parallel()

	registry := NewSourceRegistry(&stubSourceProvider{sourceKey: domain.SourceKeyOpenAIBlog})
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	issueRepo := &orchestratorIssueRepo{}
	runRepo := &orchestratorRunRepo{}
	orchestrator := NewGenerationOrchestrator(
		collector,
		pipeline,
		&stubBriefGenerator{},
		NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
			return fn(ctx, issueRepo, &stubReadItemRepo{})
		}),
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5},
		nil,
	)

	outcome, err := orchestrator.Run(context.Background(), GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		RunID:        "run-1",
		IssueID:      "issue-1",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Run.Status != domain.GenerationRunStatusFailed {
		t.Fatalf("expected failed run, got %q", outcome.Run.Status)
	}
	if outcome.FailureClass != "source_total_failure" {
		t.Fatalf("expected source_total_failure, got %q", outcome.FailureClass)
	}
}

func TestGenerationOrchestratorMarksFailureAfterGenerationContextCancellation(t *testing.T) {
	t.Parallel()

	registry := NewSourceRegistry(&stubSourceProvider{
		sourceKey: domain.SourceKeyOpenAIBlog,
		items: []domain.Candidate{{
			Title: "Story", URL: "https://example.com/story",
			Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
		}},
	})
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	issueRepo := &cancelAwareOrchestratorIssueRepo{}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})
	runRepo := &cancelAwareOrchestratorRunRepo{}
	orchestrator := NewGenerationOrchestrator(
		collector,
		pipeline,
		&cancelingBriefGenerator{cancel: cancel, err: context.DeadlineExceeded},
		publisher,
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5},
		nil,
	)

	outcome, err := orchestrator.Run(ctx, GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		RunID:        "run-canceled",
		IssueID:      "issue-canceled",
		Now:          time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.FailureClass != "generation_failure" {
		t.Fatalf("expected generation_failure, got %q", outcome.FailureClass)
	}
	if outcome.Run.Status != domain.GenerationRunStatusFailed {
		t.Fatalf("expected failed run, got %q", outcome.Run.Status)
	}
	if outcome.Issue.Status != domain.IssueStatusFailed {
		t.Fatalf("expected failed issue, got %q", outcome.Issue.Status)
	}
}

func TestGenerationOrchestratorFinalizesFailureWithoutReloadingRunOrIssue(t *testing.T) {
	t.Parallel()

	registry := NewSourceRegistry(&stubSourceProvider{
		sourceKey: domain.SourceKeyOpenAIBlog,
		items: []domain.Candidate{{
			Title: "Story", URL: "https://example.com/story",
			Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
		}},
	})
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	issueRepo := &updateOnlyOrchestratorIssueRepo{}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})
	runRepo := &updateOnlyOrchestratorRunRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	orchestrator := NewGenerationOrchestrator(
		collector,
		pipeline,
		&cancelingBriefGenerator{cancel: cancel, err: context.DeadlineExceeded},
		publisher,
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5},
		nil,
	)

	outcome, err := orchestrator.Run(ctx, GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		RunID:        "run-update-only",
		IssueID:      "issue-update-only",
		Now:          time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.Run.Status != domain.GenerationRunStatusFailed {
		t.Fatalf("expected failed run, got %q", outcome.Run.Status)
	}
	if outcome.Issue.Status != domain.IssueStatusFailed {
		t.Fatalf("expected failed issue, got %q", outcome.Issue.Status)
	}
}

func TestGenerationOrchestratorRecordsMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService()
	registry := NewSourceRegistry(&stubSourceProvider{
		sourceKey: domain.SourceKeyOpenAIBlog,
		items: []domain.Candidate{{
			Title: "Story", URL: "https://example.com/story",
			Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
		}},
	})
	collector := NewSourceCollector(registry, NewHTTPClient(nil))
	pipeline := NewCandidatePipeline(config.DailyBriefGenerationConfig{MaxCandidates: 5})
	generator := &stubBriefGenerator{artifact: domain.BriefArtifact{
		Headline: "Brief", TopSummary: "Summary",
		Sections: []domain.BriefSection{{
			Key: "tech.ai.models", Title: "AI Models",
			Items: []domain.BriefItemDraft{{
				Title: "Story", Summary: "S", WhyItMatters: "W",
				URL: "https://example.com/story", Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels,
			}},
		}},
	}}
	issueRepo := &orchestratorIssueRepo{}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})
	runRepo := &orchestratorRunRepo{}
	orchestrator := NewGenerationOrchestrator(
		collector, pipeline, generator, publisher,
		NewIssueService(issueRepo),
		NewGenerationRunService(runRepo),
		config.DailyBriefGenerationConfig{MaxItems: 5},
		metrics,
	)

	_, err := orchestrator.Run(context.Background(), GenerationRequest{
		Subscription: domain.NewSubscription("user-1", "UTC", "08:00", []string{domain.TopicKeyTechAIModels}, []string{domain.SourceKeyOpenAIBlog}),
		BriefDate:    "2026-06-29",
		TriggerType:  domain.GenerationRunTriggerTypeScheduled,
		RunID:        "run-metrics",
		IssueID:      "issue-metrics",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.SuccessfulRuns != 1 || snapshot.CandidateCountBefore != 1 || snapshot.CandidateCountAfter != 1 || snapshot.FinalItemCount != 1 {
		t.Fatalf("unexpected metrics snapshot: %+v", snapshot)
	}
}

func TestMarshalSourceStatsIncludesFailures(t *testing.T) {
	t.Parallel()

	stats := marshalSourceStats(SourceCollectResult{
		Failures: map[string]error{domain.SourceKeyHackerNews: context.Canceled},
		Candidates: []domain.Candidate{{
			Source: domain.SourceKeyOpenAIBlog,
		}},
	}, 1, 1)
	var payload map[string]any
	if err := json.Unmarshal([]byte(stats), &payload); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if payload["selectedCount"].(float64) != 1 {
		t.Fatalf("unexpected selected count: %+v", payload)
	}
}
