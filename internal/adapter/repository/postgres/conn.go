package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	frameworkconfig "local/rag-project/internal/framework/config"
)

const (
	defaultDatabasePingTimeout  = 5 * time.Second
	defaultMaxOpenConns         = 25
	defaultMaxIdleConns         = 10
	defaultConnMaxLifetime      = 30 * time.Minute
	defaultConnMaxIdleTime      = 10 * time.Minute
	defaultPGXHealthCheckPeriod = 30 * time.Second
)

func NewGormDB(cfg frameworkconfig.DataSourceConfig) (*gorm.DB, error) {
	dsn, err := ParsePostgresDSN(cfg)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	applySQLDBPoolConfig(sqlDB, cfg.Hikari)

	pingCtx, cancel := context.WithTimeout(context.Background(), resolveDatabasePingTimeout(cfg.Hikari))
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database health check failed: %w", err)
	}
	return db, nil
}

func NewPGXPool(ctx context.Context, cfg frameworkconfig.DataSourceConfig) (*pgxpool.Pool, error) {
	dsn, err := ParsePostgresDSN(cfg)
	if err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pgx pool config: %w", err)
	}
	applyPGXPoolConfig(poolConfig, cfg.Hikari)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, resolveDatabasePingTimeout(cfg.Hikari))
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgx pool health check failed: %w", err)
	}
	return pool, nil
}

func ParsePostgresDSN(cfg frameworkconfig.DataSourceConfig) (string, error) {
	raw := strings.TrimSpace(cfg.Url)
	if raw == "" {
		return "", fmt.Errorf("postgres url is required")
	}

	raw = strings.TrimPrefix(raw, "jdbc:")
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse postgres url: %w", err)
	}

	if cfg.Username != "" {
		if cfg.Password != "" {
			parsed.User = url.UserPassword(cfg.Username, cfg.Password)
		} else {
			parsed.User = url.User(cfg.Username)
		}
	}

	query := parsed.Query()
	query.Set("sslmode", "disable")
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func applySQLDBPoolConfig(sqlDB *sql.DB, cfg frameworkconfig.HikariConfig) {
	if sqlDB == nil {
		return
	}
	sqlDB.SetMaxOpenConns(resolveMaxOpenConns(cfg))
	sqlDB.SetMaxIdleConns(resolveMaxIdleConns(cfg))
	sqlDB.SetConnMaxLifetime(resolveConnMaxLifetime(cfg))
	sqlDB.SetConnMaxIdleTime(resolveConnMaxIdleTime(cfg))
}

func applyPGXPoolConfig(poolConfig *pgxpool.Config, cfg frameworkconfig.HikariConfig) {
	if poolConfig == nil {
		return
	}
	poolConfig.MaxConns = int32(resolveMaxOpenConns(cfg))
	poolConfig.MinConns = int32(resolveMinIdleConns(cfg))
	poolConfig.MaxConnLifetime = resolveConnMaxLifetime(cfg)
	poolConfig.MaxConnIdleTime = resolveConnMaxIdleTime(cfg)
	poolConfig.HealthCheckPeriod = resolvePGXHealthCheckPeriod(cfg)
}

func resolveDatabasePingTimeout(cfg frameworkconfig.HikariConfig) time.Duration {
	if cfg.ConnectionTimeout > 0 {
		return time.Duration(cfg.ConnectionTimeout) * time.Millisecond
	}
	return defaultDatabasePingTimeout
}

func resolveMaxOpenConns(cfg frameworkconfig.HikariConfig) int {
	if cfg.MaximumPoolSize > 0 {
		return cfg.MaximumPoolSize
	}
	return defaultMaxOpenConns
}

func resolveMinIdleConns(cfg frameworkconfig.HikariConfig) int {
	if cfg.MinimumIdle > 0 {
		return cfg.MinimumIdle
	}
	if resolved := resolveMaxIdleConns(cfg); resolved > 0 {
		return resolved
	}
	return defaultMaxIdleConns
}

func resolveMaxIdleConns(cfg frameworkconfig.HikariConfig) int {
	if cfg.MinimumIdle > 0 {
		return cfg.MinimumIdle
	}
	return defaultMaxIdleConns
}

func resolveConnMaxLifetime(cfg frameworkconfig.HikariConfig) time.Duration {
	if cfg.MaxLifetime > 0 {
		return time.Duration(cfg.MaxLifetime) * time.Millisecond
	}
	return defaultConnMaxLifetime
}

func resolveConnMaxIdleTime(cfg frameworkconfig.HikariConfig) time.Duration {
	if cfg.IdleTimeout > 0 {
		return time.Duration(cfg.IdleTimeout) * time.Millisecond
	}
	return defaultConnMaxIdleTime
}

func resolvePGXHealthCheckPeriod(cfg frameworkconfig.HikariConfig) time.Duration {
	idle := resolveConnMaxIdleTime(cfg)
	if idle <= 0 {
		return defaultPGXHealthCheckPeriod
	}
	period := idle / 2
	if period <= 0 {
		return defaultPGXHealthCheckPeriod
	}
	return period
}
