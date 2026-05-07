package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresingestion "local/rag-project/internal/adapter/repository/postgres/ingestion"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresuser "local/rag-project/internal/adapter/repository/postgres/user"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragmemory "local/rag-project/internal/app/rag/core/memory"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	corevector "local/rag-project/internal/app/rag/core/vector"
	ragservice "local/rag-project/internal/app/rag/service"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragbuiltin "local/rag-project/internal/app/rag/tool/builtin"
	"local/rag-project/internal/app/rag/tool/planner"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
	aichat "local/rag-project/internal/infra-ai/chat"
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
	var summaryAdapter ragmemory.SummaryService
	if cfg.Rag.Memory.SummaryEnabled {
		summaryAdapter = ragmemory.NewCompressibleSummaryService(summaryRepo, ragmemory.SummaryCompressionOptions{
			MessageRepo: messageRepo,
			ChatService: aiRuntime.Chat,
			StartTurns:  cfg.Rag.Memory.SummaryStartTurns,
			MaxChars:    cfg.Rag.Memory.SummaryMaxChars,
		})
	} else {
		summaryAdapter = ragmemory.NewSummaryServiceAdapter(summaryRepo)
	}
	memoryService := ragmemory.NewDefaultService(memoryStore, summaryAdapter, cfg.Rag.Memory.HistoryKeepTurns)

	// 根据配置决定是否启用 LLM 查询改写；未启用时 retieve 阶段直接使用原始问题。
	var rewriteService ragrewrite.Service
	if cfg.Rag.QueryRewrite.Enabled {
		rewriteService = ragrewrite.NewLLMService(aiRuntime.Chat)
	}
	promptService := ragprompt.NewService(nil)
	retrieveService := ragretrieve.NewEngine(searcher, aiRuntime.Embedding, aiRuntime.Rerank)
	feedbackService := ragservice.NewMessageFeedbackService(messageRepo, feedbackRepo)
	traceService := ragservice.NewTraceService(traceRunRepo, traceNodeRepo, userRepo)
		tracer := ragservice.NewChatTracer(traceRunRepo, traceNodeRepo)
	chatService := ragservice.NewRagChatService(
		conversationService,
		messageService,
		memoryService,
		rewriteService,
		retrieveService,
		promptService,
		aiRuntime.Chat,
			tracer,
	)
	// 检索置信度阈值：低于此值时回退到通用 LLM 模式并提醒用户。
	if cfg != nil {
		chatService.SetConfidenceThreshold(cfg.Rag.Search.Channels.VectorGlobal.ConfidenceThreshold)
	}
	chatService.SetToolWorkflow(buildLocalToolWorkflow(db, traceRunRepo, traceNodeRepo, aiRuntime.Chat))

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

// ragRequiredTables RAG 模块依赖的数据表。
var ragRequiredTables = []string{
	"t_conversation",
	"t_conversation_summary",
	"t_message",
	"t_message_feedback",
	"t_rag_trace_run",
	"t_rag_trace_node",
}

// ensureRagSchema 确保 RAG 依赖的表已通过 migration 创建，不再使用 AutoMigrate。
func ensureRagSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("rag db is required")
	}
	return postgresrepo.EnsureTablesExist(db, ragRequiredTables)
}

// closeRuntimeDB 关闭 runtime 内部持有的数据库连接。
func closeRuntimeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

type knowledgePipelineTaskReader struct {
	taskService *ingestionservice.TaskService
}

func (r knowledgePipelineTaskReader) GetKnowledgePipelineTask(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
	return r.taskService.Get(ctx, taskID)
}

func (r knowledgePipelineTaskReader) ListKnowledgePipelineTaskNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error) {
	return r.taskService.ListNodes(ctx, taskID)
}

func buildLocalToolWorkflow(
	db *gorm.DB,
	traceRunRepo *postgresrag.RagTraceRunRepository,
	traceNodeRepo *postgresrag.RagTraceNodeRepository,
	chatService aichat.LLMService,
) ragtool.Workflow {
	if db == nil {
		return nil
	}

	registry := ragtool.NewRegistry()

	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	documentRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	chunkLogRepo := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(db)
		documentService := knowledgeservice.NewKnowledgeDocumentService(
			baseRepo,
			documentRepo,
			nil,
		chunkLogRepo,
		nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)

	pipelineRepo := postgresingestion.NewPipelineRepository(db)
	taskRepo := postgresingestion.NewTaskRepository(db)
	taskNodeRepo := postgresingestion.NewTaskNodeRepository(db)
	taskService := ingestionservice.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, nil)
	documentService.SetIngestionTaskReader(knowledgePipelineTaskReader{taskService: taskService})

	registry.MustRegister(ragbuiltin.NewDocumentQueryTool(documentService))
	registry.MustRegister(ragbuiltin.NewDocumentIngestionDiagnoseTool(documentService))
	registry.MustRegister(ragbuiltin.NewDocumentChunkLogQueryTool(documentService))
	registry.MustRegister(ragbuiltin.NewTaskIngestionDiagnoseTool(taskService))
	registry.MustRegister(ragbuiltin.NewIngestionTaskQueryTool(taskService))
	registry.MustRegister(ragbuiltin.NewIngestionTaskNodeQueryTool(taskService))
	registry.MustRegister(ragbuiltin.NewTraceNodeQueryTool(traceRunRepo, traceNodeRepo))
	registry.MustRegister(ragbuiltin.NewTraceRetrievalDiagnoseTool(traceRunRepo, traceNodeRepo))

	wf := ragtool.NewLocalWorkflow(ragtool.NewExecutor(registry))
	if chatService != nil {
		wf.SetPlanner(planner.NewLLMPlanner(chatService))
	}
	return wf
}
