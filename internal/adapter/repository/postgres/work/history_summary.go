package work

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	conversationruntime "local/rag-project/internal/app/runtime"
)

func (h History) Snapshot(ctx context.Context, request conversationruntime.RunRequest) (conversationruntime.HistorySnapshot, error) {
	var out conversationruntime.HistorySnapshot
	if request.Work == nil {
		return out, fmt.Errorf("Work history requires scope")
	}
	if _, err := h.Store.GetTopic(ctx, request.UserID, request.Work.TopicID); err != nil {
		return out, err
	}
	if err := checkConversation(h.Store.db.WithContext(ctx), request.UserID, request.Work.TopicID, request.ConversationID); err != nil {
		return out, err
	}
	var link struct{ ContinueFrom string }
	if err := h.Store.db.WithContext(ctx).Raw(`SELECT COALESCE(continue_from,'') AS continue_from FROM t_work_conversation WHERE conversation_id=? AND user_id=?`, request.ConversationID, request.UserID).Scan(&link).Error; err != nil {
		return out, err
	}
	if err := h.Store.db.WithContext(ctx).Raw(`SELECT content,structured_json,covered_from_message_id,covered_to_message_id,source_message_count FROM t_work_history_summary WHERE topic_id=? AND conversation_id=? AND item_key=?`, request.Work.TopicID, request.ConversationID, request.Work.ItemID).Scan(&out.Summary).Error; err != nil {
		return out, err
	}
	var rows []struct{ ID, Role, Content string }
	err := h.Store.db.WithContext(ctx).Raw(`SELECT m.id,m.role,m.content FROM t_message m JOIN t_work_turn t ON (m.id=t.user_message_id OR m.id=t.assistant_message_id) AND m.user_id=t.user_id JOIN t_conversation c ON c.conversation_id=m.conversation_id AND c.user_id=m.user_id WHERE t.topic_id=? AND t.user_id=? AND m.deleted=0 AND c.deleted=0 AND m.id<? AND (?='' OR m.id>?) AND (t.conversation_id=? OR t.conversation_id=?) AND (COALESCE(t.item_id,'')=? OR t.conversation_id=?) ORDER BY m.id DESC LIMIT 200`, request.Work.TopicID, request.UserID, request.UserMessageID, out.Summary.CoveredToMessageID, out.Summary.CoveredToMessageID, request.ConversationID, link.ContinueFrom, request.Work.ItemID, link.ContinueFrom).Scan(&rows).Error
	if err != nil {
		return out, err
	}
	for i := len(rows) - 1; i >= 0; i-- {
		out.Messages = append(out.Messages, conversationruntime.HistoryMessage{ID: rows[i].ID, Role: conversationruntime.ModelRole(rows[i].Role), Content: rows[i].Content})
	}
	return out, nil
}
func (h History) StoreSummary(ctx context.Context, request conversationruntime.RunRequest, summary conversationruntime.HistorySummary) (bool, error) {
	if request.Work == nil {
		return false, fmt.Errorf("Work scope required")
	}
	accepted := false
	err := h.Store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := activeTopic(tx, request.UserID, request.Work.TopicID); err != nil {
			return err
		}
		if err := checkConversation(tx, request.UserID, request.Work.TopicID, request.ConversationID); err != nil {
			return err
		}
		r := tx.Exec(`INSERT INTO t_work_history_summary(topic_id,conversation_id,item_key,content,structured_json,covered_from_message_id,covered_to_message_id,source_message_count) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(conversation_id,item_key) DO UPDATE SET content=EXCLUDED.content,structured_json=EXCLUDED.structured_json,covered_to_message_id=EXCLUDED.covered_to_message_id,source_message_count=EXCLUDED.source_message_count,updated_at=CURRENT_TIMESTAMP WHERE t_work_history_summary.covered_to_message_id<EXCLUDED.covered_to_message_id`, request.Work.TopicID, request.ConversationID, request.Work.ItemID, summary.Content, summary.StructuredJSON, summary.CoveredFromMessageID, summary.CoveredToMessageID, summary.SourceMessageCount)
		accepted = r.RowsAffected == 1
		return r.Error
	})
	return accepted, err
}
