package knowledge

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service"
)

func NewKnowledgeDocumentScheduleTransaction(db *gorm.DB) service.KnowledgeDocumentScheduleTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, scheduleRepo port.KnowledgeDocumentScheduleRepository, scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(
				ctx,
				NewKnowledgeDocumentScheduleRepository(tx),
				NewKnowledgeDocumentScheduleExecRepository(tx),
			)
		})
	}
}
