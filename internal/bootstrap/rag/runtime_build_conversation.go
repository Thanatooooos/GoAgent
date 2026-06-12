package rag

import (
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	raghistory "local/rag-project/internal/app/rag/core/history"
	ragservice "local/rag-project/internal/app/rag/service"
)

type conversationBundle struct {
	conversationService *ragservice.ConversationService
	messageService      *ragservice.ConversationMessageService
	historyService      raghistory.Service
	feedbackService     *ragservice.MessageFeedbackService
	summaryJobWorker    *raghistory.InMemorySummaryJobWorker
}

func buildConversationServices(buildCtx *buildContext, repos repositoriesBundle) conversationBundle {
	cfg := buildCtx.cfg
	aiRuntime := buildCtx.aiRuntime
	db := buildCtx.db

	conversationService := ragservice.NewConversationService(
		repos.conversationRepo,
		repos.messageRepo,
		repos.summaryRepo,
		nil,
		nil,
		0,
		postgresrag.NewConversationDeleteTransaction(db),
	)
	messageService := ragservice.NewConversationMessageService(
		repos.conversationRepo,
		repos.messageRepo,
		repos.summaryRepo,
		repos.feedbackRepo,
	)
	messageService.SetContentProcessor(ragservice.NewLongMessageContentProcessor(ragservice.LongMessageProcessorOptions{
		Enabled:                     cfg.Rag.Memory.LongMessage.Enabled,
		DirectContextMaxTokens:      cfg.Rag.Memory.LongMessage.DirectContextMaxTokens,
		ChunkSummaryThresholdTokens: cfg.Rag.Memory.LongMessage.ChunkSummaryThresholdTokens,
		LargeChunkTargetTokens:      cfg.Rag.Memory.LongMessage.LargeChunkTargetTokens,
		LargeChunkOverlapTokens:     cfg.Rag.Memory.LongMessage.LargeChunkOverlapTokens,
		MediumSummaryMaxChars:       cfg.Rag.Memory.LongMessage.MediumSummaryMaxChars,
		ChunkSummaryMaxChars:        cfg.Rag.Memory.LongMessage.ChunkSummaryMaxChars,
		LargeSummaryMaxChars:        cfg.Rag.Memory.LongMessage.LargeSummaryMaxChars,
		Estimator:                   ragservice.RoughTokenEstimator{},
		ChatService:                 aiRuntime.Chat,
	}))
	messageService.SetChunkSink(postgresrag.NewConversationMessageChunkSink(db, aiRuntime.Embedding))
	messageService.SetCreateTransaction(postgresrag.NewConversationMessageCreateTransaction(db, aiRuntime.Embedding))

	memoryStore := raghistory.NewMessageServiceStore(repos.conversationRepo, repos.messageRepo)
	var summaryAdapter raghistory.SummaryService
	var summaryJobWorker *raghistory.InMemorySummaryJobWorker
	if cfg.Rag.Memory.SummaryEnabled {
		compressible := raghistory.NewCompressibleSummaryService(repos.summaryRepo, raghistory.SummaryCompressionOptions{
			MessageRepo: repos.messageRepo,
			ChatService: aiRuntime.Chat,
			StartTurns:  cfg.Rag.Memory.SummaryStartTurns,
			MaxChars:    cfg.Rag.Memory.SummaryMaxChars,
		})
		if cfg.Rag.Memory.SummaryAsync.Enabled {
			summaryJobWorker = compressible.EnableAsyncSummaryJobs(32)
		}
		summaryAdapter = compressible
	} else {
		summaryAdapter = raghistory.NewSummaryServiceAdapter(repos.summaryRepo)
	}
	historyService := raghistory.NewDefaultService(memoryStore, summaryAdapter, cfg.Rag.Memory.HistoryKeepTurns)
	feedbackService := ragservice.NewMessageFeedbackService(repos.messageRepo, repos.feedbackRepo)

	return conversationBundle{
		conversationService: conversationService,
		messageService:      messageService,
		historyService:      historyService,
		feedbackService:     feedbackService,
		summaryJobWorker:    summaryJobWorker,
	}
}
