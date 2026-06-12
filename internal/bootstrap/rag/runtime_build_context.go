package rag

import (
	"fmt"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

type buildContext struct {
	cfg       *config.Config
	db        *gorm.DB
	ownsDB    bool
	aiRuntime *infraai.Runtime
	searcher  corevector.Searcher
}

func newBuildContext(options RuntimeOptions) (*buildContext, error) {
	cfg := options.Config
	if cfg == nil {
		cfg = config.Get()
	}
	if cfg == nil {
		return nil, fmt.Errorf("rag config is required")
	}

	db := options.DB
	ownsDB := false
	if db == nil {
		createdDB, err := postgresrepo.NewGormDB(cfg.Spring.Datasource)
		if err != nil {
			return nil, fmt.Errorf("create rag gorm db: %w", err)
		}
		db = createdDB
		ownsDB = true
	}
	if err := ensureRagSchema(db); err != nil {
		if ownsDB {
			_ = closeRuntimeDB(db)
		}
		return nil, fmt.Errorf("ensure rag schema: %w", err)
	}

	aiRuntime := options.AIRuntime
	if aiRuntime == nil {
		aiRuntime = infraai.NewRuntime()
	}

	searcher := options.Searcher
	if searcher == nil {
		searcher = pgvectorstore.NewVectorStore(db)
	}

	return &buildContext{
		cfg:       cfg,
		db:        db,
		ownsDB:    ownsDB,
		aiRuntime: aiRuntime,
		searcher:  searcher,
	}, nil
}
