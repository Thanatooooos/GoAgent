package knowledge

import (
	"context"

	"gorm.io/gorm"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

// Legacy remote-fetch status writes cannot touch a durable chunk execution.
func (r *KnowledgeDocumentRepository) UpdateUnownedChunkFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	return NewKnowledgeDocumentRepository(r.db.Where("current_chunk_job_id IS NULL"), nil).UpdateFields(ctx, where, set)
}
func (r *KnowledgeDocumentRepository) TryMarkUnownedRunning(ctx context.Context, id string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`UPDATE t_knowledge_document d SET status='running',current_chunk_job_id=NULL,chunk_epoch=chunk_epoch+1,updated_by='system',update_time=clock_timestamp() WHERE d.id=? AND d.enabled=1 AND d.deleted=0 AND d.status IN ('pending','failed','success','partial') AND NOT EXISTS(SELECT 1 FROM t_document_chunk_job j WHERE j.document_id=d.id AND j.state IN ('pending','running'))`, id)
	return result.RowsAffected > 0, result.Error
}

// Capture the pre-processing occupation under the same lock that creates it.
// A delayed refresh must not adopt another refresh's running document.
func (r *KnowledgeDocumentRepository) ClaimRemoteRefresh(ctx context.Context, id string) (document domain.KnowledgeDocument, occupied bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChunkDocument(tx, id); err != nil {
			return err
		}
		repo := NewKnowledgeDocumentRepository(tx, nil)
		var err error
		occupied, err = repo.TryMarkUnownedRunning(ctx, id)
		if err != nil || !occupied {
			return err
		}
		document, err = repo.GetByID(ctx, id)
		return err
	})
	return
}

func (r *KnowledgeDocumentRepository) CanDiscardRefreshedFile(ctx context.Context, documentID, fileURL string) (bool, error) {
	var referenced bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM t_knowledge_document WHERE id=? AND file_url=?) OR EXISTS(SELECT 1 FROM t_document_chunk_job WHERE document_id=? AND state IN ('pending','running') AND document_snapshot->>'FileURL'=?)`, documentID, fileURL, documentID, fileURL).Scan(&referenced).Error
	return !referenced, err
}
