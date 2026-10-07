package work

import (
	"context"
	"fmt"
	"local/rag-project/internal/app/work/domain"
)

func (s *Store) GetMessage(ctx context.Context, user, topic, id string) (domain.Message, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.Message{}, err
	}
	var m domain.Message
	r := s.db.WithContext(ctx).Raw(`SELECT m.id,m.conversation_id,m.role,CASE WHEN COALESCE(BTRIM(m.raw_content),'') <> '' THEN m.raw_content ELSE m.content END AS content FROM t_message m JOIN t_work_conversation w ON w.conversation_id=m.conversation_id AND w.user_id=m.user_id JOIN t_conversation c ON c.conversation_id=m.conversation_id AND c.user_id=m.user_id WHERE m.id=? AND w.topic_id=? AND w.user_id=? AND m.deleted=0 AND c.deleted=0`, id, topic, user).Scan(&m)
	if r.Error != nil {
		return m, r.Error
	}
	if r.RowsAffected != 1 {
		return m, domain.ErrNotFound
	}
	return m, nil
}

func (s *Store) SearchHistory(ctx context.Context, user, topic, item, conversation, query string) ([]domain.Message, error) {
	return s.searchHistory(ctx, user, topic, item, conversation, query, false)
}
func (s *Store) SearchHistoryAcrossItems(ctx context.Context, user, topic, conversation, query string) ([]domain.Message, error) {
	return s.searchHistory(ctx, user, topic, "", conversation, query, true)
}
func (s *Store) searchHistory(ctx context.Context, user, topic, item, conversation, query string, allItems bool) ([]domain.Message, error) {
	if len(query) > 300 {
		return nil, fmt.Errorf("history query is too long")
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	if err := checkItem(s.db.WithContext(ctx), topic, item); err != nil {
		return nil, err
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
		return nil, err
	}
	rows := []domain.Message{}
	err := s.db.WithContext(ctx).Raw(`SELECT m.id,m.conversation_id,m.role,LEFT(m.content,12000) AS content,m.create_time AS created_at FROM t_message m JOIN t_work_turn t ON (m.id=t.user_message_id OR m.id=t.assistant_message_id) AND m.user_id=t.user_id JOIN t_conversation c ON c.conversation_id=m.conversation_id AND c.user_id=m.user_id WHERE t.topic_id=? AND t.user_id=? AND m.deleted=0 AND c.deleted=0 AND (? OR COALESCE(t.item_id,'')=? OR t.item_id IS NULL) AND (?='' OR m.conversation_id=?) AND (?='' OR strpos(lower(m.content),lower(?))>0) ORDER BY m.id DESC LIMIT 20`, topic, user, allItems, item, conversation, conversation, query, query).Scan(&rows).Error
	return rows, err
}
