package domain_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestNewIssueInitializesGeneratingState(t *testing.T) {
	before := time.Now()
	issue := domain.NewIssue("issue-1", "user-1", "2026-06-29")
	after := time.Now()

	if issue.ID != "issue-1" {
		t.Fatalf("expected issue id to be preserved, got %q", issue.ID)
	}
	if issue.UserID != "user-1" {
		t.Fatalf("expected user id to be preserved, got %q", issue.UserID)
	}
	if issue.BriefDate != "2026-06-29" {
		t.Fatalf("expected brief date to be preserved, got %q", issue.BriefDate)
	}
	if issue.Status != domain.IssueStatusGenerating {
		t.Fatalf("expected status %q, got %q", domain.IssueStatusGenerating, issue.Status)
	}
	if issue.ItemCount != 0 {
		t.Fatalf("expected item count to default to zero, got %d", issue.ItemCount)
	}
	if issue.PublishedRunID != "" {
		t.Fatalf("expected empty published run id, got %q", issue.PublishedRunID)
	}
	if issue.GeneratedAt != nil {
		t.Fatal("expected generated at to be nil by default")
	}
	if issue.PublishedAt != nil {
		t.Fatal("expected published at to be nil by default")
	}
	assertTimeBetween(t, issue.CreatedAt, before, after, "issue created at")
	assertTimeBetween(t, issue.UpdatedAt, before, after, "issue updated at")
	if !issue.CreatedAt.Equal(issue.UpdatedAt) {
		t.Fatalf("expected created at and updated at to match, got %v and %v", issue.CreatedAt, issue.UpdatedAt)
	}
}

func TestIssueMarkReadyTransitionsFromGenerating(t *testing.T) {
	issue := domain.NewIssue("issue-1", "user-1", "2026-06-29")
	generatedAt := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	if err := issue.MarkReady(generatedAt); err != nil {
		t.Fatalf("expected generating issue to transition to ready, got error %v", err)
	}

	if issue.Status != domain.IssueStatusReady {
		t.Fatalf("expected status %q, got %q", domain.IssueStatusReady, issue.Status)
	}
	if issue.GeneratedAt == nil {
		t.Fatal("expected generated at to be set when issue becomes ready")
	}
	if !issue.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("expected generated at %v, got %v", generatedAt, issue.GeneratedAt)
	}
	if !issue.UpdatedAt.Equal(generatedAt) {
		t.Fatalf("expected updated at %v, got %v", generatedAt, issue.UpdatedAt)
	}
	if issue.PublishedAt == nil {
		t.Fatal("expected published at to be set when issue becomes ready")
	}
	if !issue.PublishedAt.Equal(generatedAt) {
		t.Fatalf("expected published at %v, got %v", generatedAt, issue.PublishedAt)
	}
}

func TestIssueRetryTransitionsFailedToGeneratingToReady(t *testing.T) {
	issue := domain.NewIssue("issue-1", "user-1", "2026-06-29")
	failedAt := time.Date(2026, 6, 29, 9, 15, 0, 0, time.UTC)
	retryAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	readyAt := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)

	if err := issue.MarkFailed(failedAt); err != nil {
		t.Fatalf("expected generating issue to transition to failed, got error %v", err)
	}
	if err := issue.MarkGenerating(retryAt); err != nil {
		t.Fatalf("expected failed issue to transition to generating for retry, got error %v", err)
	}
	if issue.Status != domain.IssueStatusGenerating {
		t.Fatalf("expected status %q, got %q", domain.IssueStatusGenerating, issue.Status)
	}
	if err := issue.MarkReady(readyAt); err != nil {
		t.Fatalf("expected generating issue to transition to ready after retry, got error %v", err)
	}
	if issue.Status != domain.IssueStatusReady {
		t.Fatalf("expected status %q, got %q", domain.IssueStatusReady, issue.Status)
	}
}

func TestIssueRejectsReadyTransitionFromFailedWithoutGenerating(t *testing.T) {
	issue := domain.NewIssue("issue-1", "user-1", "2026-06-29")
	failedAt := time.Date(2026, 6, 29, 9, 15, 0, 0, time.UTC)

	if err := issue.MarkFailed(failedAt); err != nil {
		t.Fatalf("expected generating issue to transition to failed, got error %v", err)
	}
	if err := issue.MarkReady(time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected failed issue to require generating before ready")
	}
}

func TestNewItemInitializesTimestamps(t *testing.T) {
	before := time.Now()
	item := domain.NewItem("item-1", "issue-1", "top-stories", 1, "LLM release")
	after := time.Now()

	if item.ID != "item-1" {
		t.Fatalf("expected item id to be preserved, got %q", item.ID)
	}
	if item.IssueID != "issue-1" {
		t.Fatalf("expected issue id to be preserved, got %q", item.IssueID)
	}
	if item.SectionKey != "top-stories" {
		t.Fatalf("expected section key to be preserved, got %q", item.SectionKey)
	}
	if item.Rank != 1 {
		t.Fatalf("expected rank to be preserved, got %d", item.Rank)
	}
	if item.Title != "LLM release" {
		t.Fatalf("expected title to be preserved, got %q", item.Title)
	}
	if item.PublishedAt != nil {
		t.Fatal("expected published at to be nil by default")
	}
	assertTimeBetween(t, item.CreatedAt, before, after, "item created at")
	assertTimeBetween(t, item.UpdatedAt, before, after, "item updated at")
	if !item.CreatedAt.Equal(item.UpdatedAt) {
		t.Fatalf("expected created at and updated at to match, got %v and %v", item.CreatedAt, item.UpdatedAt)
	}
}

func TestGenerationRunTriggerTypeValidatorAcceptsOnlyKnownValues(t *testing.T) {
	valid := []string{
		domain.GenerationRunTriggerTypeScheduled,
		domain.GenerationRunTriggerTypeRetry,
	}

	for _, triggerType := range valid {
		if !domain.IsValidGenerationRunTriggerType(triggerType) {
			t.Fatalf("expected trigger type %q to be valid", triggerType)
		}
	}

	if domain.IsValidGenerationRunTriggerType("manual") {
		t.Fatal("expected unknown trigger type to be rejected")
	}
}

func TestNewGenerationRunInitializesRunningStateAndRejectsUnknownTriggerType(t *testing.T) {
	before := time.Now()
	run, err := domain.NewGenerationRun("run-1", "user-1", "2026-06-29", domain.GenerationRunTriggerTypeScheduled)
	after := time.Now()
	if err != nil {
		t.Fatalf("expected valid trigger type to succeed, got error %v", err)
	}

	if run.ID != "run-1" {
		t.Fatalf("expected run id to be preserved, got %q", run.ID)
	}
	if run.UserID != "user-1" {
		t.Fatalf("expected user id to be preserved, got %q", run.UserID)
	}
	if run.BriefDate != "2026-06-29" {
		t.Fatalf("expected brief date to be preserved, got %q", run.BriefDate)
	}
	if run.TriggerType != domain.GenerationRunTriggerTypeScheduled {
		t.Fatalf("expected trigger type %q, got %q", domain.GenerationRunTriggerTypeScheduled, run.TriggerType)
	}
	if run.Status != domain.GenerationRunStatusRunning {
		t.Fatalf("expected status %q, got %q", domain.GenerationRunStatusRunning, run.Status)
	}
	if run.FinishedAt != nil {
		t.Fatal("expected finished at to be nil by default")
	}
	assertTimeBetween(t, run.StartedAt, before, after, "run started at")
	assertTimeBetween(t, run.CreatedAt, before, after, "run created at")
	assertTimeBetween(t, run.UpdatedAt, before, after, "run updated at")
	if !run.StartedAt.Equal(run.CreatedAt) || !run.CreatedAt.Equal(run.UpdatedAt) {
		t.Fatalf("expected started, created, and updated timestamps to match, got %v, %v, and %v", run.StartedAt, run.CreatedAt, run.UpdatedAt)
	}

	if _, err := domain.NewGenerationRun("run-2", "user-1", "2026-06-29", "manual"); err == nil {
		t.Fatal("expected unknown trigger type to return an error")
	}
}

func TestGenerationRunMarkSucceededTransitionsFromRunning(t *testing.T) {
	run, err := domain.NewGenerationRun("run-1", "user-1", "2026-06-29", domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("expected valid trigger type to succeed, got error %v", err)
	}
	finishedAt := time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC)

	if err := run.MarkSucceeded(finishedAt); err != nil {
		t.Fatalf("expected running generation run to transition to succeeded, got error %v", err)
	}

	if run.Status != domain.GenerationRunStatusSucceeded {
		t.Fatalf("expected status %q, got %q", domain.GenerationRunStatusSucceeded, run.Status)
	}
	if run.FinishedAt == nil {
		t.Fatal("expected finished at to be set when run succeeds")
	}
	if !run.FinishedAt.Equal(finishedAt) {
		t.Fatalf("expected finished at %v, got %v", finishedAt, run.FinishedAt)
	}
	if !run.UpdatedAt.Equal(finishedAt) {
		t.Fatalf("expected updated at %v, got %v", finishedAt, run.UpdatedAt)
	}
	if run.ErrorMessage != "" {
		t.Fatalf("expected empty error message on success, got %q", run.ErrorMessage)
	}
}

func TestGenerationRunMarkDegradedTransitionsFromRunning(t *testing.T) {
	run, err := domain.NewGenerationRun("run-2", "user-1", "2026-06-29", domain.GenerationRunTriggerTypeRetry)
	if err != nil {
		t.Fatalf("expected valid trigger type to succeed, got error %v", err)
	}
	finishedAt := time.Date(2026, 6, 29, 9, 45, 0, 0, time.UTC)

	if err := run.MarkDegraded(finishedAt); err != nil {
		t.Fatalf("expected running generation run to transition to degraded, got error %v", err)
	}

	if run.Status != domain.GenerationRunStatusDegraded {
		t.Fatalf("expected status %q, got %q", domain.GenerationRunStatusDegraded, run.Status)
	}
	if run.FinishedAt == nil || !run.FinishedAt.Equal(finishedAt) {
		t.Fatalf("expected finished at %v, got %v", finishedAt, run.FinishedAt)
	}
	if !run.UpdatedAt.Equal(finishedAt) {
		t.Fatalf("expected updated at %v, got %v", finishedAt, run.UpdatedAt)
	}
}

func TestGenerationRunRejectsInvalidTransitionAfterTerminalState(t *testing.T) {
	run, err := domain.NewGenerationRun("run-3", "user-1", "2026-06-29", domain.GenerationRunTriggerTypeScheduled)
	if err != nil {
		t.Fatalf("expected valid trigger type to succeed, got error %v", err)
	}
	finishedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)

	if err := run.MarkFailed(finishedAt, "source timeout"); err != nil {
		t.Fatalf("expected running generation run to transition to failed, got error %v", err)
	}

	if run.Status != domain.GenerationRunStatusFailed {
		t.Fatalf("expected status %q, got %q", domain.GenerationRunStatusFailed, run.Status)
	}
	if run.ErrorMessage != "source timeout" {
		t.Fatalf("expected error message to be preserved, got %q", run.ErrorMessage)
	}
	if err := run.MarkSucceeded(time.Date(2026, 6, 29, 10, 1, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected invalid generation run transition to be rejected")
	}
}
