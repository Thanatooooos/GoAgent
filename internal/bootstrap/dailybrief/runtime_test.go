package dailybrief

import (
	"testing"

	"local/rag-project/internal/framework/config"
)

func TestScheduleScanIntervalDefaults(t *testing.T) {
	t.Parallel()

	if got := scheduleScanInterval(nil); got != 10*1e9 {
		t.Fatalf("expected 10s default, got %v", got)
	}
	cfg := &config.Config{
		DailyBrief: config.DailyBriefConfig{
			Schedule: config.DailyBriefScheduleConfig{ScanDelayMs: 5000},
		},
	}
	if got := scheduleScanInterval(cfg); got != 5*1e9 {
		t.Fatalf("expected 5s, got %v", got)
	}
}

func TestScheduleRunTimeoutDefaults(t *testing.T) {
	t.Parallel()

	if got := scheduleRunTimeout(nil); got != 30*1e9 {
		t.Fatalf("expected 30s default, got %v", got)
	}
}

func TestNewRuntimeRequiresConfigOrDB(t *testing.T) {
	t.Parallel()

	if _, err := NewRuntime(t.Context(), RuntimeOptions{}); err == nil {
		t.Fatal("expected error when config and db are missing")
	}
}
