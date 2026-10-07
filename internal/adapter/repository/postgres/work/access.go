package work

import (
	"context"
	"local/rag-project/internal/app/work/domain"
)

func (s *Store) ValidateWorkConversation(ctx context.Context, user, id string) error {
	var topic string
	r := s.db.WithContext(ctx).Raw(`SELECT w.topic_id FROM t_work_conversation w JOIN t_conversation c ON c.conversation_id=w.conversation_id AND c.user_id=w.user_id JOIN t_work_topic t ON t.id=w.topic_id AND t.user_id=w.user_id WHERE w.user_id=? AND w.conversation_id=? AND c.deleted=0 AND t.status='active'`, user, id).Scan(&topic)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
