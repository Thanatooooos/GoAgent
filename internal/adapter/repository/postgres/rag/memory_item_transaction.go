package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

func NewMemoryItemTransaction(db *gorm.DB) longtermmemory.MemoryMutationTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, repo port.MemoryItemRepository) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(ctx, NewMemoryItemRepository(tx))
		})
	}
}
