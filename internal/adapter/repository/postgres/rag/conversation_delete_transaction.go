package rag

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/port"
)

// NewConversationDeleteTransaction 创建会话级联删除事务包装器。
func NewConversationDeleteTransaction(db *gorm.DB) port.ConversationDeleteTransaction {
	return func(
		ctx context.Context,
		userID string,
		conversationID string,
		fn func(
			ctx context.Context,
			conversationRepo port.ConversationRepository,
			messageRepo port.ConversationMessageRepository,
			summaryRepo port.ConversationSummaryRepository,
		) error,
	) error {
		if db == nil {
			return fmt.Errorf("gorm db is required")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var topic struct{ ID, Status string }
			if err := tx.Raw(`SELECT t.id,t.status FROM t_work_topic t JOIN t_work_conversation c ON c.topic_id=t.id WHERE c.user_id=? AND c.conversation_id=? FOR UPDATE OF t`, userID, conversationID).Scan(&topic).Error; err != nil {
				return err
			}
			if topic.Status == "archived" {
				return fmt.Errorf("归档专题只读，请先恢复专题")
			}
			if topic.ID != "" {
				if err := tx.Exec(`DELETE FROM t_work_history_summary WHERE conversation_id=? OR conversation_id IN (SELECT conversation_id FROM t_work_conversation WHERE continue_from=? AND topic_id=?)`, conversationID, conversationID, topic.ID).Error; err != nil {
					return err
				}
				if err := tx.Exec(`UPDATE t_work_source SET status=CASE WHEN source_type='shared' THEN 'removed' ELSE 'cleanup_pending' END,updated_at=CURRENT_TIMESTAMP WHERE user_id=? AND topic_id=? AND conversation_id=? AND promoted=false AND status NOT IN ('removed','cleaned','cleanup_pending')`, userID, topic.ID, conversationID).Error; err != nil {
					return err
				}
				if err := tx.Exec(`UPDATE t_work_turn SET status='cancelled',updated_at=CURRENT_TIMESTAMP WHERE topic_id=? AND user_id=? AND conversation_id=? AND status IN ('accepted','running')`, topic.ID, userID, conversationID).Error; err != nil {
					return err
				}
			}
			// Lock the task before deleting its conversation, matching the
			// task-to-conversation lock order used by report publication.
			if err := tx.Exec(`UPDATE t_scheduled_task SET status = 'paused', next_due_at = NULL,
				conversation_id = NULL, update_time = CURRENT_TIMESTAMP
				WHERE user_id = ? AND conversation_id = ? AND deleted_at IS NULL`, userID, conversationID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DELETE FROM t_conversation_unread WHERE user_id = ? AND conversation_id = ?`, userID, conversationID).Error; err != nil {
				return err
			}
			return fn(
				ctx,
				NewConversationRepository(tx),
				NewConversationMessageRepository(tx),
				NewConversationSummaryRepository(tx),
			)
		})
	}
}
