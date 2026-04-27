package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	frameworkconfig "local/rag-project/internal/framework/config"
)

func NewGormDB(cfg frameworkconfig.DataSourceConfig) (*gorm.DB, error) {
	dsn, err := ParsePostgresDSN(cfg)
	if err != nil {
		return nil, err
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func NewPGXPool(ctx context.Context, cfg frameworkconfig.DataSourceConfig) (*pgxpool.Pool, error) {
	dsn, err := ParsePostgresDSN(cfg)
	if err != nil {
		return nil, err
	}
	return pgxpool.New(ctx, dsn)
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
