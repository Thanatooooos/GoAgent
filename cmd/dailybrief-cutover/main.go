// Explicit handoff only; never starts workers or applies database migrations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	briefrepo "local/rag-project/internal/adapter/repository/postgres/dailybrief"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/framework/config"
)

func main() {
	user := flag.String("user", "", "subscription user ID (one user per handoff)")
	database := flag.String("database", "", "required expected database name")
	apply := flag.Bool("apply", false, "apply handoff; default is read-only inspection")
	missedDate := flag.String("acknowledge-missed-date", "", "explicitly retain an expired local date as missed (YYYY-MM-DD); never generates or deletes its legacy issue")
	flag.Parse()
	if err := run(strings.TrimSpace(*user), strings.TrimSpace(*database), *apply, strings.TrimSpace(*missedDate)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(user, expected string, apply bool, missedDate string) error {
	if user == "" || expected == "" {
		return fmt.Errorf("-user and -database are required")
	}
	if err := config.LoadConfig(""); err != nil {
		return err
	}
	cfg := config.Get()
	db, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var actual string
	if err := db.Raw(`SELECT current_database()`).Scan(&actual).Error; err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("database mismatch: expected %q, actual %q", expected, actual)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if !apply {
		sub, err := briefrepo.NewSubscriptionRepository(db).GetByUserID(ctx, user)
		if err != nil {
			return err
		}
		if sub.UserID == "" {
			return fmt.Errorf("subscription not found")
		}
		var taskID string
		if err := db.Raw(`SELECT task_id FROM t_daily_brief_task_binding WHERE user_id=?`, user).Scan(&taskID).Error; err != nil {
			return err
		}
		fmt.Printf("database=%s user=%s enabled=%t timezone=%s delivery=%s topics=%v sources=%v task=%s legacyLeaseUntil=%v\n", actual, user, sub.Enabled, sub.Timezone, sub.DeliveryTimeLocal, sub.Topics, sub.Sources, taskID, sub.LockUntil)
		if missedDate != "" {
			fmt.Printf("requestedMissedDate=%s (read-only; no handoff applied)\n", missedDate)
		}
		return nil
	}
	store := storepkg.NewStore(db)
	window := time.Duration(cfg.ScheduledTask.RecurringDeadlineSeconds) * time.Second
	var id string
	if missedDate != "" {
		id, err = store.CutoverDailyBriefWithMissedDate(ctx, user, time.Now(), cfg.DailyBrief.Generation, window, missedDate)
	} else {
		id, err = store.CutoverDailyBrief(ctx, user, time.Now(), cfg.DailyBrief.Generation, window)
	}
	if err != nil {
		return err
	}
	fmt.Printf("database=%s user=%s scheduledTask=%s\n", actual, user, id)
	if missedDate != "" {
		fmt.Printf("acknowledgedMissedDate=%s; legacy issue retained\n", missedDate)
	}
	return nil
}
