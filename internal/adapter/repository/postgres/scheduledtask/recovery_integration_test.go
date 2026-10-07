package scheduledtask_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/scheduledtask/domain"
)

func TestConcurrentClaimAndSavedReportRecovery(t *testing.T) {
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
	s := storepkg.NewStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(time.Minute)
	draft, err := s.CreateDraft(ctx, "recovery-user", "", "", 0, storepkg.ProposedConfig{
		Prompt: "Report official results", Schedule: domain.Schedule{Kind: domain.ScheduleInterval, Timezone: "UTC", At: due, EverySeconds: 3600}, ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := s.ConfirmDraft(ctx, "recovery-user", draft.ID, now, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Delete(ctx, task.UserID, task.ID); err != nil {
			t.Error(err)
		}
	})
	opts := storepkg.ClaimOptions{Limit: 10, WorkerID: "worker", Lease: time.Minute, MaxAttempts: 3, OnceDeadline: 10 * time.Minute, RecurringDeadline: 10 * time.Minute}
	claims := make(chan storepkg.Claim, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.ClaimDue(ctx, due, opts)
			if err != nil {
				t.Error(err)
				return
			}
			for _, claim := range result {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	var claimed []storepkg.Claim
	for claim := range claims {
		claimed = append(claimed, claim)
	}
	if len(claimed) != 1 || claimed[0].Task.ID != task.ID {
		t.Fatalf("concurrent claim duplicated: %+v", claimed)
	}
	claim := claimed[0]
	attempt, err := s.StartAttempt(ctx, claim, due)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domain.Outcome{Signal: domain.SignalReport, Body: "Persisted before process restart"}
	if err := s.FinishAttempt(ctx, storepkg.FinishInput{Claim: claim, AttemptID: attempt, Outcome: &outcome, Now: due.Add(time.Second), MaxAttempts: 3, RetryDelay: time.Minute}); err != nil {
		t.Fatal(err)
	}
	// A new store represents a restarted worker. It must use the saved result,
	// with no second model attempt, and concurrent publishers must share one message.
	restarted := storepkg.NewStore(db)
	recovered, err := restarted.ClaimPending(ctx, due.Add(2*time.Minute), opts)
	if err != nil || len(recovered) != 1 || recovered[0].ReadyOutcome == nil {
		t.Fatalf("saved report recovery: %+v, %v", recovered, err)
	}
	ids := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := restarted.PublishReport(ctx, storepkg.PublishReportInput{TaskID: task.ID, UserID: task.UserID, Version: 1, OccurrenceID: claim.OccurrenceID, Body: outcome.Body, Now: due.Add(2 * time.Minute)})
			if err != nil {
				t.Error(err)
				return
			}
			ids <- result.MessageID
		}()
	}
	wg.Wait()
	close(ids)
	message := ""
	for id := range ids {
		if id == "" || (message != "" && message != id) {
			t.Fatalf("duplicate publication: %s, %s", message, id)
		}
		message = id
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM t_scheduled_task_attempt WHERE occurrence_id = ?`, claim.OccurrenceID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("recovery reran model: %d, %v", count, err)
	}
	// A saved result may not publish under a newer confirmed configuration.
	nextDue := due.Add(time.Hour)
	next, err := s.ClaimDue(ctx, nextDue, opts)
	if err != nil || len(next) != 1 {
		t.Fatalf("next occurrence: %+v, %v", next, err)
	}
	nextAttempt, err := s.StartAttempt(ctx, next[0], nextDue)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAttempt(ctx, storepkg.FinishInput{Claim: next[0], AttemptID: nextAttempt, Outcome: &outcome, Now: nextDue.Add(time.Second), MaxAttempts: 3, RetryDelay: time.Minute}); err != nil {
		t.Fatal(err)
	}
	edit, err := s.CreateDraft(ctx, task.UserID, "", task.ID, 1, storepkg.ProposedConfig{Prompt: "Report the revised official results", Schedule: domain.Schedule{Kind: domain.ScheduleDaily, Timezone: "UTC", LocalTime: "09:00"}, ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone}, nextDue.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConfirmDraft(ctx, task.UserID, edit.ID, nextDue.Add(2*time.Second), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishReport(ctx, storepkg.PublishReportInput{TaskID: task.ID, UserID: task.UserID, Version: 1, OccurrenceID: next[0].OccurrenceID, Body: outcome.Body, Now: nextDue.Add(3 * time.Second)}); err == nil {
		t.Fatal("old version report was published")
	}
	if pending, err := s.ClaimPending(ctx, nextDue.Add(4*time.Second), opts); err != nil || len(pending) != 0 {
		t.Fatalf("obsolete report reclaimed: %+v, %v", pending, err)
	}
	var obsoleteStatus string
	if err := db.Raw(`SELECT status FROM t_scheduled_task_occurrence WHERE id = ?`, next[0].OccurrenceID).Scan(&obsoleteStatus).Error; err != nil || obsoleteStatus != "superseded" {
		t.Fatalf("obsolete result status: %s, %v", obsoleteStatus, err)
	}
}
