package dailybrief

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	briefrepo "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/distributedid"
)

func TestScheduledOnlyRuntimeMapsExplicitSubscriptionAndSkipsLegacyGenerator(t *testing.T) {
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
		t.Fatal("isolated codex_ database required")
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	cfg := &config.Config{}
	cfg.DailyBrief.Generation.MaxItems = 5
	cfg.ScheduledTask.RecurringDeadlineSeconds = 1800
	r, err := NewRuntime(context.Background(), RuntimeOptions{DB: db, Config: cfg, DisableSchedule: true, UseScheduledTasks: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.ScheduleJob != nil || r.Orchestrator != nil || r.ReadService == nil || r.SubscriptionService == nil {
		t.Fatal("scheduled-only mode still assembled the legacy collector/retries or lost page services")
	}
	id, err := distributedid.NextID()
	if err != nil {
		t.Fatal(err)
	}
	user := fmt.Sprint(id)
	now := time.Now().UTC()
	// A new subscription created after today's local delivery starts tomorrow;
	// it does not fail as though a delivery predating creation had been missed.
	sub := domain.NewSubscription(user, "UTC", now.Add(-time.Minute).Format("15:04"), []string{domain.TopicKeyTechDev}, nil)
	if _, err := r.SubscriptionService.Upsert(context.Background(), sub); err != nil {
		t.Fatal(err)
	}
	var taskID string
	db.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, user).Scan(&taskID)
	if taskID == "" {
		t.Fatal("scheduled-only subscription has no owner")
	}
	t.Cleanup(func() {
		if err := storepkg.NewStore(db).Delete(context.Background(), user, taskID); err != nil {
			t.Error(err)
		}
	})
	task, v, err := storepkg.NewStore(db).Get(context.Background(), user, taskID)
	if err != nil || v.DailyBrief == nil || len(v.DailyBrief.Sources) == 0 || !task.NextDueAt.After(now) {
		t.Fatalf("new subscription: %+v %+v %v", task, v, err)
	}
	saved, err := briefrepo.NewSubscriptionRepository(db).GetByUserID(context.Background(), user)
	if err != nil || saved.Timezone != "UTC" || saved.DeliveryTimeLocal != sub.DeliveryTimeLocal {
		t.Fatalf("preferences changed: %+v %v", saved, err)
	}
	// Saving the same approved preference must not create another version/task.
	if _, err := r.SubscriptionService.Upsert(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	_, v, err = storepkg.NewStore(db).Get(context.Background(), user, taskID)
	if err != nil || v.Number != 1 {
		t.Fatalf("unchanged save created version: %+v %v", v, err)
	}
	// The preference UI sends an explicit false flag. GORM must not replace it
	// with the legacy enabled=1 default before the scheduled-task sync runs.
	saved.Enabled = false
	if _, err := r.SubscriptionService.Upsert(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	saved, err = briefrepo.NewSubscriptionRepository(db).GetByUserID(context.Background(), user)
	if err != nil || saved.Enabled {
		t.Fatalf("explicit unsubscribe was not persisted: enabled=%t err=%v", saved.Enabled, err)
	}
	task, v, err = storepkg.NewStore(db).Get(context.Background(), user, taskID)
	if err != nil || string(task.Status) != "paused" || !task.NextDueAt.IsZero() || v.Number != 1 {
		t.Fatalf("unsubscribe did not pause the existing task: %+v version=%d err=%v", task, v.Number, err)
	}
}
