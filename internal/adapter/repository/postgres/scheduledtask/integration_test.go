package scheduledtask_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/scheduledtask/domain"
)

func TestScheduledTaskDatabaseLifecycle(t *testing.T) {
	dsn := os.Getenv("SCHEDULED_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("SCHEDULED_TASK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := storepkg.NewStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(2 * time.Minute)
	draft, err := store.CreateDraft(ctx, "test-user", "", "", 0, storepkg.ProposedConfig{
		Name:   "官方公告追踪",
		Prompt: "Report the confirmed event", Schedule: domain.Schedule{Kind: domain.ScheduleOnce, Timezone: "UTC", At: due},
		ReportMode: domain.ReportOnCondition, ConditionKind: domain.ConditionEvent,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	task, version, err := store.ConfirmDraft(ctx, "test-user", draft.ID, now, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Delete(ctx, task.UserID, task.ID); err != nil {
			t.Error(err)
		}
	})
	again, _, err := store.ConfirmDraft(ctx, "test-user", draft.ID, now, false)
	if err != nil || again.ID != task.ID {
		t.Fatalf("confirmation is not idempotent: %v", err)
	}
	if version.Number != 1 || task.ConversationID != "" {
		t.Fatalf("unexpected task: %+v", task)
	}
	_, savedVersion, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || savedVersion.Name != "官方公告追踪" {
		t.Fatalf("task name was not persisted: %+v, %v", savedVersion, err)
	}
	opts := storepkg.ClaimOptions{Limit: 5, WorkerID: "test", Lease: 15 * time.Minute, MaxAttempts: 3,
		OnceDeadline: 10 * time.Minute, RecurringDeadline: 30 * time.Minute}
	claims, err := store.ClaimDue(ctx, due, opts)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim: %v, %+v", err, claims)
	}
	if claims[0].Task.ID != task.ID {
		t.Fatalf("wrong claim: %+v", claims[0])
	}
	duplicateClaims, err := store.ClaimDue(ctx, due, opts)
	if err != nil || len(duplicateClaims) != 0 {
		t.Fatalf("duplicate claim: %v, %+v", err, duplicateClaims)
	}
	attemptID, err := store.StartAttempt(ctx, claims[0], due)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domain.Outcome{Signal: domain.SignalReport, Body: "Confirmed update", Sources: []string{"https://example.org/official"}}
	if err := store.FinishAttempt(ctx, storepkg.FinishInput{Claim: claims[0], AttemptID: attemptID,
		RuntimeSessionID: task.ID + "session", Outcome: &outcome, Now: due.Add(time.Second), MaxAttempts: 3,
		RetryDelay: time.Minute}); err != nil {
		t.Fatal(err)
	}
	published, err := store.PublishReport(ctx, storepkg.PublishReportInput{TaskID: task.ID,
		UserID: task.UserID, Version: 1, OccurrenceID: claims[0].OccurrenceID, Body: outcome.Body, Sources: outcome.Sources, Now: due.Add(2 * time.Second)})
	if err != nil || published.MessageID == "" || published.ConversationID == "" {
		t.Fatalf("publish: %v, %+v", err, published)
	}
	var sourceURL string
	if err := db.Raw(`SELECT sources->0->>'url' FROM t_message WHERE id = ?`, published.MessageID).Scan(&sourceURL).Error; err != nil || sourceURL != outcome.Sources[0] {
		t.Fatalf("message sources: %q, %v", sourceURL, err)
	}
	if err := db.Exec(`INSERT INTO t_runtime_task_session (id, task_type, task_id, user_id, status)
		VALUES (?, 'scheduled', ?, ?, 'completed')`, task.ID+"session", attemptID, task.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO t_runtime_task_journal (id, runtime_task_session_id, sequence, event_type, tool_name, tool_state, evidence_json)
		VALUES (?, ?, 1, 'tool_settled', 'web_fetch', 'completed', '[]')`, task.ID+"journal", task.ID+"session").Error; err != nil {
		t.Fatal(err)
	}
	runDetail, err := store.GetRun(ctx, task.UserID, task.ID, claims[0].OccurrenceID)
	if err != nil || runDetail.Run.ID != claims[0].OccurrenceID || len(runDetail.Outcome) == 0 || len(runDetail.Attempts) != 1 {
		t.Fatalf("run detail: %+v, %v", runDetail, err)
	}
	if len(runDetail.Attempts[0].Tools) != 1 || string(runDetail.Attempts[0].Tools[0].Evidence) != "[]" {
		t.Fatalf("tool details: %+v", runDetail.Attempts[0])
	}
	if runDetail.Version.Number != 1 || runDetail.Version.Prompt != version.Prompt {
		t.Fatalf("run lost immutable config: %+v", runDetail.Version)
	}
	if _, err := store.GetRun(ctx, "another-user", task.ID, claims[0].OccurrenceID); err == nil {
		t.Fatal("run detail leaked across users")
	}
	if _, err := store.LatestReport(ctx, "another-user", task.ID); err == nil {
		t.Fatal("latest report leaked across users")
	}
	// More than a page of silent checks must not hide the published report.
	for i := 1; i <= 101; i++ {
		at := due.Add(time.Duration(i) * time.Hour)
		if err := db.Exec(`INSERT INTO t_scheduled_task_occurrence
			(id, task_id, version, scheduled_at, deadline_at, status, result_signal)
			VALUES (?, ?, 1, ?, ?, 'no_report', 'no_report')`, fmt.Sprintf("%s-silent-%d", task.ID, i), task.ID, at, at.Add(time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
	}
	latest, err := store.LatestReport(ctx, task.UserID, task.ID)
	if err != nil || latest == nil || latest.Run.ID != claims[0].OccurrenceID {
		t.Fatalf("silent checks hid latest report: %+v, %v", latest, err)
	}
	second, err := store.PublishReport(ctx, storepkg.PublishReportInput{TaskID: task.ID,
		UserID: task.UserID, Version: 1, OccurrenceID: claims[0].OccurrenceID, Body: outcome.Body, Now: due.Add(3 * time.Second)})
	if err != nil || !second.AlreadySent || second.MessageID != published.MessageID {
		t.Fatalf("duplicate publish: %v, %+v", err, second)
	}
	unread, err := store.ListUnread(ctx, task.UserID)
	currentUnread := 0
	for _, item := range unread {
		if item.ConversationID == published.ConversationID {
			currentUnread = item.UnreadCount
		}
	}
	if err != nil || currentUnread != 1 {
		t.Fatalf("unread: %v, %+v", err, unread)
	}
	if err := store.MarkRead(ctx, task.UserID, published.ConversationID); err != nil {
		t.Fatal(err)
	}
	unread, err = store.ListUnread(ctx, task.UserID)
	currentUnread = 0
	for _, item := range unread {
		if item.ConversationID == published.ConversationID {
			currentUnread = item.UnreadCount
		}
	}
	if err != nil || currentUnread != 0 {
		t.Fatalf("mark read: %v, %+v", err, unread)
	}
	loaded, _, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || loaded.Status != domain.TaskCompleted {
		t.Fatalf("completion: %v, %+v", err, loaded)
	}
	deleteTx := postgresrag.NewConversationDeleteTransaction(db)
	if err := deleteTx(ctx, task.UserID, published.ConversationID, func(txCtx context.Context,
		conversationRepo port.ConversationRepository, messageRepo port.ConversationMessageRepository,
		summaryRepo port.ConversationSummaryRepository) error {
		if err := conversationRepo.Delete(txCtx, published.ConversationID); err != nil {
			return err
		}
		if err := messageRepo.DeleteByConversationIDAndUserID(txCtx, published.ConversationID, task.UserID); err != nil {
			return err
		}
		return summaryRepo.DeleteByConversationIDAndUserID(txCtx, published.ConversationID, task.UserID)
	}); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = store.Get(ctx, task.UserID, task.ID)
	if err != nil || loaded.Status != domain.TaskPaused || loaded.ConversationID != "" {
		t.Fatalf("deleting report conversation did not pause task: %v, %+v", err, loaded)
	}
	latest, err = store.LatestReport(ctx, task.UserID, task.ID)
	if err != nil || latest == nil || latest.Run.ID != claims[0].OccurrenceID {
		t.Fatalf("conversation deletion lost report history: %+v, %v", latest, err)
	}
	duplicateDraft, err := store.CreateDraft(ctx, task.UserID, "", "", 0, storepkg.ProposedConfig{
		Prompt: "Report the confirmed event", Schedule: domain.Schedule{Kind: domain.ScheduleOnce, Timezone: "UTC", At: due},
		ReportMode: domain.ReportOnCondition, ConditionKind: domain.ConditionEvent,
	}, now)
	if err != nil || duplicateDraft.DuplicateTaskID != task.ID {
		t.Fatalf("duplicate not surfaced: %v, %+v", err, duplicateDraft)
	}
	if _, _, err := store.ConfirmDraft(ctx, task.UserID, duplicateDraft.ID, now, false); err == nil {
		t.Fatal("duplicate confirmation should require explicit override")
	}
	separate, _, err := store.ConfirmDraft(ctx, task.UserID, duplicateDraft.ID, now, true)
	if err != nil || separate.ID == task.ID {
		t.Fatalf("explicit separate task: %v, %+v", err, separate)
	}
	t.Cleanup(func() {
		if err := store.Delete(ctx, separate.UserID, separate.ID); err != nil {
			t.Error(err)
		}
	})
}

func TestScheduledTaskFailureNotificationAndCompletedResume(t *testing.T) {
	dsn := os.Getenv("SCHEDULED_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("SCHEDULED_TASK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := storepkg.NewStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	draft, err := store.CreateDraft(ctx, "failure-user", "", "", 0, storepkg.ProposedConfig{
		Prompt: "Summarize official sports updates", Schedule: domain.Schedule{Kind: domain.ScheduleDaily, Timezone: "UTC", LocalTime: "09:00"}, ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := store.ConfirmDraft(ctx, "failure-user", draft.ID, now, false)
	if err != nil {
		t.Fatal(err)
	}
	idleVersion := domain.Version{TaskID: task.ID + "idle", Number: 1, Prompt: "Idle summary", Schedule: domain.Schedule{Kind: domain.ScheduleDaily, Timezone: "UTC", LocalTime: "09:00"}, ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone, ConfirmedAt: now}
	idleTask := domain.Task{ID: idleVersion.TaskID, UserID: task.UserID, CurrentVersion: 1, Status: domain.TaskActive, ConfirmedAt: now, NextDueAt: task.NextDueAt}
	if err := store.Create(ctx, idleTask, idleVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE t_scheduled_task SET create_time = ? WHERE id = ?`, now.Add(-24*time.Hour), idleTask.ID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Delete(ctx, idleTask.UserID, idleTask.ID); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() {
		if err := store.Delete(ctx, task.UserID, task.ID); err != nil {
			t.Error(err)
		}
	})
	for i, status := range []string{"failed", "uncertain", "failed"} {
		due := now.Add(time.Duration(i+1) * time.Minute)
		if err := db.Exec(`INSERT INTO t_scheduled_task_occurrence (id, task_id, version, scheduled_at, deadline_at, status)
			VALUES (?, ?, 1, ?, ?, ?)`, task.ID+fmt.Sprint(i), task.ID, due, due.Add(time.Hour), status).Error; err != nil {
			t.Fatal(err)
		}
	}
	opts := storepkg.FeedbackOptions{Daily: 7 * 24 * time.Hour, Weekly: 30 * 24 * time.Hour, Monthly: 60 * 24 * time.Hour, FailureThreshold: 3}
	count, err := store.PublishDueFeedback(ctx, now.Add(4*time.Minute), 1, opts)
	if err != nil || count != 0 {
		t.Fatalf("idle status scan: %d, %v", count, err)
	}
	count, err = store.PublishDueFeedback(ctx, now.Add(4*time.Minute+time.Second), 1, opts)
	if err != nil || count != 1 {
		t.Fatalf("failure notice: %d, %v", count, err)
	}
	count, err = store.PublishDueFeedback(ctx, now.Add(5*time.Minute), 100, opts)
	if err != nil || count != 0 {
		t.Fatalf("duplicate failure notice: %d, %v", count, err)
	}
	loaded, _, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || loaded.Status != domain.TaskActive || loaded.ConversationID == "" {
		t.Fatalf("notification state: %+v, %v", loaded, err)
	}
	if err := store.SetStatus(ctx, task.UserID, task.ID, domain.TaskCompleted, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Resume(ctx, task.UserID, task.ID, now.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	resumed, version, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || resumed.CurrentVersion != 2 || version.Number != 2 || resumed.ConversationID != loaded.ConversationID || resumed.Status != domain.TaskActive {
		t.Fatalf("new stage: %+v, %+v, %v", resumed, version, err)
	}
}

func TestConditionalTaskLowFrequencyFeedback(t *testing.T) {
	dsn := os.Getenv("SCHEDULED_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("SCHEDULED_TASK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := storepkg.NewStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	schedule := domain.Schedule{Kind: domain.ScheduleDaily, Timezone: "UTC", LocalTime: "09:00"}
	draft, err := store.CreateDraft(ctx, "feedback-user", "", "", 0, storepkg.ProposedConfig{
		Prompt: "Watch for an official update", Schedule: schedule,
		ReportMode: domain.ReportOnCondition, ConditionKind: domain.ConditionEvent,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := store.ConfirmDraft(ctx, "feedback-user", draft.ID, now, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Delete(ctx, task.UserID, task.ID); err != nil {
			t.Error(err)
		}
	})
	due := task.NextDueAt
	opts := storepkg.ClaimOptions{Limit: 5, WorkerID: "feedback-test", Lease: 15 * time.Minute,
		OnceDeadline: 10 * time.Minute, RecurringDeadline: 30 * time.Minute, MaxAttempts: 3}
	claims, err := store.ClaimDue(ctx, due, opts)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim: %v, %+v", err, claims)
	}
	attemptID, err := store.StartAttempt(ctx, claims[0], due)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domain.Outcome{Signal: domain.SignalNoReport}
	if err := store.FinishAttempt(ctx, storepkg.FinishInput{Claim: claims[0], AttemptID: attemptID,
		Outcome: &outcome, Now: due.Add(time.Second), MaxAttempts: 3, RetryDelay: time.Minute}); err != nil {
		t.Fatal(err)
	}
	feedbackAt := due.Add(8 * 24 * time.Hour)
	feedbackOpts := storepkg.FeedbackOptions{Daily: 7 * 24 * time.Hour, Weekly: 30 * 24 * time.Hour, Monthly: 60 * 24 * time.Hour}
	count, err := store.PublishDueFeedback(ctx, feedbackAt, 5, feedbackOpts)
	if err != nil || count != 1 {
		t.Fatalf("first feedback: %v, %d", err, count)
	}
	count, err = store.PublishDueFeedback(ctx, feedbackAt.Add(time.Minute), 5, feedbackOpts)
	if err != nil || count != 0 {
		t.Fatalf("duplicate feedback: %v, %d", err, count)
	}
	loaded, _, err := store.Get(ctx, task.UserID, task.ID)
	if err != nil || loaded.Status != domain.TaskActive || loaded.ConversationID == "" {
		t.Fatalf("feedback changed watch status: %v, %+v", err, loaded)
	}
	unread, err := store.ListUnread(ctx, task.UserID)
	currentUnread := 0
	for _, item := range unread {
		if item.ConversationID == loaded.ConversationID {
			currentUnread = item.UnreadCount
		}
	}
	if err != nil || currentUnread != 1 {
		t.Fatalf("feedback unread: %v, %+v", err, unread)
	}
}
