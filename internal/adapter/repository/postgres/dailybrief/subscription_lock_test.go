package dailybrief

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestSubscriptionRepositoryTryAcquireLockBuildsLockUntilGuard(t *testing.T) {
	t.Parallel()

	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db := newDryRunDB(t, recorder)
	repo := NewSubscriptionRepository(db)
	now := time.Date(2026, 6, 29, 6, 0, 0, 0, time.UTC)
	lockUntil := now.Add(15 * time.Minute)

	_, err := repo.TryAcquireLock(context.Background(), domain.SubscriptionLockLease{
		UserID:    "user-1",
		LockOwner: "worker-1",
	}, lockUntil, now)
	if err != nil {
		t.Fatalf("TryAcquireLock returned error: %v", err)
	}

	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "lock_until is null or lock_until <") {
		t.Fatalf("expected lock expiry guard in SQL, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "lock_owner") {
		t.Fatalf("expected lock_owner update in SQL, got %q", recorder.lastSQL)
	}
}

func TestSubscriptionRepositoryRenewAndReleaseScopeByOwnership(t *testing.T) {
	t.Parallel()

	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db := newDryRunDB(t, recorder)
	repo := NewSubscriptionRepository(db)
	lockUntil := time.Date(2026, 6, 29, 7, 0, 0, 0, time.UTC)
	lease := domain.SubscriptionLockLease{UserID: "user-1", LockOwner: "worker-2"}

	_, err := repo.RenewLock(context.Background(), lease, lockUntil)
	if err != nil {
		t.Fatalf("RenewLock returned error: %v", err)
	}
	renewSQL := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(renewSQL, "lock_owner") || !strings.Contains(renewSQL, "where") {
		t.Fatalf("expected ownership filter in renew SQL, got %q", recorder.lastSQL)
	}

	_, err = repo.ReleaseLock(context.Background(), lease)
	if err != nil {
		t.Fatalf("ReleaseLock returned error: %v", err)
	}
	releaseSQL := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(releaseSQL, "lock_owner") || !strings.Contains(releaseSQL, "lock_until") {
		t.Fatalf("expected lock fields to be cleared, got %q", recorder.lastSQL)
	}
}

func TestGenerationRunRepositoryCountRetryRunsScopesToRetryTrigger(t *testing.T) {
	t.Parallel()

	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db := newDryRunDB(t, recorder)
	repo := NewGenerationRunRepository(db)

	_, err := repo.CountRetryRunsByUserIDAndBriefDate(context.Background(), "user-1", "2026-06-29")
	if err != nil {
		t.Fatalf("CountRetryRunsByUserIDAndBriefDate returned error: %v", err)
	}

	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "trigger_type") || !strings.Contains(sql, "retry") {
		t.Fatalf("expected retry trigger filter in SQL, got %q", recorder.lastSQL)
	}
}

func TestGenerationRunRepositoryGetLatestFailedOrdersByNewestFailure(t *testing.T) {
	t.Parallel()

	recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
	db := newDryRunDB(t, recorder)
	repo := NewGenerationRunRepository(db)

	_, err := repo.GetLatestFailedByUserIDAndBriefDate(context.Background(), "user-1", "2026-06-29")
	if err != nil {
		t.Fatalf("GetLatestFailedByUserIDAndBriefDate returned error: %v", err)
	}

	sql := strings.ToLower(recorder.lastSQL)
	if !strings.Contains(sql, "order by finished_at desc") {
		t.Fatalf("expected latest failed run query to order by newest finished_at, got %q", recorder.lastSQL)
	}
	if !strings.Contains(sql, "status") || !strings.Contains(sql, "failed") {
		t.Fatalf("expected failed status filter in SQL, got %q", recorder.lastSQL)
	}
}

type gormTraceRecorder struct {
	logger.Interface
	lastSQL string
}

func (r *gormTraceRecorder) LogMode(level logger.LogLevel) logger.Interface {
	if r.Interface == nil {
		r.Interface = logger.Default
	}
	r.Interface = r.Interface.LogMode(level)
	return r
}

func (r *gormTraceRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.lastSQL = sql
	if r.Interface != nil {
		r.Interface.Trace(ctx, begin, fc, err)
	}
}

func newDryRunDB(t *testing.T, recorder logger.Interface) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 recorder,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return db
}
