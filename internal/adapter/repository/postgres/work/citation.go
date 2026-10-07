package work

import (
	"context"
	"local/rag-project/internal/app/work/domain"
	"slices"
)

type Citation struct {
	ID         string `json:"id"`
	DocumentID string `json:"documentId"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	SourceID   string `json:"sourceId,omitempty"`
	Image      bool   `json:"image,omitempty"`
}

func (s *Store) ReadCitation(ctx context.Context, user, topic, conversation, id string) (Citation, error) {
	ids, err := s.SourceScope(ctx, user, topic, conversation)
	if err != nil {
		return Citation{}, err
	}
	var row struct{ ID, KBID, DocID, Content, Title string }
	r := s.db.WithContext(ctx).Raw(`SELECT c.id,c.kb_id,c.doc_id,c.content,d.doc_name AS title FROM t_knowledge_chunk c JOIN t_knowledge_document d ON d.id=c.doc_id WHERE c.id=? AND c.deleted=0 AND c.enabled=1 AND d.deleted=0 AND d.enabled=1`, id).Scan(&row)
	if r.Error != nil {
		return Citation{}, r.Error
	}
	image := false
	if r.RowsAffected == 0 {
		r = s.db.WithContext(ctx).Raw(`SELECT e.id,d.kb_id,d.id AS doc_id,d.doc_name AS title,concat_ws(E'\n',e.ocr_text,e.caption_text,e.adjacent_text) AS content FROM t_knowledge_image_evidence e JOIN t_knowledge_image_occurrence i ON i.id=e.occurrence_id JOIN t_knowledge_document d ON d.id=i.doc_id WHERE e.id=? AND i.deleted_at IS NULL AND d.deleted=0 AND d.enabled=1`, id).Scan(&row)
		if r.Error != nil {
			return Citation{}, r.Error
		}
		image = true
	}
	if r.RowsAffected != 1 || !slices.Contains(ids, row.KBID) {
		return Citation{}, domain.ErrNotFound
	}
	var source string
	if err := s.db.WithContext(ctx).Raw(`SELECT id FROM t_work_source WHERE topic_id=? AND user_id=? AND kb_id=? AND status='ready' AND (promoted OR conversation_id=?) ORDER BY created_at LIMIT 1`, topic, user, row.KBID, conversation).Scan(&source).Error; err != nil {
		return Citation{}, err
	}
	return Citation{ID: row.ID, DocumentID: row.DocID, Title: row.Title, Content: row.Content, SourceID: source, Image: image}, nil
}
