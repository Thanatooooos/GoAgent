package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	ragmodels "local/rag-project/internal/adapter/repository/postgres/rag/models"
	postgresuser "local/rag-project/internal/adapter/repository/postgres/user"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	ragmemory "local/rag-project/internal/app/rag/core/memory"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	corevector "local/rag-project/internal/app/rag/core/vector"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

// RuntimeOptions 描述 RAG runtime 的装配选项。
type RuntimeOptions struct {
	Config    *config.Config
	DB        *gorm.DB
	AIRuntime *infraai.Runtime
	Searcher  corevector.Searcher
}

// Runtime 聚合最小 RAG 闭环需要的服务。
type Runtime struct {
	DB           *gorm.DB
	ownsDB       bool
	Conversation *ragservice.ConversationService
	Message      *ragservice.ConversationMessageService
	Feedback     *ragservice.MessageFeedbackService
	Trace        *ragservice.TraceService
	Chat         *ragservice.RagChatService
}

// NewRuntime 创建 RAG 最小运行时。
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	_ = ctx

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

	conversationRepo := postgresrag.NewConversationRepository(db)
	messageRepo := postgresrag.NewConversationMessageRepository(db)
	summaryRepo := postgresrag.NewConversationSummaryRepository(db)
	feedbackRepo := postgresrag.NewMessageFeedbackRepository(db)
	traceRunRepo := postgresrag.NewRagTraceRunRepository(db)
	traceNodeRepo := postgresrag.NewRagTraceNodeRepository(db)
	userRepo := postgresuser.NewUserRepository(db)

	conversationService := ragservice.NewConversationService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		nil,
		nil,
		0,
		postgresrag.NewConversationDeleteTransaction(db),
	)
	messageService := ragservice.NewConversationMessageService(
		conversationRepo,
		messageRepo,
		summaryRepo,
		feedbackRepo,
	)

	memoryStore := ragmemory.NewMessageServiceStore(conversationRepo, messageRepo)
	summaryAdapter := ragmemory.NewSummaryServiceAdapter(summaryRepo)
	memoryService := ragmemory.NewDefaultService(memoryStore, summaryAdapter, cfg.Rag.Memory.HistoryKeepTurns)

	promptService := ragprompt.NewService(nil)
	retrieveService := ragretrieve.NewEngine(searcher, aiRuntime.Embedding, aiRuntime.Rerank)
	feedbackService := ragservice.NewMessageFeedbackService(messageRepo, feedbackRepo)
	traceService := ragservice.NewTraceService(traceRunRepo, traceNodeRepo, userRepo)
	chatService := ragservice.NewRagChatService(
		conversationService,
		messageService,
		memoryService,
		retrieveService,
		promptService,
		aiRuntime.Chat,
		traceRunRepo,
		traceNodeRepo,
	)

	return &Runtime{
		DB:           db,
		ownsDB:       ownsDB,
		Conversation: conversationService,
		Message:      messageService,
		Feedback:     feedbackService,
		Trace:        traceService,
		Chat:         chatService,
	}, nil
}

// Close 关闭 runtime 持有的数据库资源。
func (r *Runtime) Close() error {
	if r == nil || r.DB == nil || !r.ownsDB {
		return nil
	}
	return closeRuntimeDB(r.DB)
}

// ensureRagSchema 确保最小 RAG 闭环依赖的数据表存在。
func ensureRagSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("rag db is required")
	}
	return db.AutoMigrate(
		&ragmodels.ConversationModel{},
		&ragmodels.ConversationMessageModel{},
		&ragmodels.ConversationSummaryModel{},
		&ragmodels.MessageFeedbackModel{},
		&ragmodels.RagTraceRunModel{},
		&ragmodels.RagTraceNodeModel{},
	)
}

// closeRuntimeDB 关闭 runtime 内部持有的数据库连接。
func closeRuntimeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
