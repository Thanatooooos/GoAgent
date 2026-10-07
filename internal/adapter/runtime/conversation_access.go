package runtimeadapter

import (
	"context"
	"fmt"
	"gorm.io/gorm"
)

// Work conversations must never execute with ordinary chat's global tools,
// memory loaders or default knowledge-base scope.
func OrdinaryConversationGuard(db *gorm.DB) func(context.Context, string, string) error {
	return func(ctx context.Context, user, conversation string) error {
		var count int64
		if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM t_work_conversation WHERE conversation_id=?`, conversation).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("work conversations require the Work chat endpoint")
		}
		return nil
	}
}
