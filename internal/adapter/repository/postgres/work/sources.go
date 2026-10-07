package work

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"local/rag-project/internal/app/work/domain"
	"strings"
)

func (s *Store) ReserveSource(ctx context.Context, user, topic, embedding string, in domain.ReserveSource) (domain.Source, error) {
	if len(in.Name) == 0 || len(in.Name) > 256 {
		return domain.Source{}, fmt.Errorf("source name required, at most 256 bytes")
	}
	if in.SourceType != "file" && in.SourceType != "url" && in.SourceType != "shared" {
		return domain.Source{}, fmt.Errorf("unsupported source type")
	}
	return mutate(s, ctx, user, topic, "source.reserve", in.Mutation, in, func(tx *gorm.DB) (domain.Source, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.Source{}, err
		}
		if err := checkConversation(tx, user, topic, in.ConversationID); err != nil {
			return domain.Source{}, err
		}
		id, err := nextID()
		if err != nil {
			return domain.Source{}, err
		}
		kb := in.KnowledgeBaseID
		status := "reserved"
		if in.SourceType == "shared" {
			var n int64
			if err := tx.Raw(`SELECT COUNT(*) FROM t_knowledge_base WHERE id=? AND work_private=false AND deleted=0`, kb).Scan(&n).Error; err != nil {
				return domain.Source{}, err
			}
			if n != 1 {
				return domain.Source{}, domain.ErrNotFound
			}
			status = "ready"
		} else {
			if kb != "" {
				return domain.Source{}, fmt.Errorf("private source container is server-owned")
			}
			kb, err = nextID()
			if err != nil {
				return domain.Source{}, err
			}
			// One private container per source makes the existing KB filter a precise
			// document filter for every vector/lexical/metadata/parent channel.
			if err := tx.Exec(`INSERT INTO t_knowledge_base(id,name,embedding_model,collection_name,created_by,create_time,update_time,deleted,work_private) VALUES(?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0,true)`, kb, "Work source "+id, embedding, "work_"+id, user).Error; err != nil {
				return domain.Source{}, err
			}
		}
		if err := tx.Exec(`INSERT INTO t_work_source(id,topic_id,user_id,conversation_id,kb_id,name,source_type,source_location,promoted,status) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, topic, user, nullable(in.ConversationID), kb, in.Name, in.SourceType, in.SourceLocation, in.ConversationID == "" && !in.Attachment, status).Error; err != nil {
			return domain.Source{}, err
		}
		return loadSource(tx, user, topic, id)
	})
}

const sourceSelect = `SELECT s.id,s.topic_id,COALESCE(s.conversation_id,'') AS conversation_id,s.kb_id AS knowledge_base_id,COALESCE(s.doc_id,'') AS document_id,s.name,s.source_type,s.source_location,s.promoted,s.status,s.error,s.created_at,COALESCE(d.status,'') AS processing_status,COALESCE(d.chunk_count,0) AS chunk_count,(s.status='ready' AND k.deleted=0 AND (s.source_type='shared' OR (d.deleted=0 AND d.enabled=1 AND d.status IN ('success','partial')))) AS available FROM t_work_source s JOIN t_knowledge_base k ON k.id=s.kb_id LEFT JOIN t_knowledge_document d ON d.id=s.doc_id `

func loadSource(tx *gorm.DB, user, topic, id string) (domain.Source, error) {
	var out domain.Source
	r := tx.Raw(sourceSelect+`WHERE s.id=? AND s.topic_id=? AND s.user_id=?`, id, topic, user).Scan(&out)
	if r.Error != nil {
		return out, r.Error
	}
	if r.RowsAffected != 1 {
		return out, domain.ErrNotFound
	}
	return out, nil
}
func (s *Store) GetSource(ctx context.Context, user, topic, id, conversation string) (domain.Source, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return domain.Source{}, err
	}
	src, err := loadSource(s.db.WithContext(ctx), user, topic, id)
	if err != nil {
		return src, err
	}
	if src.Status == "removed" || src.Status == "cleanup_pending" || src.Status == "cleaned" {
		return src, domain.ErrNotFound
	}
	if !src.Promoted && src.ConversationID != conversation {
		return src, domain.ErrNotFound
	}
	if !src.Promoted {
		if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
			return src, err
		}
	}
	return src, nil
}
func (s *Store) ListSources(ctx context.Context, user, topic, conversation string, page domain.Page) ([]domain.Source, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
		return nil, err
	}
	out := []domain.Source{}
	err := s.db.WithContext(ctx).Raw(sourceSelect+`WHERE s.topic_id=? AND s.user_id=? AND (s.promoted OR s.conversation_id=? OR (?='' AND s.conversation_id IS NULL)) AND s.status NOT IN ('removed','cleanup_pending','cleaned') ORDER BY s.created_at DESC,s.id DESC LIMIT ? OFFSET ?`, topic, user, conversation, conversation, page.Limit, page.Offset).Scan(&out).Error
	return out, err
}
func (s *Store) SourceScope(ctx context.Context, user, topic, conversation string) ([]string, error) {
	if _, err := s.GetTopic(ctx, user, topic); err != nil {
		return nil, err
	}
	if err := checkConversation(s.db.WithContext(ctx), user, topic, conversation); err != nil {
		return nil, err
	}
	ids := []string{}
	err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT s.kb_id FROM t_work_source s JOIN t_knowledge_base k ON k.id=s.kb_id LEFT JOIN t_knowledge_document d ON d.id=s.doc_id WHERE s.topic_id=? AND s.user_id=? AND s.status='ready' AND k.deleted=0 AND (s.promoted OR s.conversation_id=?) AND (s.source_type='shared' OR (d.deleted=0 AND d.enabled=1 AND d.status IN ('success','partial')))`, topic, user, conversation).Scan(&ids).Error
	return ids, err
}
func (s *Store) ClaimSource(ctx context.Context, user, topic, id string) (bool, error) {
	ok := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return err
		}
		r := tx.Exec(`UPDATE t_work_source SET status='uploading',updated_at=CURRENT_TIMESTAMP WHERE id=? AND topic_id=? AND user_id=? AND (status IN ('reserved','failed') OR (status='uploading' AND updated_at<CURRENT_TIMESTAMP-INTERVAL '2 minutes'))`, id, topic, user)
		ok = r.RowsAffected == 1
		return r.Error
	})
	return ok, err
}
func (s *Store) AttachSource(ctx context.Context, user, topic, id, doc, key string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return err
		}
		r := tx.Exec(`UPDATE t_work_source SET doc_id=?,file_key=?,status='ready',error='',updated_at=CURRENT_TIMESTAMP WHERE id=? AND topic_id=? AND user_id=? AND status='uploading'`, doc, key, id, topic, user)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		return nil
	})
}
func (s *Store) FailSource(ctx context.Context, user, id string, err error) error {
	message := err.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.db.WithContext(ctx).Exec(`UPDATE t_work_source SET status='failed',error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND status='uploading'`, message, id, user).Error
}
func (s *Store) ChangeSource(ctx context.Context, user, topic, id string, promote bool, m domain.Mutation) (domain.Source, error) {
	return mutate(s, ctx, user, topic, "source.change/"+id, m, struct{ Promote bool }{promote}, func(tx *gorm.DB) (domain.Source, error) {
		if _, err := activeTopic(tx, user, topic); err != nil {
			return domain.Source{}, err
		}
		src, err := loadSource(tx, user, topic, id)
		if err != nil {
			return src, err
		}
		if strings.Contains(src.Status, "clean") || src.Status == "removed" {
			return src, domain.ErrNotFound
		}
		if promote {
			if err := checkConversation(tx, user, topic, src.ConversationID); err != nil {
				return src, err
			}
			err = tx.Exec(`UPDATE t_work_source SET promoted=true,updated_at=CURRENT_TIMESTAMP WHERE id=?`, id).Error
		} else {
			status := "cleanup_pending"
			if src.SourceType == "shared" {
				status = "removed"
			}
			err = tx.Exec(`UPDATE t_work_source SET status=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, id).Error
		}
		if err != nil {
			return src, err
		}
		return loadSource(tx, user, topic, id)
	})
}
func (s *Store) SourceText(ctx context.Context, user, topic, id, conversation string) (string, error) {
	src, err := s.GetSource(ctx, user, topic, id, conversation)
	if err != nil {
		return "", err
	}
	if !src.Available || src.DocumentID == "" {
		return "", fmt.Errorf("资料尚未处理完成或已不可用")
	}
	var rows []struct{ Content string }
	if err := s.db.WithContext(ctx).Raw(`SELECT LEFT(content,12000) AS content FROM t_knowledge_chunk WHERE doc_id=? AND kb_id=? AND deleted=0 AND enabled=1 AND record_type='parent' ORDER BY chunk_index LIMIT 12`, src.DocumentID, src.KnowledgeBaseID).Scan(&rows).Error; err != nil {
		return "", err
	}
	var b strings.Builder
	if len(rows) == 0 {
		if err := s.db.WithContext(ctx).Raw(`SELECT LEFT(content,12000) AS content FROM t_knowledge_chunk WHERE doc_id=? AND kb_id=? AND deleted=0 AND enabled=1 AND record_type='child' ORDER BY chunk_index LIMIT 12`, src.DocumentID, src.KnowledgeBaseID).Scan(&rows).Error; err != nil {
			return "", err
		}
	}
	for _, r := range rows {
		if b.Len()+len(r.Content) > 60000 {
			break
		}
		b.WriteString(r.Content)
		b.WriteString("\n")
	}
	return b.String(), nil
}
