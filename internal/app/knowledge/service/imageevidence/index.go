package imageevidence

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/knowledge/port"
)

// indexStagedEvidence publishes OCR/caption results that finished before the
// corresponding text revision became active.
func (s *Service) indexStagedEvidence(ctx context.Context, owner string, claimed task) error {
	var snapshot struct {
		EvidenceID     string
		SearchableText string
		OCRStatus      string
		CaptionStatus  string
		Ordinal        int
		KBID           string
		DocumentName   string
		EmbeddingModel string
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT e.id AS evidence_id,
		COALESCE(e.searchable_text, '') AS searchable_text,
		e.ocr_status, e.caption_status, i.ordinal, d.kb_id,
		d.doc_name AS document_name, b.embedding_model
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id AND d.active_revision_id = i.revision_id
		JOIN t_knowledge_base b ON b.id = d.kb_id AND b.deleted = 0
		JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.id = ? AND i.revision_id = ? AND d.deleted = 0 AND i.deleted_at IS NULL`,
		claimed.OccurrenceID, claimed.RevisionID).Scan(&snapshot).Error; err != nil {
		return s.failTask(ctx, owner, claimed, "index_lookup")
	}
	if snapshot.EvidenceID == "" {
		return s.markTaskError(ctx, owner, claimed, "index_evidence_missing")
	}
	if snapshot.SearchableText == "" || snapshot.EmbeddingModel == "" || s.Embedding == nil {
		return s.markTaskError(ctx, owner, claimed, "index_content_unavailable")
	}
	vector, err := s.Embedding.EmbedWithModel(snapshot.SearchableText, snapshot.EmbeddingModel)
	if err != nil {
		return s.failTask(ctx, owner, claimed, "index_embedding")
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active struct {
			EvidenceID sql.NullString
		}
		if err := tx.Raw(`SELECT i.active_evidence_id AS evidence_id
			FROM t_knowledge_document d
			JOIN t_knowledge_image_occurrence i ON i.doc_id = d.id
			WHERE d.id = ? AND d.active_revision_id = ? AND d.deleted = 0
			AND i.id = ? AND i.deleted_at IS NULL
			FOR UPDATE OF d, i`, claimed.DocumentID, claimed.RevisionID,
			claimed.OccurrenceID).Scan(&active).Error; err != nil {
			return err
		}
		if !active.EvidenceID.Valid {
			return nil
		}
		var leasedTaskID string
		if err := tx.Raw(`SELECT id FROM t_knowledge_image_task WHERE id = ?
			AND status = 'running' AND lease_owner = ? AND lease_until > now() FOR UPDATE`,
			claimed.ID, owner).Scan(&leasedTaskID).Error; err != nil {
			return err
		}
		if leasedTaskID == "" {
			return nil
		}
		if active.EvidenceID.String == snapshot.EvidenceID {
			var published bool
			if err := tx.Raw(`SELECT vector_published FROM t_knowledge_image_evidence WHERE id = ?`,
				snapshot.EvidenceID).Scan(&published).Error; err != nil {
				return err
			}
			if !published {
				if err := pgvectorstore.NewVectorStore(tx).UpsertDocumentChunks(ctx, []port.ChunkVector{{
					ChunkID: snapshot.EvidenceID, DocumentID: claimed.DocumentID,
					KnowledgeBaseID: snapshot.KBID, Index: snapshot.Ordinal,
					Text: snapshot.SearchableText, Embedding: vector,
					Metadata: map[string]any{
						"record_type": "image", "image_evidence_id": snapshot.EvidenceID,
						"document_id": claimed.DocumentID, "document_revision_id": claimed.RevisionID,
						"image_occurrence_id": claimed.OccurrenceID, "image_ordinal": snapshot.Ordinal,
						"document_name": snapshot.DocumentName, "ocr_status": snapshot.OCRStatus,
						"caption_status":    snapshot.CaptionStatus,
						"caption_generated": snapshot.CaptionStatus == "described",
					},
				}}); err != nil {
					return fmt.Errorf("index staged image evidence: %w", err)
				}
				if err := tx.Exec(`UPDATE t_knowledge_image_evidence SET vector_published = TRUE WHERE id = ?`,
					snapshot.EvidenceID).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Exec(`UPDATE t_knowledge_image_task SET status = 'done', lease_owner = NULL,
			lease_until = NULL, error_code = NULL, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, claimed.ID, owner).Error; err != nil {
			return err
		}
		return refreshDocumentStatus(tx, claimed.DocumentID, claimed.RevisionID)
	})
}
