package knowledge

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service"
)

func NewKnowledgeChunkTransaction(db *gorm.DB) service.KnowledgeChunkMutationTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(
				ctx,
				NewKnowledgeDocumentRepository(tx, nil),
				NewKnowledgeChunkRepository(tx),
				pgvectorstore.NewVectorStore(tx),
			)
		})
	}
}
