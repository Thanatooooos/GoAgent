package user

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresuser "local/rag-project/internal/adapter/repository/postgres/user"
	userservice "local/rag-project/internal/app/user/service"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/contextx"
)

type RuntimeOptions struct {
	Config *config.Config
}

type Runtime struct {
	DB          *gorm.DB
	UserService *userservice.UserService
	AuthService *userservice.AuthService
}

func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil {
		return nil, fmt.Errorf("user config is required")
	}

	db, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
	if err != nil {
		return nil, fmt.Errorf("create user gorm db: %w", err)
	}

	userRepo := postgresuser.NewUserRepository(db)
	sessionRepo := postgresuser.NewUserSessionRepository(db)
	timeout := time.Duration(cfg.SaToken.Timeout) * time.Second

	return &Runtime{
		DB:          db,
		UserService: userservice.NewUserService(userRepo, sessionRepo),
		AuthService: userservice.NewAuthService(userRepo, sessionRepo, timeout, cfg.SaToken.IsConcurrent),
	}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.DB == nil {
		return nil
	}
	sqlDB, err := r.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (r *Runtime) LoadLoginUser(token string) (*contextx.LoginUser, error) {
	if r == nil || r.AuthService == nil {
		return nil, nil
	}
	return r.AuthService.LoadLoginUser(context.Background(), token)
}
