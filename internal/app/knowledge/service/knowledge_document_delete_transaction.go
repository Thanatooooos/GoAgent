package service

import (
	"context"

	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentDeleteTransaction func(
	ctx context.Context,
	fn func(
		ctx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
		scheduleRepo port.KnowledgeDocumentScheduleRepository,
		scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
	) error,
) error
