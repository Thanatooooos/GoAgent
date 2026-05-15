package knowledge

import (
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
)

func TestScheduleRunTimeoutDefaultsToThirtySeconds(t *testing.T) {
	t.Parallel()

	if got := scheduleRunTimeout(nil); got != 30*time.Second {
		t.Fatalf("scheduleRunTimeout(nil) = %s, want %s", got, 30*time.Second)
	}

	cfg := &config.Config{}
	if got := scheduleRunTimeout(cfg); got != 30*time.Second {
		t.Fatalf("scheduleRunTimeout(zero cfg) = %s, want %s", got, 30*time.Second)
	}
}

func TestScheduleRunTimeoutUsesConfiguredValue(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.Rag.Knowledge.Schedule.RunTimeoutMs = 45000

	if got := scheduleRunTimeout(cfg); got != 45*time.Second {
		t.Fatalf("scheduleRunTimeout(cfg) = %s, want %s", got, 45*time.Second)
	}
}
