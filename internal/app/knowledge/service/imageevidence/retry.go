package imageevidence

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RetryFailed preserves the evidence-id API used by existing clients.
func (s *Service) RetryFailed(ctx context.Context, evidenceID string) (int, error) {
	if _, err := s.Get(ctx, evidenceID); err != nil {
		return 0, err
	}
	var occurrenceID string
	if err := s.DB.WithContext(ctx).Raw(`SELECT occurrence_id FROM t_knowledge_image_evidence
		WHERE id = ?`, evidenceID).Scan(&occurrenceID).Error; err != nil {
		return 0, err
	}
	return s.RetryOccurrence(ctx, occurrenceID)
}

// RetryOccurrence requeues failed OCR and caption operations, including tasks
// that failed before their first evidence version could be published.
func (s *Service) RetryOccurrence(ctx context.Context, occurrenceID string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, fmt.Errorf("image evidence repository is unavailable")
	}
	var item struct {
		DocumentID    string `gorm:"column:doc_id"`
		RevisionID    string
		SourceError   *string
		OCRStatus     string
		CaptionStatus string
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.doc_id, i.revision_id, i.source_error,
		COALESCE(e.ocr_status, 'pending') AS ocr_status,
		COALESCE(e.caption_status, 'pending') AS caption_status
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		JOIN t_knowledge_base b ON b.id = d.kb_id
		LEFT JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.id = ? AND i.revision_id = d.active_revision_id
		  AND i.deleted_at IS NULL AND d.deleted = 0 AND b.deleted = 0`,
		occurrenceID).Scan(&item).Error; err != nil {
		return 0, err
	}
	if item.RevisionID == "" {
		return 0, gorm.ErrRecordNotFound
	}
	if item.SourceError != nil {
		return 0, fmt.Errorf("image source is unavailable for retry")
	}
	count := 0
	for operation, status := range map[string]string{"ocr": item.OCRStatus, "caption": item.CaptionStatus} {
		var taskErrors int64
		if err := s.DB.WithContext(ctx).Raw(`SELECT count(*) FROM t_knowledge_image_task
			WHERE revision_id = ? AND occurrence_id = ? AND operation = ? AND status = 'error'`,
			item.RevisionID, occurrenceID, operation).Scan(&taskErrors).Error; err != nil {
			return count, err
		}
		if status != "error" && taskErrors == 0 {
			continue
		}
		result := s.DB.WithContext(ctx).Exec(`INSERT INTO t_knowledge_image_task
			(id, doc_id, revision_id, occurrence_id, operation)
			SELECT ?, ?, ?, ?, ? WHERE NOT EXISTS (
				SELECT 1 FROM t_knowledge_image_task
				WHERE revision_id = ? AND occurrence_id = ? AND operation = ?
				  AND status IN ('pending','running'))`,
			uuid.NewString(), item.DocumentID, item.RevisionID, occurrenceID, operation,
			item.RevisionID, occurrenceID, operation)
		if result.Error != nil {
			return count, result.Error
		}
		count += int(result.RowsAffected)
		if result.RowsAffected > 0 {
			if err := s.DB.WithContext(ctx).Exec(`UPDATE t_knowledge_image_task
				SET status = 'retried', updated_at = now()
				WHERE revision_id = ? AND occurrence_id = ? AND operation = ? AND status = 'error'`,
				item.RevisionID, occurrenceID, operation).Error; err != nil {
				return count, err
			}
		}
	}
	return count, nil
}
