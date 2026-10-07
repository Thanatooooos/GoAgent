package work

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/work/service"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/stream"
	"sync"
)

type Runtime struct {
	DB             *gorm.DB
	Knowledge      *knowledgebootstrap.Runtime
	EmbeddingModel string
	cleanupCancel  context.CancelFunc
	cleanupWG      sync.WaitGroup
	Store          *postgreswork.Store
	Service        *service.Service
	Kernel         *conversationruntime.Runtime
	Chat           *conversationruntime.ChatService
	Streams        stream.StreamManager
}

func NewRuntime(db *gorm.DB) (*Runtime, error) {
	if db == nil {
		return nil, fmt.Errorf("work database is required")
	}
	if err := postgresrepo.EnsureTablesExist(db, []string{"t_work_topic", "t_work_item", "t_work_conversation", "t_work_state", "t_work_state_revision", "t_work_artifact", "t_work_artifact_version", "t_work_operation", "t_work_turn", "t_work_proposal", "t_work_turn_operation", "t_work_source", "t_work_history_summary"}); err != nil {
		return nil, err
	}
	store := postgreswork.NewStore(db)
	return &Runtime{DB: db, Store: store, Service: service.New(store)}, nil
}
