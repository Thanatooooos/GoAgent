package dailybrief

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/dailybrief/port"
)

type PublishTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, issueRepo port.IssueRepository, itemRepo port.ItemRepository) error,
) error

func NewPublishTransaction(db *gorm.DB) PublishTransaction {
	return func(
		ctx context.Context,
		fn func(ctx context.Context, issueRepo port.IssueRepository, itemRepo port.ItemRepository) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(
				ctx,
				NewIssueRepository(tx),
				NewItemRepository(tx),
			)
		})
	}
}
