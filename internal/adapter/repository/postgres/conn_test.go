package postgres

import (
	"testing"
	"time"

	frameworkconfig "local/rag-project/internal/framework/config"
)

func TestResolveDatabasePoolDefaults(t *testing.T) {
	cfg := frameworkconfig.HikariConfig{}

	if got := resolveMaxOpenConns(cfg); got != defaultMaxOpenConns {
		t.Fatalf("resolveMaxOpenConns() = %d, want %d", got, defaultMaxOpenConns)
	}
	if got := resolveMaxIdleConns(cfg); got != defaultMaxIdleConns {
		t.Fatalf("resolveMaxIdleConns() = %d, want %d", got, defaultMaxIdleConns)
	}
	if got := resolveConnMaxLifetime(cfg); got != defaultConnMaxLifetime {
		t.Fatalf("resolveConnMaxLifetime() = %s, want %s", got, defaultConnMaxLifetime)
	}
	if got := resolveConnMaxIdleTime(cfg); got != defaultConnMaxIdleTime {
		t.Fatalf("resolveConnMaxIdleTime() = %s, want %s", got, defaultConnMaxIdleTime)
	}
	if got := resolveDatabasePingTimeout(cfg); got != defaultDatabasePingTimeout {
		t.Fatalf("resolveDatabasePingTimeout() = %s, want %s", got, defaultDatabasePingTimeout)
	}
}

func TestResolveDatabasePoolConfigUsesHikariValues(t *testing.T) {
	cfg := frameworkconfig.HikariConfig{
		ConnectionTimeout: 3000,
		IdleTimeout:       4000,
		MaxLifetime:       5000,
		MaximumPoolSize:   11,
		MinimumIdle:       7,
	}

	if got := resolveMaxOpenConns(cfg); got != 11 {
		t.Fatalf("resolveMaxOpenConns() = %d, want 11", got)
	}
	if got := resolveMaxIdleConns(cfg); got != 7 {
		t.Fatalf("resolveMaxIdleConns() = %d, want 7", got)
	}
	if got := resolveMinIdleConns(cfg); got != 7 {
		t.Fatalf("resolveMinIdleConns() = %d, want 7", got)
	}
	if got := resolveConnMaxLifetime(cfg); got != 5*time.Second {
		t.Fatalf("resolveConnMaxLifetime() = %s, want 5s", got)
	}
	if got := resolveConnMaxIdleTime(cfg); got != 4*time.Second {
		t.Fatalf("resolveConnMaxIdleTime() = %s, want 4s", got)
	}
	if got := resolveDatabasePingTimeout(cfg); got != 3*time.Second {
		t.Fatalf("resolveDatabasePingTimeout() = %s, want 3s", got)
	}
}
