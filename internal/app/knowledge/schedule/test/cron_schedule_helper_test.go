package schedule_test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/schedule"
)

func TestNextRunTimeReturnsNextMatchingSecond(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.April, 28, 10, 0, 7, 0, time.UTC)
	next, err := schedule.NextRunTime("*/15 * * * * *", from)
	if err != nil {
		t.Fatalf("NextRunTime() error = %v", err)
	}
	assertTime(t, next, time.Date(2026, time.April, 28, 10, 0, 15, 0, time.UTC))
}

func TestNextRunTimeSupportsFiveFieldCronAndAliases(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.April, 28, 10, 30, 0, 0, time.UTC)
	next, err := schedule.NextRunTime("0 9 ? MAY MON", from)
	if err != nil {
		t.Fatalf("NextRunTime() error = %v", err)
	}
	assertTime(t, next, time.Date(2026, time.May, 4, 9, 0, 0, 0, time.UTC))
}

func TestNextRunTimeReturnsNilForBlankCronOrZeroFrom(t *testing.T) {
	t.Parallel()

	next, err := schedule.NextRunTime(" ", time.Now())
	if err != nil {
		t.Fatalf("NextRunTime(blank) error = %v", err)
	}
	if next != nil {
		t.Fatalf("NextRunTime(blank) = %v, want nil", next)
	}

	next, err = schedule.NextRunTime("* * * * * *", time.Time{})
	if err != nil {
		t.Fatalf("NextRunTime(zero from) error = %v", err)
	}
	if next != nil {
		t.Fatalf("NextRunTime(zero from) = %v, want nil", next)
	}
}

func TestNextRunTimeRejectsInvalidCron(t *testing.T) {
	t.Parallel()

	if _, err := schedule.NextRunTime("0 0 25 * * *", time.Now()); err == nil {
		t.Fatal("NextRunTime() should reject invalid hour")
	}
}

func TestIsIntervalLessThan(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.April, 28, 10, 0, 0, 0, time.UTC)

	less, err := schedule.IsIntervalLessThan("*/30 * * * * *", from, 60)
	if err != nil {
		t.Fatalf("IsIntervalLessThan(30s) error = %v", err)
	}
	if !less {
		t.Fatal("30s cron interval should be less than 60s")
	}

	less, err = schedule.IsIntervalLessThan("0 */5 * * * *", from, 60)
	if err != nil {
		t.Fatalf("IsIntervalLessThan(5m) error = %v", err)
	}
	if less {
		t.Fatal("5m cron interval should not be less than 60s")
	}
}

func assertTime(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("got nil, want %s", want)
	}
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}
