package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/distributedid"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const minCandidatesForPublish = 1
const finalizationTimeout = 5 * time.Second

type GenerationOrchestrator struct {
	sourceCollector      *SourceCollector
	candidatePipeline    *CandidatePipeline
	generator            port.BriefGenerator
	publisher            *Publisher
	issueService         *IssueService
	generationRunService *GenerationRunService
	generationCfg        config.DailyBriefGenerationConfig
	metrics              *MetricsService
}

func NewGenerationOrchestrator(
	sourceCollector *SourceCollector,
	candidatePipeline *CandidatePipeline,
	generator port.BriefGenerator,
	publisher *Publisher,
	issueService *IssueService,
	generationRunService *GenerationRunService,
	generationCfg config.DailyBriefGenerationConfig,
	metrics *MetricsService,
) *GenerationOrchestrator {
	return &GenerationOrchestrator{
		sourceCollector:      sourceCollector,
		candidatePipeline:    candidatePipeline,
		generator:            generator,
		publisher:            publisher,
		issueService:         issueService,
		generationRunService: generationRunService,
		generationCfg:        generationCfg,
		metrics:              metrics,
	}
}

type GenerationRequest struct {
	Subscription domain.Subscription
	BriefDate    string
	TriggerType  string
	RunID        string
	IssueID      string
	Now          time.Time
}

type GenerationOutcome struct {
	Issue        domain.Issue
	Run          domain.GenerationRun
	Degraded     bool
	FailureClass string
}

func (o *GenerationOrchestrator) Run(ctx context.Context, request GenerationRequest) (GenerationOutcome, error) {
	if strings.TrimSpace(request.Subscription.UserID) == "" {
		return GenerationOutcome{}, fmt.Errorf("generation request user id is required")
	}
	if strings.TrimSpace(request.BriefDate) == "" {
		return GenerationOutcome{}, fmt.Errorf("generation request brief date is required")
	}
	now := request.Now
	if now.IsZero() {
		now = time.Now()
	}

	runID, err := ensureID(request.RunID)
	if err != nil {
		return GenerationOutcome{}, err
	}
	issueID, err := ensureID(request.IssueID)
	if err != nil {
		return GenerationOutcome{}, err
	}

	run, err := domain.NewGenerationRun(runID, request.Subscription.UserID, request.BriefDate, request.TriggerType)
	if err != nil {
		return GenerationOutcome{}, err
	}
	run.Model = strings.TrimSpace(o.generationCfg.Model)
	run.PromptVersion = strings.TrimSpace(o.generationCfg.PromptVersion)
	run, err = o.generationRunService.Create(ctx, run)
	if err != nil {
		return GenerationOutcome{}, err
	}

	issue, err := o.ensureIssue(ctx, request, issueID, now)
	if err != nil {
		outcome, failErr := o.failRun(ctx, run, now, "publish_failure", fmt.Errorf("prepare issue: %w", err))
		recordRunObservability(o.metrics, request, outcome.Run, SourceCollectResult{}, 0, 0, 0)
		return outcome, failErr
	}

	collectResult := o.sourceCollector.Collect(ctx, request.Subscription.Sources)
	selected := o.candidatePipeline.Process(collectResult.Candidates, request.Subscription.Topics)
	candidateCount := len(collectResult.Candidates)
	selectedCount := len(selected)
	sourceStatsJSON := marshalSourceStats(collectResult, candidateCount, selectedCount)
	run.SourceStatsJSON = sourceStatsJSON

	if selectedCount < minCandidatesForPublish {
		outcome, failErr := o.failIssueAndRun(ctx, issue, run, now, "source_total_failure", "not enough valid candidates after filtering")
		recordRunObservability(o.metrics, request, outcome.Run, collectResult, candidateCount, selectedCount, 0)
		return outcome, failErr
	}

	generation, err := o.generator.Generate(ctx, port.BriefGenerationInput{
		UserID:        request.Subscription.UserID,
		RunID:         run.ID,
		BriefDate:     request.BriefDate,
		Topics:        request.Subscription.Topics,
		Candidates:    selected,
		PromptVersion: run.PromptVersion,
		Model:         run.Model,
	})
	if err != nil {
		outcome, failErr := o.failIssueAndRun(ctx, issue, run, now, "generation_failure", err.Error())
		recordRunObservability(o.metrics, request, outcome.Run, collectResult, candidateCount, selectedCount, 0)
		return outcome, failErr
	}

	run.Model = generation.Model
	run.PromptVersion = generation.PromptVersion
	run.TokenUsageJSON = marshalTokenUsage(generation.TokenUsage)

	publishedIssue, _, err := o.publisher.Publish(ctx, PublishInput{
		Issue:          issue,
		Artifact:       generation.Output,
		PublishedRunID: run.ID,
		PublishedAt:    now,
	})
	if err != nil {
		outcome, failErr := o.failIssueAndRun(ctx, issue, run, now, "publish_failure", err.Error())
		recordRunObservability(o.metrics, request, outcome.Run, collectResult, candidateCount, selectedCount, 0)
		return outcome, failErr
	}

	degraded := len(collectResult.Failures) > 0
	finalizeCtx, cancel := finalizationContext(ctx)
	defer cancel()
	if degraded {
		if err := run.MarkDegraded(now); err != nil {
			return GenerationOutcome{}, err
		}
	} else {
		if err := run.MarkSucceeded(now); err != nil {
			return GenerationOutcome{}, err
		}
	}
	run, err = o.generationRunService.Update(finalizeCtx, run)
	if err != nil {
		return GenerationOutcome{}, err
	}

	failureClass := ""
	if degraded {
		failureClass = "source_partial_failure"
	}
	recordRunObservability(o.metrics, request, run, collectResult, candidateCount, selectedCount, publishedIssue.ItemCount)
	return GenerationOutcome{
		Issue:        publishedIssue,
		Run:          run,
		Degraded:     degraded,
		FailureClass: failureClass,
	}, nil
}

func (o *GenerationOrchestrator) ensureIssue(ctx context.Context, request GenerationRequest, issueID string, now time.Time) (domain.Issue, error) {
	existing, err := o.issueService.GetByUserIDAndBriefDate(ctx, request.Subscription.UserID, request.BriefDate)
	if err != nil {
		return domain.Issue{}, err
	}
	if strings.TrimSpace(existing.ID) != "" {
		switch existing.Status {
		case domain.IssueStatusReady:
			return domain.Issue{}, fmt.Errorf("issue already ready for %s on %s", request.Subscription.UserID, request.BriefDate)
		case domain.IssueStatusFailed:
			return o.issueService.MarkGenerating(ctx, existing.ID, now)
		case domain.IssueStatusGenerating:
			return existing, nil
		default:
			return existing, nil
		}
	}

	issue := domain.NewIssue(issueID, request.Subscription.UserID, request.BriefDate)
	return o.issueService.Create(ctx, issue)
}

func (o *GenerationOrchestrator) failIssueAndRun(
	ctx context.Context,
	issue domain.Issue,
	run domain.GenerationRun,
	now time.Time,
	failureClass string,
	message string,
) (GenerationOutcome, error) {
	finalizeCtx, cancel := finalizationContext(ctx)
	defer cancel()

	outcome, err := o.failRun(finalizeCtx, run, now, failureClass, fmt.Errorf("%s", message))
	if err != nil {
		return GenerationOutcome{}, err
	}
	if strings.TrimSpace(issue.ID) != "" && issue.Status == domain.IssueStatusGenerating {
		if markErr := issue.MarkFailed(now); markErr != nil {
			return GenerationOutcome{}, markErr
		}
		updated, markErr := o.issueService.Update(finalizeCtx, issue)
		if markErr != nil {
			return GenerationOutcome{}, markErr
		}
		outcome.Issue = updated
	}
	outcome.FailureClass = failureClass
	return outcome, nil
}

func (o *GenerationOrchestrator) failRun(
	ctx context.Context,
	run domain.GenerationRun,
	now time.Time,
	failureClass string,
	err error,
) (GenerationOutcome, error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = failureClass
	}
	if updateErr := run.MarkFailed(now, message); updateErr != nil {
		return GenerationOutcome{}, updateErr
	}
	updatedRun, updateErr := o.generationRunService.Update(ctx, run)
	if updateErr != nil {
		return GenerationOutcome{}, updateErr
	}
	return GenerationOutcome{
		Run:          updatedRun,
		FailureClass: failureClass,
	}, nil
}

func ensureID(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	id, err := distributedid.NextID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

func finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), finalizationTimeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), finalizationTimeout)
}

func marshalSourceStats(result SourceCollectResult, candidateCount int, selectedCount int) string {
	sources := make(map[string]map[string]any, len(result.Failures)+candidateCount)
	for sourceKey, fetchErr := range result.Failures {
		sources[sourceKey] = map[string]any{
			"status": "failed",
			"error":  fetchErr.Error(),
		}
	}
	for _, candidate := range result.Candidates {
		entry, ok := sources[candidate.Source]
		if !ok {
			entry = map[string]any{"status": "ok", "count": 0}
		}
		if entry["status"] != "failed" {
			entry["status"] = "ok"
			count, _ := entry["count"].(int)
			entry["count"] = count + 1
		}
		sources[candidate.Source] = entry
	}
	payload := map[string]any{
		"sources":        sources,
		"candidateCount": candidateCount,
		"selectedCount":  selectedCount,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func marshalTokenUsage(usage aichat.TokenUsage) string {
	encoded, err := json.Marshal(usage.Normalized())
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
