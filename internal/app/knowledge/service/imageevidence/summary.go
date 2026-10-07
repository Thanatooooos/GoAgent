package imageevidence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// enqueueOCRSummary is called after OCR settles and after a staged revision is
// activated. The OCR content hash deduplicates work while allowing a retry to
// replace an earlier OCR result. Captions never enter the input.
func (s *Service) enqueueOCRSummary(tx *gorm.DB, documentID, revisionID string) error {
	if !s.SummaryEnabled || s.SummaryChat == nil {
		return nil
	}
	return tx.Exec(`INSERT INTO t_knowledge_image_task
		(id, doc_id, revision_id, operation)
		SELECT ?, ?, ?, 'summary'
		WHERE EXISTS (SELECT 1 FROM t_knowledge_document d
			JOIN t_knowledge_document_revision r ON r.id = d.active_revision_id
			WHERE d.id = ? AND d.active_revision_id = ? AND d.deleted = 0 AND r.status = 'published')
		AND NOT EXISTS (SELECT 1 FROM t_knowledge_image_task
			WHERE revision_id = ? AND operation = 'ocr' AND status IN ('pending', 'running'))
		AND EXISTS (SELECT 1 FROM t_knowledge_image_occurrence i
			JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
			WHERE i.revision_id = ? AND e.ocr_status = 'success' AND COALESCE(e.ocr_text, '') <> '')
		AND EXISTS (SELECT 1 FROM t_knowledge_document_revision r WHERE r.id = ?
			AND COALESCE(r.ocr_summary_hash, '') <> (
			 SELECT COALESCE(md5(string_agg(i.id || ':' || e.ocr_status || ':' || COALESCE(e.ocr_text, ''),
			 '|' ORDER BY i.ordinal, i.id)), '')
			 FROM t_knowledge_image_occurrence i
			 JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
			 WHERE i.revision_id = ?))
		AND NOT EXISTS (SELECT 1 FROM t_knowledge_image_task
			WHERE revision_id = ? AND operation = 'summary' AND status IN ('pending', 'running'))
		ON CONFLICT DO NOTHING`, uuid.NewString(), documentID, revisionID,
		documentID, revisionID, revisionID, revisionID, revisionID, revisionID, revisionID).Error
}

func ocrSummaryHash(tx *gorm.DB, revisionID string) (string, error) {
	var hash string
	err := tx.Raw(`SELECT COALESCE(md5(string_agg(i.id || ':' || e.ocr_status || ':' || COALESCE(e.ocr_text, ''),
		'|' ORDER BY i.ordinal, i.id)), '')
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.revision_id = ?`, revisionID).Scan(&hash).Error
	return hash, err
}

func (s *Service) processOCRSummary(ctx context.Context, owner string, claimed task) error {
	if !s.SummaryEnabled || s.SummaryChat == nil {
		return s.failOCRSummary(ctx, owner, claimed, "summary_unconfigured")
	}
	var chunks []struct{ Content string }
	if err := s.DB.WithContext(ctx).Raw(`SELECT content FROM t_knowledge_chunk c
		WHERE doc_id = ? AND deleted = 0 AND
		 (record_type = 'parent' OR NOT EXISTS
		  (SELECT 1 FROM t_knowledge_chunk p WHERE p.doc_id = c.doc_id AND p.deleted = 0 AND p.record_type = 'parent'))
		ORDER BY chunk_index`, claimed.DocumentID).Scan(&chunks).Error; err != nil {
		return s.failOCRSummary(ctx, owner, claimed, "summary_body_read")
	}
	var images []struct {
		Ordinal int
		OCRText string
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.ordinal, e.ocr_text
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.revision_id = ? AND e.ocr_status = 'success' AND COALESCE(e.ocr_text, '') <> ''
		ORDER BY i.ordinal`, claimed.RevisionID).Scan(&images).Error; err != nil {
		return s.failOCRSummary(ctx, owner, claimed, "summary_ocr_read")
	}
	if len(images) == 0 {
		return s.completeOCRSummary(ctx, owner, claimed, "", "", false)
	}
	inputHash, err := ocrSummaryHash(s.DB.WithContext(ctx), claimed.RevisionID)
	if err != nil {
		return s.failOCRSummary(ctx, owner, claimed, "summary_hash_read")
	}
	var body strings.Builder
	for _, chunk := range chunks {
		body.WriteString(chunk.Content)
		body.WriteByte('\n')
	}
	var ocr strings.Builder
	for _, image := range images {
		fmt.Fprintf(&ocr, imageOCRTextTemplate, image.Ordinal, image.OCRText)
	}
	prompt := imageOCRSummaryInstruction + limitSummaryRunes(body.String(), 9000) +
		imageOCRSummaryTextHeader + limitSummaryRunes(ocr.String(), 3000)
	response, err := s.SummaryChat.Chat(prompt)
	if err != nil || strings.TrimSpace(response) == "" {
		return s.failOCRSummary(ctx, owner, claimed, "summary_chat")
	}
	return s.completeOCRSummary(ctx, owner, claimed, strings.TrimSpace(response), inputHash, true)
}

func (s *Service) completeOCRSummary(ctx context.Context, owner string, claimed task, summary, inputHash string, update bool) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document struct {
			ID               string
			ActiveRevisionID sql.NullString
		}
		if err := tx.Raw(`SELECT id, active_revision_id FROM t_knowledge_document
			WHERE id = ? AND deleted = 0 FOR UPDATE`, claimed.DocumentID).Scan(&document).Error; err != nil {
			return err
		}
		if document.ID == "" || !document.ActiveRevisionID.Valid || document.ActiveRevisionID.String != claimed.RevisionID {
			return tx.Exec(`UPDATE t_knowledge_image_task SET status = 'done', lease_owner = NULL,
				lease_until = NULL, updated_at = now() WHERE id = ? AND status = 'running' AND lease_owner = ?`,
				claimed.ID, owner).Error
		}
		var leased string
		if err := tx.Raw(`SELECT id FROM t_knowledge_image_task WHERE id = ? AND status = 'running'
			AND lease_owner = ? AND lease_until > now() FOR UPDATE`, claimed.ID, owner).Scan(&leased).Error; err != nil {
			return err
		}
		if leased == "" {
			return nil
		}
		currentHash, err := ocrSummaryHash(tx, claimed.RevisionID)
		if err != nil {
			return err
		}
		if currentHash != inputHash {
			update = false
		}
		if update {
			if err := tx.Exec(`UPDATE t_knowledge_document SET summary = ?, summary_status = 'success',
				summary_error_message = NULL, update_time = now()
				WHERE id = ? AND active_revision_id = ?`, summary, claimed.DocumentID, claimed.RevisionID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_knowledge_chunk_vector SET metadata =
				jsonb_set(COALESCE(metadata, '{}'::jsonb), '{document_summary}', to_jsonb(?::text), true)
				WHERE doc_id = ? AND metadata->>'record_type' IN ('child', 'parent')`,
				summary, claimed.DocumentID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_knowledge_document_revision SET ocr_summary_hash = ?
				WHERE id = ?`, inputHash, claimed.RevisionID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE t_knowledge_image_task SET status = 'done', lease_owner = NULL,
			lease_until = NULL, error_code = NULL, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, claimed.ID, owner).Error; err != nil {
			return err
		}
		if currentHash != inputHash {
			return s.enqueueOCRSummary(tx, claimed.DocumentID, claimed.RevisionID)
		}
		return nil
	})
}

func (s *Service) failOCRSummary(ctx context.Context, owner string, claimed task, code string) error {
	if claimed.Attempts < claimed.MaxAttempts && code != "summary_unconfigured" {
		return s.DB.WithContext(ctx).Exec(`UPDATE t_knowledge_image_task SET status = 'pending',
			next_run_at = ?, lease_owner = NULL, lease_until = NULL, error_code = ?, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`,
			time.Now().Add(retryDelay(claimed.Attempts)), code, claimed.ID, owner).Error
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`UPDATE t_knowledge_image_task SET status = 'error', lease_owner = NULL,
			lease_until = NULL, error_code = ?, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, code, claimed.ID, owner)
		if updated.Error != nil || updated.RowsAffected == 0 {
			return updated.Error
		}
		return tx.Exec(`UPDATE t_knowledge_document SET summary_status = 'degraded',
			summary_error_message = ? WHERE id = ? AND active_revision_id = ?`,
			code, claimed.DocumentID, claimed.RevisionID).Error
	})
}

func limitSummaryRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
