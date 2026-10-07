package work

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"local/rag-project/internal/app/work/domain"
	"local/rag-project/internal/app/work/port"
	"local/rag-project/internal/framework/distributedid"
)

type Store struct{ db *gorm.DB }

var _ port.Repository = (*Store)(nil)

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }
func nextID() (string, error) {
	id, err := distributedid.NextID()
	return strconv.FormatInt(id, 10), err
}
func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// The request lock precedes the topic lock in every mutation. A committed
// result survives a lost HTTP reply; different input cannot reuse its key.
func mutate[T any](s *Store, ctx context.Context, user, topic, kind string, m domain.Mutation, input any, fn func(*gorm.DB) (T, error)) (T, error) {
	var result T
	if s == nil || s.db == nil {
		return result, fmt.Errorf("work database is required")
	}
	if strings.TrimSpace(user) == "" {
		return result, fmt.Errorf("user id is required")
	}
	if err := m.Validate(); err != nil {
		return result, err
	}
	data, err := json.Marshal(struct {
		Topic string
		Kind  string
		Input any
	}{topic, kind, input})
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(hash[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, user+"/"+m.RequestID).Error; err != nil {
			return err
		}
		var prior struct {
			InputHash  string
			ResultJSON []byte
		}
		r := tx.Raw(`SELECT input_hash,result_json FROM t_work_operation WHERE user_id=? AND request_id=?`, user, m.RequestID).Scan(&prior)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if prior.InputHash != fingerprint {
				return domain.ErrRequestReused
			}
			return json.Unmarshal(prior.ResultJSON, &result)
		}
		var err error
		result, err = fn(tx)
		if err != nil {
			return err
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		storedTopic := topic
		if t, ok := any(result).(domain.Topic); ok && storedTopic == "" {
			storedTopic = t.ID
		}
		return tx.Exec(`INSERT INTO t_work_operation(user_id,request_id,topic_id,kind,input_hash,result_json) VALUES(?,?,?,?,?,CAST(? AS jsonb))`, user, m.RequestID, storedTopic, kind, fingerprint, string(resultJSON)).Error
	})
	return result, err
}
func getTopic(tx *gorm.DB, user, id string, lock bool) (domain.Topic, error) {
	var topic domain.Topic
	sql := `SELECT * FROM t_work_topic WHERE id=? AND user_id=?`
	if lock {
		sql += ` FOR UPDATE`
	}
	r := tx.Raw(sql, id, user).Scan(&topic)
	if r.Error != nil {
		return topic, r.Error
	}
	if r.RowsAffected == 0 {
		return topic, domain.ErrNotFound
	}
	return topic, nil
}
func activeTopic(tx *gorm.DB, user, id string) (domain.Topic, error) {
	t, e := getTopic(tx, user, id, true)
	if e != nil {
		return t, e
	}
	if t.Status != "active" {
		return t, domain.ErrArchived
	}
	return t, nil
}
func checkItem(tx *gorm.DB, topic, item string) error {
	if item == "" {
		return nil
	}
	var n int64
	if err := tx.Raw(`SELECT COUNT(*) FROM t_work_item WHERE topic_id=? AND id=?`, topic, item).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func checkConversation(tx *gorm.DB, user, topic, id string) error {
	if id == "" {
		return nil
	}
	var n int64
	if err := tx.Raw(`SELECT COUNT(*) FROM t_work_conversation w JOIN t_conversation c ON c.conversation_id=w.conversation_id AND c.user_id=w.user_id WHERE w.topic_id=? AND w.user_id=? AND w.conversation_id=? AND c.deleted=0`, topic, user, id).Scan(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func touch(tx *gorm.DB, topic string) error {
	return tx.Exec(`UPDATE t_work_topic SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, topic).Error
}

func (s *Store) CreateTopic(ctx context.Context, user string, in domain.CreateTopic) (domain.Topic, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if err := domain.ValidateName(in.Name); err != nil {
		return domain.Topic{}, err
	}
	if len(in.Description) > 16000 {
		return domain.Topic{}, fmt.Errorf("description is too long")
	}
	return mutate(s, ctx, user, "", "topic.create", in.Mutation, in, func(tx *gorm.DB) (domain.Topic, error) {
		id, err := nextID()
		if err != nil {
			return domain.Topic{}, err
		}
		if err := tx.Exec(`INSERT INTO t_work_topic(id,user_id,name,description) VALUES(?,?,?,?)`, id, user, in.Name, in.Description).Error; err != nil {
			return domain.Topic{}, err
		}
		entries := []domain.StateEntry{}
		if in.Description != "" {
			entries = append(entries, domain.StateEntry{ID: "goal-" + id, Kind: "goal", Text: in.Description})
		}
		if _, err := insertState(tx, id, 1, entries, "user"); err != nil {
			return domain.Topic{}, err
		}
		if err := tx.Exec(`INSERT INTO t_work_state(topic_id,revision) VALUES(?,1)`, id).Error; err != nil {
			return domain.Topic{}, err
		}
		return getTopic(tx, user, id, false)
	})
}
func (s *Store) ListTopics(ctx context.Context, user, status string, page domain.Page) ([]domain.Topic, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if status != "active" && status != "archived" && status != "" {
		return nil, fmt.Errorf("invalid topic status")
	}
	rows := []domain.Topic{}
	query := s.db.WithContext(ctx).Table("t_work_topic").Where("user_id=?", user)
	if status != "" {
		query = query.Where("status=?", status)
	}
	err := query.Order("updated_at DESC,id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error
	return rows, err
}
func (s *Store) GetTopic(ctx context.Context, user, topic string) (domain.Topic, error) {
	return getTopic(s.db.WithContext(ctx), user, topic, false)
}
func (s *Store) UpdateTopic(ctx context.Context, user, topic string, in domain.UpdateTopic) (domain.Topic, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := domain.ValidateName(in.Name); err != nil {
		return domain.Topic{}, err
	}
	if len(in.Description) > 16000 {
		return domain.Topic{}, fmt.Errorf("description is too long")
	}
	if in.Status != "active" && in.Status != "archived" {
		return domain.Topic{}, fmt.Errorf("invalid topic status")
	}
	return mutate(s, ctx, user, topic, "topic.update", in.Mutation, in, func(tx *gorm.DB) (domain.Topic, error) {
		t, err := getTopic(tx, user, topic, true)
		if err != nil {
			return t, err
		}
		if t.Revision != in.ExpectedRevision {
			return t, &domain.Conflict{CurrentRevision: t.Revision}
		}
		// Restoring or archiving does not silently rename archived content.
		if t.Status == "archived" && (t.Name != in.Name || t.Description != in.Description) {
			return t, domain.ErrArchived
		}
		if err := tx.Exec(`UPDATE t_work_topic SET name=?,description=?,status=?,revision=revision+1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.Name, in.Description, in.Status, topic).Error; err != nil {
			return t, err
		}
		return getTopic(tx, user, topic, false)
	})
}
func (s *Store) CreateItem(ctx context.Context, user, topic string, in domain.CreateItem) (domain.Item, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := domain.ValidateName(in.Name); err != nil {
		return domain.Item{}, err
	}
	return mutate(s, ctx, user, topic, "item.create", in.Mutation, in, func(tx *gorm.DB) (domain.Item, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.Item{}, err
		}
		id, err := nextID()
		if err != nil {
			return domain.Item{}, err
		}
		item := domain.Item{ID: id, TopicID: topic, Name: in.Name, CreatedAt: time.Now().UTC()}
		if err := tx.Exec(`INSERT INTO t_work_item(id,topic_id,name,created_at) VALUES(?,?,?,?)`, item.ID, topic, item.Name, item.CreatedAt).Error; err != nil {
			return item, err
		}
		return item, touch(tx, topic)
	})
}
func (s *Store) ListItems(ctx context.Context, user, topic string, page domain.Page) ([]domain.Item, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	rows := []domain.Item{}
	err := s.db.WithContext(ctx).Table("t_work_item").Where("topic_id=?", topic).Order("created_at DESC,id DESC").Limit(page.Limit).Offset(page.Offset).Find(&rows).Error
	return rows, err
}
func (s *Store) CreateConversation(ctx context.Context, user, topic string, in domain.CreateConversation) (domain.Conversation, error) {
	in.Title = strings.TrimSpace(in.Title)
	if err := domain.ValidateName(in.Title); err != nil {
		return domain.Conversation{}, err
	}
	return mutate(s, ctx, user, topic, "conversation.create", in.Mutation, in, func(tx *gorm.DB) (domain.Conversation, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.Conversation{}, err
		}
		if err := checkItem(tx, topic, in.ItemID); err != nil {
			return domain.Conversation{}, err
		}
		if err := checkConversation(tx, user, topic, in.ContinueFrom); err != nil {
			return domain.Conversation{}, err
		}
		id, err := nextID()
		if err != nil {
			return domain.Conversation{}, err
		}
		now := time.Now()
		if err := tx.Exec(`INSERT INTO t_conversation(id,conversation_id,user_id,title,last_time,create_time,update_time,deleted) VALUES(?,?,?,?,?,?,?,0)`, id, id, user, in.Title, now, now, now).Error; err != nil {
			return domain.Conversation{}, err
		}
		if err := tx.Exec(`INSERT INTO t_work_conversation(conversation_id,topic_id,user_id,item_id,continue_from,created_at) VALUES(?,?,?,?,?,?)`, id, topic, user, nullable(in.ItemID), nullable(in.ContinueFrom), now).Error; err != nil {
			return domain.Conversation{}, err
		}
		return domain.Conversation{ID: id, TopicID: topic, ItemID: in.ItemID, ContinueFrom: in.ContinueFrom, Title: in.Title, CreatedAt: now, UpdatedAt: now}, touch(tx, topic)
	})
}
func (s *Store) ListConversations(ctx context.Context, user, topic string, page domain.Page) ([]domain.Conversation, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	rows := []domain.Conversation{}
	err := s.db.WithContext(ctx).Raw(`SELECT w.conversation_id AS id,w.topic_id,COALESCE(w.item_id,'') AS item_id,COALESCE(w.continue_from,'') AS continue_from,c.title,w.created_at,c.update_time AS updated_at FROM t_work_conversation w JOIN t_conversation c ON c.conversation_id=w.conversation_id AND c.user_id=w.user_id WHERE w.topic_id=? AND w.user_id=? AND c.deleted=0 ORDER BY c.update_time DESC,w.conversation_id DESC LIMIT ? OFFSET ?`, topic, user, page.Limit, page.Offset).Scan(&rows).Error
	return rows, err
}
