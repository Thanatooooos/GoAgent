package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	briefrepo "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	briefdomain "local/rag-project/internal/app/dailybrief/domain"
	briefport "local/rag-project/internal/app/dailybrief/port"
	briefservice "local/rag-project/internal/app/dailybrief/service"
	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/distributedid"
)

const briefArtifact = `{"headline":"迁移简报","topSummary":"覆盖缺口说明","sections":[{"key":"tech.dev","title":"开发","items":[{"title":"官方更新","summary":"摘要","whyItMatters":"影响","url":"https://example.org/update","source":"官网","topic":"tech.dev"}]}]}`

type briefRuntimeStub struct {
	requests []runtime.TaskRequest
	answer   string
}

func (r *briefRuntimeStub) RunTask(_ context.Context, req runtime.TaskRequest) (runtime.RunResult, error) {
	r.requests = append(r.requests, req)
	if r.answer != "" {
		return runtime.RunResult{AssistantContent: r.answer, RuntimeSessionID: "brief-" + req.AttemptID}, nil
	}
	return runtime.RunResult{AssistantContent: `{"signal":"report","body":"separate body","artifact":` + briefArtifact + `}`, RuntimeSessionID: "brief-" + req.AttemptID}, nil
}

func briefDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("SCHEDULED_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("SCHEDULED_TASK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil || !strings.HasPrefix(name, "codex_") {
		t.Fatal("DailyBrief integration tests require a codex_ isolated database")
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func briefUser(t *testing.T, db *gorm.DB, now time.Time) (briefdomain.Subscription, *briefrepo.SubscriptionRepository) {
	t.Helper()
	id, err := distributedid.NextID()
	if err != nil {
		t.Fatal(err)
	}
	sub := briefdomain.NewSubscription(fmt.Sprint(id), "Asia/Shanghai", now.In(mustLocation()).Format("15:04"), []string{briefdomain.TopicKeyTechDev}, []string{briefdomain.SourceKeyHackerNews})
	sub.CreatedAt = now.Add(-24 * time.Hour)
	repo := briefrepo.NewSubscriptionRepository(db)
	repo.SyncScheduledTask = storepkg.SyncDailyBriefSubscription
	if _, err := repo.Upsert(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var task string
		db.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, sub.UserID).Scan(&task)
		if task != "" {
			if err := storepkg.NewStore(db).Delete(context.Background(), sub.UserID, task); err != nil {
				t.Error(err)
			}
		} else {
			db.Exec(`UPDATE t_daily_brief_subscription SET enabled=0 WHERE user_id=?`, sub.UserID)
		}
	})
	return sub, repo
}
func mustLocation() *time.Location { loc, _ := time.LoadLocation("Asia/Shanghai"); return loc }

func briefConfig() config.ScheduledTaskConfig {
	return config.ScheduledTaskConfig{ScanIntervalSeconds: 1, ClaimBatchSize: 100, MaxConcurrentRuns: 4, LeaseSeconds: 60, OnceDeadlineSeconds: 600, RecurringDeadlineSeconds: 1800, RetryMaxAttempts: 3, RetryDelaySeconds: 1, DailyFeedbackDays: 7, WeeklyFeedbackDays: 30, MonthlyFeedbackDays: 60, FailureThreshold: 3}
}

func cutover(t *testing.T, db *gorm.DB, sub briefdomain.Subscription, now time.Time) string {
	t.Helper()
	id, err := storepkg.NewStore(db).CutoverDailyBrief(context.Background(), sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func readyBrief(t *testing.T, db *gorm.DB, taskID, userID string, now time.Time) storepkg.Claim {
	t.Helper()
	store := storepkg.NewStore(db)
	claims, err := store.ClaimDue(context.Background(), now, storepkg.ClaimOptions{Limit: 100, WorkerID: "brief-test", Lease: time.Minute, OnceDeadline: 10 * time.Minute, RecurringDeadline: 30 * time.Minute, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	var claim storepkg.Claim
	for _, c := range claims {
		if c.Task.ID == taskID {
			claim = c
		}
	}
	if claim.OccurrenceID == "" {
		t.Fatal("missing DailyBrief occurrence")
	}
	a, err := store.StartAttempt(context.Background(), claim, now)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domain.Outcome{Signal: domain.SignalReport, Body: "caller body", Artifact: json.RawMessage(briefArtifact)}
	if err := store.FinishAttempt(context.Background(), storepkg.FinishInput{Claim: claim, AttemptID: a, Outcome: &outcome, Now: now, MaxAttempts: 3, RetryDelay: time.Second}); err != nil {
		t.Fatal(err)
	}
	return claim
}

func publishInput(c storepkg.Claim, now time.Time) storepkg.PublishReportInput {
	return storepkg.PublishReportInput{TaskID: c.Task.ID, UserID: c.Task.UserID, Version: c.Version.Number, OccurrenceID: c.OccurrenceID, Body: "different caller body", Now: now}
}

func TestDailyBriefWorkerHandoffAndViews(t *testing.T) {
	db := briefDB(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	sub, repo := briefUser(t, db, now)
	staleIssue := briefdomain.NewIssue(sub.UserID, sub.UserID, now.In(mustLocation()).Format("2006-01-02"))
	if _, err := briefrepo.NewIssueRepository(db).Create(ctx, staleIssue); err != nil {
		t.Fatal(err)
	}
	id := cutover(t, db, sub, now)
	store := storepkg.NewStore(db)
	task, version, err := store.Get(ctx, sub.UserID, id)
	if err != nil || task.ConversationID != "" || version.DailyBrief == nil || version.Schedule.Timezone != sub.Timezone {
		t.Fatalf("mapping: %+v %+v %v", task, version, err)
	}
	if again := cutover(t, db, sub, now); again != id {
		t.Fatal("handoff duplicated task")
	}
	acquired, err := repo.TryAcquireLock(ctx, briefdomain.SubscriptionLockLease{UserID: sub.UserID, LockOwner: "legacy"}, now.Add(time.Minute), now)
	if err != nil || acquired {
		t.Fatalf("legacy lease after handoff: %t %v", acquired, err)
	}
	legacy, err := repo.List(ctx, briefport.SubscriptionListFilter{LegacyOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range legacy {
		if s.UserID == sub.UserID {
			t.Fatal("migrated user consumes legacy scan batch")
		}
	}
	// A stale old binary is fenced even when it bypasses the new repository.
	raw := db.Exec(`UPDATE t_daily_brief_subscription SET lock_owner='stale',lock_until=? WHERE user_id=?`, now.Add(time.Minute), sub.UserID)
	if raw.Error != nil || raw.RowsAffected != 0 {
		t.Fatalf("old binary lock fence: %v %d", raw.Error, raw.RowsAffected)
	}
	if err := db.Exec(`INSERT INTO t_daily_brief_issue(id,user_id,brief_date,status,sections_json,item_count,create_time,update_time) VALUES (?,?,'2026-01-01','generating','[]',0,?,?)`, sub.UserID, sub.UserID, now, now).Error; err == nil {
		t.Fatal("legacy issue writer was not fenced")
	}
	staleIssue.PublishedRunID = "stale-old-run"
	staleIssue.MarkReady(now)
	if _, err := briefrepo.NewIssueRepository(db).Update(ctx, staleIssue); err == nil {
		t.Fatal("late legacy update published after handoff")
	}
	stub := &briefRuntimeStub{}
	worker, err := NewRuntime(db, stub, briefConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(stub.requests) != 1 || stub.requests[0].ScheduledTaskID != id || stub.requests[0].TaskType != "scheduled" || len(stub.requests[0].Sources) != 1 {
		t.Fatalf("formal worker request: %+v", stub.requests)
	}
	model, err := briefservice.NewReadService(repo, briefrepo.NewIssueRepository(db), briefrepo.NewItemRepository(db)).GetToday(ctx, sub.UserID, now)
	if err != nil || model.PageState != briefservice.PageStateReady || len(model.Items) != 1 || model.Issue.Headline != "迁移简报" {
		t.Fatalf("retained issue page: %+v %v", model, err)
	}
	if model.Issue.ID != staleIssue.ID {
		t.Fatal("handoff duplicated the existing date issue")
	}
	var message struct{ Content, Sources string }
	if err := db.Raw(`SELECT content,sources::text FROM t_message WHERE id=(SELECT published_message_id FROM t_scheduled_task_occurrence WHERE id=?)`, model.Issue.PublishedRunID).Scan(&message).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Content, model.Issue.Headline) || !strings.Contains(message.Content, model.Items[0].URL) || !strings.Contains(message.Sources, model.Items[0].URL) {
		t.Fatalf("views differ: %+v", message)
	}
	if err := worker.RunOnce(ctx, time.Now()); err != nil || len(stub.requests) != 1 {
		t.Fatalf("duplicate model invocation: %v", err)
	}
	var unread int
	db.Raw(`SELECT unread_count FROM t_conversation_unread WHERE user_id=?`, sub.UserID).Scan(&unread)
	if unread != 1 {
		t.Fatalf("unread=%d", unread)
	}
	// Generic editing preserves the page contract and syncs the old preference UI.
	draft, err := store.CreateDraft(ctx, sub.UserID, "", id, 1, storepkg.ProposedConfig{Name: version.Name, Prompt: "自定义简报内容", Schedule: domain.Schedule{Kind: domain.ScheduleDaily, Timezone: sub.Timezone, LocalTime: "23:58"}, ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone, AllowedToolIDs: version.AllowedToolIDs}, time.Now())
	if err != nil || draft.Config.DailyBrief == nil {
		t.Fatalf("edit contract: %+v %v", draft, err)
	}
	_, edited, err := store.ConfirmDraft(ctx, sub.UserID, draft.ID, time.Now(), true)
	if err != nil || edited.DailyBrief == nil {
		t.Fatalf("confirmed contract: %+v %v", edited, err)
	}
	saved, err := repo.GetByUserID(ctx, sub.UserID)
	if err != nil || saved.DeliveryTimeLocal != "23:58" {
		t.Fatalf("schedule desync: %+v %v", saved, err)
	}
	if err := store.SetStatus(ctx, sub.UserID, id, domain.TaskPaused, time.Time{}); err != nil {
		t.Fatal(err)
	}
	saved, _ = repo.GetByUserID(ctx, sub.UserID)
	if saved.Enabled {
		t.Fatal("pause did not sync subscription")
	}
	// Saving preferences also versions the task; does not overwrite its prompt.
	saved.Enabled = true
	saved.DeliveryTimeLocal = "23:59"
	if _, err := repo.Upsert(ctx, saved); err != nil {
		t.Fatal(err)
	}
	_, edited, err = store.Get(ctx, sub.UserID, id)
	if err != nil || edited.Number != 3 || edited.Prompt != "自定义简报内容" || edited.Schedule.LocalTime != "23:59" {
		t.Fatalf("subscription sync: %+v %v", edited, err)
	}
}

func TestDailyBriefUncertainKeepsPageFailureAndNoConversation(t *testing.T) {
	db := briefDB(t)
	now := time.Now().Truncate(time.Minute)
	sub, repo := briefUser(t, db, now)
	id := cutover(t, db, sub, now)
	worker, err := NewRuntime(db, &briefRuntimeStub{answer: `{"signal":"uncertain","reason":"配置来源暂不可用"}`}, briefConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	model, err := briefservice.NewReadService(repo, briefrepo.NewIssueRepository(db), briefrepo.NewItemRepository(db)).GetToday(context.Background(), sub.UserID, now)
	if err != nil || model.PageState != briefservice.PageStateFailed || len(model.Items) != 0 {
		t.Fatalf("uncertain page state: %+v %v", model, err)
	}
	task, _, err := worker.Store.Get(context.Background(), sub.UserID, id)
	if err != nil || task.ConversationID != "" {
		t.Fatalf("uncertain created conversation: %+v %v", task, err)
	}
	var n int
	db.Raw(`SELECT count(*) FROM t_daily_brief_issue WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 0 {
		t.Fatal("unpublished run created an artifact")
	}
}

func TestDailyBriefCutoverRefusesLeaseAndExpiredDay(t *testing.T) {
	db := briefDB(t)
	now := time.Date(2026, 10, 3, 1, 0, 0, 0, time.UTC)
	sub, repo := briefUser(t, db, now)
	ctx := context.Background()
	acquired, err := repo.TryAcquireLock(ctx, briefdomain.SubscriptionLockLease{UserID: sub.UserID, LockOwner: "old"}, now.Add(time.Hour), now)
	if err != nil || !acquired {
		t.Fatalf("legacy lease: %v", err)
	}
	store := storepkg.NewStore(db)
	if _, err := store.CutoverDailyBrief(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, time.Minute*30); err == nil {
		t.Fatal("active lease handoff accepted")
	}
	repo.ReleaseLock(ctx, briefdomain.SubscriptionLockLease{UserID: sub.UserID, LockOwner: "old"})
	if _, err := store.CutoverDailyBrief(ctx, sub.UserID, now.Add(time.Hour), config.DailyBriefGenerationConfig{MaxItems: 5}, time.Minute*30); err == nil {
		t.Fatal("expired undelivered day silently skipped")
	}
	var n int
	db.Raw(`SELECT count(*) FROM t_daily_brief_task_binding WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 0 {
		t.Fatal("rejected handoff changed owner")
	}
}

func TestDailyBriefLegacyPublicationSerializesWithCutover(t *testing.T) {
	db := briefDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 1, 0, 0, 0, time.UTC)
	sub, repo := briefUser(t, db, now)
	issue := briefdomain.NewIssue(sub.UserID, sub.UserID, "2026-10-03")
	legacy := db.Begin()
	if legacy.Error != nil {
		t.Fatal(legacy.Error)
	}
	defer legacy.Rollback()
	if _, err := briefrepo.NewIssueRepository(legacy).Create(ctx, issue); err != nil {
		t.Fatal(err)
	}
	var artifact briefdomain.BriefArtifact
	if err := json.Unmarshal([]byte(briefArtifact), &artifact); err != nil {
		t.Fatal(err)
	}
	if _, _, err := briefservice.NewPublisher(briefrepo.NewPublishTransaction(legacy)).Publish(ctx, briefservice.PublishInput{Issue: issue, Artifact: artifact, PublishedRunID: sub.UserID, PublishedAt: now}); err != nil {
		t.Fatal(err)
	}
	type handoff struct {
		id  string
		err error
	}
	done := make(chan handoff, 1)
	go func() {
		id, err := storepkg.NewStore(db).CutoverDailyBrief(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute)
		done <- handoff{id, err}
	}()
	select {
	case result := <-done:
		t.Fatalf("handoff did not wait for legacy publication: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	if err := legacy.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var result handoff
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handoff remained blocked")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	task, _, err := storepkg.NewStore(db).Get(ctx, sub.UserID, result.id)
	if err != nil || task.NextDueAt.In(mustLocation()).Format("2006-01-02") != "2026-10-04" || task.ConversationID != "" {
		t.Fatalf("legacy date replay: %+v %v", task, err)
	}
	if err := storepkg.NewStore(db).SetStatus(ctx, sub.UserID, result.id, domain.TaskPaused, time.Time{}); err != nil {
		t.Fatal(err)
	}
	model, err := briefservice.NewReadService(repo, briefrepo.NewIssueRepository(db), briefrepo.NewItemRepository(db)).GetByDate(ctx, sub.UserID, "2026-10-03")
	if err != nil || model.PageState != briefservice.PageStateReady || model.Issue.PublishedRunID != sub.UserID {
		t.Fatalf("legacy history was hidden/rewritten: %+v %v", model, err)
	}
}

func TestDailyBriefExplicitMissedDateHandoffRetainsFailureAndNextSchedule(t *testing.T) {
	db := briefDB(t)
	ctx := context.Background()
	due := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	now := due.Add(time.Hour)
	sub, _ := briefUser(t, db, due)
	issue := briefdomain.NewIssue(sub.UserID, sub.UserID, "2026-10-06")
	if err := issue.MarkFailed(due); err != nil {
		t.Fatal(err)
	}
	if _, err := briefrepo.NewIssueRepository(db).Create(ctx, issue); err != nil {
		t.Fatal(err)
	}
	store := storepkg.NewStore(db)
	for _, date := range []string{"", "2026-10-05", "invalid"} {
		var err error
		if date == "" {
			_, err = store.CutoverDailyBrief(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute)
		} else {
			_, err = store.CutoverDailyBriefWithMissedDate(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute, date)
		}
		if err == nil {
			t.Fatalf("expired date was skipped with acknowledgement %q", date)
		}
	}
	id, err := store.CutoverDailyBriefWithMissedDate(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute, "2026-10-06")
	if err != nil {
		t.Fatal(err)
	}
	task, version, err := store.Get(ctx, sub.UserID, id)
	if err != nil || !task.NextDueAt.Equal(due.AddDate(0, 0, 1)) || task.ConversationID != "" || version.Schedule.LocalTime != sub.DeliveryTimeLocal || version.Schedule.Timezone != sub.Timezone {
		t.Fatalf("acknowledged handoff changed schedule or created a conversation: %+v %+v %v", task, version, err)
	}
	if again, err := store.CutoverDailyBriefWithMissedDate(ctx, sub.UserID, now, config.DailyBriefGenerationConfig{MaxItems: 5}, 30*time.Minute, "2026-10-06"); err != nil || again != id {
		t.Fatalf("repeated acknowledgement duplicated handoff: %s %v", again, err)
	}
	runs, err := store.ListRuns(ctx, sub.UserID, id)
	if err != nil || len(runs) != 1 || runs[0].Status != "missed" || !runs[0].ScheduledAt.Equal(due) || runs[0].PublishedMessageID != nil {
		t.Fatalf("missed occurrence was not recorded once: %+v %v", runs, err)
	}
	saved, err := briefrepo.NewIssueRepository(db).GetByID(ctx, issue.ID)
	if err != nil || saved.Status != briefdomain.IssueStatusFailed || saved.PublishedRunID != "" {
		t.Fatalf("legacy failure was changed: %+v %v", saved, err)
	}
	var messages int64
	if err := db.Raw(`SELECT count(*) FROM t_message WHERE user_id=?`, sub.UserID).Scan(&messages).Error; err != nil || messages != 0 {
		t.Fatal("missed handoff sent a report")
	}
}

func TestDailyBriefChangedVersionAndDeletedConversationSuppressBothViews(t *testing.T) {
	db := briefDB(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	sub, repo := briefUser(t, db, now)
	id := cutover(t, db, sub, now)
	claim := readyBrief(t, db, id, sub.UserID, now)
	saved, _ := repo.GetByUserID(ctx, sub.UserID)
	saved.Topics = []string{briefdomain.TopicKeyTechAIModels}
	saved.Sources = briefdomain.SourceKeysForTopics(saved.Topics)
	if _, err := repo.Upsert(ctx, saved); err != nil {
		t.Fatal(err)
	}
	store := storepkg.NewStore(db)
	if _, err := store.PublishReport(ctx, publishInput(claim, now)); err == nil {
		t.Fatal("old configuration published")
	}
	var n int
	db.Raw(`SELECT count(*) FROM t_daily_brief_issue WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 0 {
		t.Fatal("old version projected issue")
	}
	// Restore topic through explicit subscription edit, then delete the target
	// conversation while a report is ready. Neither view may be published.
	saved.Topics = sub.Topics
	saved.Sources = sub.Sources
	if _, err := repo.Upsert(ctx, saved); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE t_scheduled_task SET next_due_at=? WHERE id=?`, now, id)
	claim = readyBrief(t, db, id, sub.UserID, now)
	db.Exec(`UPDATE t_scheduled_task SET conversation_id='deleted-target' WHERE id=?`, id)
	if _, err := store.PublishReport(ctx, publishInput(claim, now)); err == nil {
		t.Fatal("deleted conversation published")
	}
	db.Raw(`SELECT count(*) FROM t_daily_brief_issue WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 0 {
		t.Fatal("deleted conversation still projected issue")
	}
	saved, _ = repo.GetByUserID(ctx, sub.UserID)
	if saved.Enabled {
		t.Fatal("conversation deletion did not pause subscription")
	}
	// User deletion remains a tombstone; explicit re-enable gets a new task.
	if err := store.Delete(ctx, sub.UserID, id); err != nil {
		t.Fatal(err)
	}
	saved.Enabled = true
	if _, err := repo.Upsert(ctx, saved); err != nil {
		t.Fatal(err)
	}
	var replacement string
	db.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, sub.UserID).Scan(&replacement)
	if replacement == id || replacement == "" {
		t.Fatal("deleted task was resurrected instead of replaced")
	}
	if _, err := store.PublishReport(ctx, publishInput(claim, now)); err == nil {
		t.Fatal("replaced task published")
	}
}

func TestDailyBriefAtomicPublishRecoveryAndDateDedup(t *testing.T) {
	db := briefDB(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	sub, _ := briefUser(t, db, now)
	id := cutover(t, db, sub, now)
	claim := readyBrief(t, db, id, sub.UserID, now)
	// Fail message insertion AFTER the issue/items have been projected.
	if err := db.Exec(`CREATE OR REPLACE FUNCTION test_daily_brief_message_failure() RETURNS trigger AS $$
BEGIN
IF current_setting('app.test_fail_message',true)='on' THEN RAISE EXCEPTION 'injected message failure'; END IF;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER test_daily_brief_message_failure BEFORE INSERT ON t_message FOR EACH ROW EXECUTE FUNCTION test_daily_brief_message_failure();`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS test_daily_brief_message_failure ON t_message; DROP FUNCTION IF EXISTS test_daily_brief_message_failure();`)
	})
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT set_config('app.test_fail_message','on',true)`).Error; err != nil {
			return err
		}
		if _, err := storepkg.NewStore(tx).PublishReport(ctx, publishInput(claim, now)); err == nil {
			t.Fatal("message failure not injected")
		}
		var n int
		if err := tx.Raw(`SELECT count(*) FROM t_daily_brief_issue WHERE user_id=?`, sub.UserID).Scan(&n).Error; err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("partial issue survived failed message transaction")
		}
		if err := tx.Raw(`SELECT count(*) FROM t_conversation WHERE user_id=?`, sub.UserID).Scan(&n).Error; err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("empty conversation survived failure")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The formal worker recovers ready results without another model call.
	stub := &briefRuntimeStub{}
	worker, err := NewRuntime(db, stub, briefConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(ctx, time.Now()); err != nil || len(stub.requests) != 0 {
		t.Fatalf("recovery reran model: %v %+v", err, stub.requests)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := worker.Store.PublishReport(ctx, publishInput(claim, now))
			if err != nil || !result.AlreadySent {
				t.Errorf("idempotent recovery: %+v %v", result, err)
			}
		}()
	}
	wg.Wait()
	var n int
	db.Raw(`SELECT count(*) FROM t_message WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 1 {
		t.Fatalf("messages=%d", n)
	}
	// Same local date under a newly confirmed version cannot send twice.
	task, v, _ := worker.Store.Get(ctx, sub.UserID, id)
	v.Number++
	v.ConfirmedAt = now.Add(time.Second)
	if err := worker.Store.ReplaceVersion(ctx, sub.UserID, id, task.CurrentVersion, v, now); err != nil {
		t.Fatal(err)
	}
	second := readyBrief(t, db, id, sub.UserID, now)
	if _, err := worker.Store.PublishReport(ctx, publishInput(second, now)); err != nil {
		t.Fatal(err)
	}
	db.Raw(`SELECT count(*) FROM t_message WHERE user_id=?`, sub.UserID).Scan(&n)
	if n != 1 {
		t.Fatalf("date dedup failed: messages=%d", n)
	}
	var status string
	db.Raw(`SELECT status FROM t_scheduled_task_occurrence WHERE id=?`, second.OccurrenceID).Scan(&status)
	if status != "superseded" {
		t.Fatalf("duplicate date status=%s", status)
	}
}
