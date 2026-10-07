package imageevidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/port"
)

type task struct {
	ID           string
	DocumentID   string `gorm:"column:doc_id"`
	RevisionID   string
	OccurrenceID string
	Operation    string
	Attempts     int
	MaxAttempts  int
}

type occurrence struct {
	ID                  string
	DocumentID          string `gorm:"column:doc_id"`
	RevisionID          string
	OriginalObjectKey   sql.NullString
	OriginalMimeType    sql.NullString
	ProcessingObjectKey sql.NullString
	ProcessingMimeType  sql.NullString
	ActiveEvidenceID    sql.NullString
	OriginalRef         sql.NullString
	AdjacentText        sql.NullString
	Ordinal             int
	KnowledgeBaseID     string `gorm:"column:kb_id"`
	RevisionStatus      string
}

type evidence struct {
	OCRStatus           string
	OCRText             sql.NullString
	OCRError            sql.NullString
	CaptionStatus       string
	CaptionText         sql.NullString
	CaptionError        sql.NullString
	CaptionModel        sql.NullString
	PromptVersion       sql.NullString
	AdjacentText        sql.NullString
	CaptionInputTokens  int
	CaptionOutputTokens int
	CaptionLatencyMs    int
}

type outcome struct {
	status, text, model, promptVersion, errorCode string
	inputTokens, outputTokens, latencyMs          int
}

// Run polls durable tasks. Expired leases are claimable after a crash.
func (s *Service) Run(ctx context.Context, concurrency int) {
	if concurrency <= 0 {
		concurrency = 2
	}
	for i := 0; i < concurrency; i++ {
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			s.loop(ctx)
		}()
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		s.cleanupLoop(ctx)
	}()
}

func (s *Service) Wait() { s.workers.Wait() }

func (s *Service) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		_ = s.cleanupDeleted(ctx)
		_ = s.cleanupOrphans(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) cleanupOrphans(ctx context.Context) error {
	var items []struct{ ObjectKey string }
	if err := s.DB.WithContext(ctx).Raw(`SELECT object_key FROM t_knowledge_image_object_cleanup
		WHERE status = 'pending' AND next_run_at <= now() LIMIT 20`).Scan(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		if err := s.Storage.Delete(ctx, item.ObjectKey); err != nil {
			_ = s.DB.WithContext(ctx).Exec(`UPDATE t_knowledge_image_object_cleanup
				SET attempts = attempts + 1, next_run_at = now() + interval '5 minutes'
				WHERE object_key = ?`, item.ObjectKey).Error
			continue
		}
		if err := s.DB.WithContext(ctx).Exec(`DELETE FROM t_knowledge_image_object_cleanup
			WHERE object_key = ?`, item.ObjectKey).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) cleanupDeleted(ctx context.Context) error {
	var items []struct {
		ID                  string
		OriginalObjectKey   string
		ProcessingObjectKey string
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.id,
		COALESCE(i.original_object_key, '') AS original_object_key,
		COALESCE(i.processing_object_key, '') AS processing_object_key
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		WHERE d.deleted <> 0 AND i.deleted_at IS NULL
		LIMIT 20`).Scan(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		if item.OriginalObjectKey != "" {
			if err := s.Storage.Delete(ctx, item.OriginalObjectKey); err != nil {
				continue
			}
		}
		if item.ProcessingObjectKey != "" {
			if err := s.Storage.Delete(ctx, item.ProcessingObjectKey); err != nil {
				continue
			}
		}
		if err := s.DB.WithContext(ctx).Exec(`UPDATE t_knowledge_image_occurrence
			SET deleted_at = now() WHERE id = ? AND deleted_at IS NULL`, item.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	owner := uuid.NewString()
	for {
		if ctx.Err() != nil {
			return
		}
		processed, err := s.ProcessOne(ctx, owner)
		if err == nil && processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOne handles one due task and is also used by deterministic integration
// tests without starting a background scheduler.
func (s *Service) ProcessOne(ctx context.Context, owner string) (bool, error) {
	claimed, err := s.claim(ctx, owner)
	if err != nil || claimed.ID == "" {
		return false, err
	}
	return true, s.process(ctx, owner, claimed)
}

func (s *Service) claim(ctx context.Context, owner string) (task, error) {
	var claimed task
	err := s.DB.WithContext(ctx).Raw(`WITH candidate AS (
		SELECT t.id FROM t_knowledge_image_task t
		JOIN t_knowledge_document d ON d.id = t.doc_id
		JOIN t_knowledge_document_revision r ON r.id = t.revision_id
		WHERE d.deleted = 0 AND d.status <> 'deleting'
		  AND ((d.active_revision_id = t.revision_id AND r.status = 'published'
		        AND t.operation IN ('ocr', 'caption', 'inventory', 'index', 'summary'))
		    OR (r.status = 'building' AND t.operation IN ('ocr', 'caption')))
		  AND ((t.status = 'pending' AND t.next_run_at <= now())
		    OR (t.status = 'running' AND t.lease_until < now()))
		ORDER BY t.next_run_at, t.created_at
		FOR UPDATE OF t SKIP LOCKED LIMIT 1
	)
	UPDATE t_knowledge_image_task t
	SET status = 'running', attempts = attempts + 1, lease_owner = ?,
	    lease_until = now() + interval '3 minutes', updated_at = now()
	FROM candidate WHERE t.id = candidate.id
	RETURNING t.id, t.doc_id, t.revision_id, t.occurrence_id,
	          t.operation, t.attempts, t.max_attempts`, owner).Scan(&claimed).Error
	return claimed, err
}

func (s *Service) process(ctx context.Context, owner string, claimed task) error {
	if claimed.Attempts > claimed.MaxAttempts {
		if claimed.Operation == "summary" {
			return s.failOCRSummary(ctx, owner, claimed, "retry_exhausted")
		}
		return s.markTaskError(ctx, owner, claimed, "retry_exhausted")
	}
	if claimed.Operation == "index" {
		return s.indexStagedEvidence(ctx, owner, claimed)
	}
	if claimed.Operation == "summary" {
		return s.processOCRSummary(ctx, owner, claimed)
	}
	if claimed.Operation == "inventory" {
		return s.processInventory(ctx, owner, claimed)
	}
	var image occurrence
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.id, i.doc_id, i.revision_id,
		i.original_object_key, i.original_mime_type, i.processing_object_key,
		i.processing_mime_type, i.active_evidence_id,
		i.original_ref, i.adjacent_text, i.ordinal, d.kb_id
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		JOIN t_knowledge_document_revision r ON r.id = i.revision_id
		WHERE i.id = ? AND i.revision_id = ?
		  AND (d.active_revision_id = i.revision_id OR r.status = 'building')
		  AND d.deleted = 0 AND i.deleted_at IS NULL`, claimed.OccurrenceID, claimed.RevisionID).Scan(&image).Error; err != nil {
		return s.failTask(ctx, owner, claimed, "image_lookup")
	}
	if image.ID == "" || !image.ProcessingObjectKey.Valid {
		return s.failTask(ctx, owner, claimed, "image_unavailable")
	}
	reader, err := s.Storage.Open(ctx, image.ProcessingObjectKey.String)
	if err != nil {
		return s.failTask(ctx, owner, claimed, "image_storage")
	}
	data, err := io.ReadAll(io.LimitReader(reader, 10<<20+1))
	_ = reader.Close()
	if err != nil || len(data) == 0 || len(data) > 10<<20 {
		return s.failTask(ctx, owner, claimed, "image_size")
	}
	var result outcome
	if claimed.Operation == "ocr" {
		if s.OCR == nil {
			return s.failTask(ctx, owner, claimed, "ocr_unconfigured")
		}
		recognized, err := s.OCR.Recognize(ctx, data)
		if err != nil {
			code, _ := parser.ClassifyOCRError(err)
			return s.failTask(ctx, owner, claimed, code)
		}
		result.status, result.text = recognized.Status, recognized.Text
	} else {
		if s.Vision == nil {
			return s.failTask(ctx, owner, claimed, "vision_unconfigured")
		}
		started := time.Now()
		described, err := s.Vision.Describe(ctx, data, image.ProcessingMimeType.String)
		if err != nil {
			code, _ := vision.Classify(err)
			return s.failTask(ctx, owner, claimed, code)
		}
		result.status, result.text = described.Status, described.Text
		result.model, result.promptVersion = described.Model, described.PromptVersion
		result.inputTokens, result.outputTokens = described.InputTokens, described.OutputTokens
		result.latencyMs = int(time.Since(started).Milliseconds())
	}
	if err := s.publish(ctx, owner, claimed, image, result); err != nil {
		return s.failTask(ctx, owner, claimed, "evidence_publish")
	}
	return nil
}

func (s *Service) processInventory(ctx context.Context, owner string, claimed task) error {
	if s.InventoryParser == nil {
		return s.markTaskError(ctx, owner, claimed, "docreader_unconfigured")
	}
	var source struct {
		FileURL string
		DocName string
		KBID    string
		Hash    string
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT d.file_url, d.doc_name,
		d.kb_id, r.source_hash AS hash
		FROM t_knowledge_document d
		JOIN t_knowledge_document_revision r ON r.id = d.active_revision_id
		WHERE d.id = ? AND r.id = ? AND d.deleted = 0 AND d.status <> 'deleting'`,
		claimed.DocumentID, claimed.RevisionID).Scan(&source).Error; err != nil {
		return s.failTask(ctx, owner, claimed, "inventory_source")
	}
	if source.FileURL == "" {
		return s.markTaskError(ctx, owner, claimed, "inventory_source_missing")
	}
	reader, err := s.Storage.Open(ctx, source.FileURL)
	if err != nil {
		return s.failTask(ctx, owner, claimed, "inventory_storage")
	}
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return s.failTask(ctx, owner, claimed, "inventory_read")
	}
	checksum := sha256.Sum256(content)
	if hex.EncodeToString(checksum[:]) != source.Hash {
		return s.markTaskError(ctx, owner, claimed, "inventory_source_changed")
	}
	mimeType := mime.TypeByExtension(filepath.Ext(source.DocName))
	parsed, err := s.InventoryParser.ParseStructured(ctx, content, mimeType,
		map[string]any{"file_name": source.DocName})
	if err != nil || !parsed.ImageInventoryConfirmed {
		return s.failTask(ctx, owner, claimed, "inventory_docreader")
	}
	registered, err := s.Register(ctx, RegisterInput{
		DocumentID: claimed.DocumentID, KnowledgeBaseID: source.KBID,
		ExpectedRevisionID: claimed.RevisionID, Source: content, Parsed: parsed,
	})
	if err != nil {
		return s.failTask(ctx, owner, claimed, "inventory_register")
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE t_knowledge_image_task
			SET status = 'done', lease_owner = NULL, lease_until = NULL,
			    error_code = NULL, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, claimed.ID, owner).Error; err != nil {
			return err
		}
		return refreshDocumentStatus(tx, claimed.DocumentID, registered.RevisionID)
	})
}

func (s *Service) failTask(ctx context.Context, owner string, claimed task, code string) error {
	if claimed.Attempts < claimed.MaxAttempts && !permanentTaskError(code) {
		return s.DB.WithContext(ctx).Exec(`UPDATE t_knowledge_image_task
			SET status = 'pending', next_run_at = ?, lease_owner = NULL,
			    lease_until = NULL, error_code = ?, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`,
			time.Now().Add(retryDelay(claimed.Attempts)), code, claimed.ID, owner).Error
	}
	if claimed.Operation == "inventory" || claimed.Operation == "index" {
		return s.markTaskError(ctx, owner, claimed, code)
	}
	var image occurrence
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.id, i.doc_id, i.revision_id,
		i.original_object_key, i.original_mime_type, i.processing_object_key,
		i.processing_mime_type, i.active_evidence_id,
		i.original_ref, i.adjacent_text, i.ordinal, d.kb_id
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		WHERE i.id = ?`, claimed.OccurrenceID).Scan(&image).Error; err != nil {
		return err
	}
	if image.ID == "" {
		return errors.New("image occurrence disappeared")
	}
	if err := s.publish(ctx, owner, claimed, image, outcome{status: "error", errorCode: code}); err != nil {
		return s.markTaskError(ctx, owner, claimed, code)
	}
	return nil
}

func permanentTaskError(code string) bool {
	switch code {
	case "ocr_unconfigured", "ocr_auth", "ocr_request_invalid", "ocr_image_size",
		"vision_unconfigured", "vision_auth",
		"vision_model_unavailable", "vision_request_invalid", "vision_image_size",
		"vision_image_format", "image_size", "image_unavailable":
		return true
	default:
		return false
	}
}

// A failed vector publication must leave the previously published evidence
// searchable. The task still becomes terminal after its finite retry budget.
func (s *Service) markTaskError(ctx context.Context, owner string, claimed task, code string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`UPDATE t_knowledge_image_task
			SET status = 'error', lease_owner = NULL, lease_until = NULL,
			    error_code = ?, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, code, claimed.ID, owner)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return nil
		}
		return refreshDocumentStatus(tx, claimed.DocumentID, claimed.RevisionID)
	})
}

func (s *Service) publish(ctx context.Context, owner string, claimed task, image occurrence, result outcome) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document struct {
			ID               string
			ActiveRevisionID sql.NullString
		}
		if err := tx.Raw(`SELECT id, active_revision_id FROM t_knowledge_document
			WHERE id = ? AND deleted = 0 AND status <> 'deleting' FOR UPDATE`,
			image.DocumentID).Scan(&document).Error; err != nil {
			return err
		}
		if document.ID == "" {
			return nil
		}
		var revision struct {
			ID     string
			Status string
		}
		if err := tx.Raw(`SELECT id, status FROM t_knowledge_document_revision
			WHERE id = ? AND doc_id = ? FOR UPDATE`, claimed.RevisionID, image.DocumentID).Scan(&revision).Error; err != nil {
			return err
		}
		staged := revision.Status == "building"
		if revision.ID == "" || (!staged && (revision.Status != "published" ||
			!document.ActiveRevisionID.Valid || document.ActiveRevisionID.String != claimed.RevisionID)) {
			return nil
		}
		var current occurrence
		if err := tx.Raw(`SELECT i.id, i.doc_id, i.revision_id, i.active_evidence_id,
			i.ordinal, i.adjacent_text, d.kb_id FROM t_knowledge_image_occurrence i
			JOIN t_knowledge_document d ON d.id = i.doc_id
			WHERE i.id = ? AND i.revision_id = ? AND i.deleted_at IS NULL
			FOR UPDATE OF i`, image.ID, claimed.RevisionID).Scan(&current).Error; err != nil {
			return err
		}
		if current.ID == "" {
			return nil // stale revision or deleted document
		}
		var leasedTaskID string
		if err := tx.Raw(`SELECT id FROM t_knowledge_image_task
			WHERE id = ? AND status = 'running' AND lease_owner = ? AND lease_until > now()
			FOR UPDATE`,
			claimed.ID, owner).Scan(&leasedTaskID).Error; err != nil {
			return err
		}
		if leasedTaskID == "" {
			return nil
		}
		previous := evidence{OCRStatus: "pending", CaptionStatus: "pending"}
		if current.ActiveEvidenceID.Valid {
			if err := tx.Raw(`SELECT ocr_status, ocr_text, ocr_error, caption_status,
				caption_text, caption_error, caption_model, prompt_version,
				caption_input_tokens, caption_output_tokens, caption_latency_ms
				FROM t_knowledge_image_evidence WHERE id = ?`, current.ActiveEvidenceID.String).Scan(&previous).Error; err != nil {
				return err
			}
		}
		if claimed.Operation == "ocr" {
			previous.OCRStatus = result.status
			previous.OCRText = nullText(result.text)
			previous.OCRError = nullText(result.errorCode)
		} else {
			previous.CaptionStatus = result.status
			previous.CaptionText = nullText(result.text)
			previous.CaptionError = nullText(result.errorCode)
			previous.CaptionModel = nullText(result.model)
			previous.PromptVersion = nullText(result.promptVersion)
			previous.CaptionInputTokens = result.inputTokens
			previous.CaptionOutputTokens = result.outputTokens
			previous.CaptionLatencyMs = result.latencyMs
		}
		previous.AdjacentText = current.AdjacentText
		searchText := searchableText(previous)
		id := uuid.NewString()
		if err := tx.Exec(`INSERT INTO t_knowledge_image_evidence
			(id, occurrence_id, revision_id, ocr_status, ocr_text, ocr_error,
			 caption_status, caption_text, caption_error, caption_model,
			 prompt_version, caption_input_tokens, caption_output_tokens,
			 caption_latency_ms, adjacent_text, searchable_text, published_at)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''),
			 NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), now())`,
			id, image.ID, claimed.RevisionID, previous.OCRStatus, previous.OCRText.String,
			previous.OCRError.String, previous.CaptionStatus, previous.CaptionText.String,
			previous.CaptionError.String, previous.CaptionModel.String,
			previous.PromptVersion.String, previous.CaptionInputTokens,
			previous.CaptionOutputTokens, previous.CaptionLatencyMs,
			previous.AdjacentText.String, searchText).Error; err != nil {
			return err
		}
		if searchText != "" && !staged {
			var embeddingModel string
			if err := tx.Raw(`SELECT embedding_model FROM t_knowledge_base WHERE id = ? AND deleted = 0`,
				current.KnowledgeBaseID).Scan(&embeddingModel).Error; err != nil {
				return err
			}
			var documentName string
			if err := tx.Raw(`SELECT doc_name FROM t_knowledge_document WHERE id = ? AND deleted = 0`,
				image.DocumentID).Scan(&documentName).Error; err != nil {
				return err
			}
			if embeddingModel == "" || s.Embedding == nil {
				return fmt.Errorf("image evidence embedding unavailable")
			}
			vector, err := s.Embedding.EmbedWithModel(searchText, embeddingModel)
			if err != nil {
				return fmt.Errorf("embed image evidence: %w", err)
			}
			if err := pgvectorstore.NewVectorStore(tx).UpsertDocumentChunks(ctx, []port.ChunkVector{{
				ChunkID: id, DocumentID: image.DocumentID, KnowledgeBaseID: current.KnowledgeBaseID,
				Index: image.Ordinal, Text: searchText, Embedding: vector,
				Metadata: map[string]any{"record_type": "image", "image_evidence_id": id,
					"document_id": image.DocumentID, "document_revision_id": claimed.RevisionID,
					"image_occurrence_id": image.ID, "image_ordinal": image.Ordinal,
					"document_name": documentName,
					"ocr_status":    previous.OCRStatus, "caption_status": previous.CaptionStatus,
					"caption_generated": previous.CaptionStatus == "described"},
			}}); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_knowledge_image_evidence SET vector_published = TRUE WHERE id = ?`, id).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE t_knowledge_image_occurrence SET active_evidence_id = ? WHERE id = ?`, id, image.ID).Error; err != nil {
			return err
		}
		if current.ActiveEvidenceID.Valid && !staged {
			if err := tx.Exec(`DELETE FROM t_knowledge_chunk_vector WHERE chunk_id = ?`, current.ActiveEvidenceID.String).Error; err != nil {
				return err
			}
		}
		updated := tx.Exec(`UPDATE t_knowledge_image_task SET status = 'done', lease_owner = NULL,
			lease_until = NULL, error_code = NULL, updated_at = now()
			WHERE id = ? AND status = 'running' AND lease_owner = ?`, claimed.ID, owner)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("image task lease changed during publication")
		}
		if staged {
			return nil
		}
		if err := refreshDocumentStatus(tx, image.DocumentID, claimed.RevisionID); err != nil {
			return err
		}
		if claimed.Operation == "ocr" {
			return s.enqueueOCRSummary(tx, image.DocumentID, claimed.RevisionID)
		}
		return nil
	})
}

func nullText(text string) sql.NullString {
	return sql.NullString{String: text, Valid: text != ""}
}

func searchableText(value evidence) string {
	var parts []string
	if value.OCRStatus == "success" && strings.TrimSpace(value.OCRText.String) != "" {
		parts = append(parts, "[图片 OCR 原文] "+strings.TrimSpace(value.OCRText.String))
	}
	if value.CaptionStatus == "described" && strings.TrimSpace(value.CaptionText.String) != "" {
		parts = append(parts, "[AI 生成的图片描述] "+strings.TrimSpace(value.CaptionText.String))
	}
	if len(parts) > 0 && strings.TrimSpace(value.AdjacentText.String) != "" {
		parts = append(parts, "[相邻正文] "+strings.TrimSpace(value.AdjacentText.String))
	}
	return strings.Join(parts, "\n")
}

func refreshDocumentStatus(tx *gorm.DB, documentID, revisionID string) error {
	var counts struct {
		Total              int
		Pending            int
		Failed             int
		Completed          int
		Available          int
		InventoryConfirmed bool
	}
	if err := tx.Raw(`SELECT
		(SELECT count(*) FROM t_knowledge_image_occurrence WHERE doc_id = ? AND revision_id = ?) AS total,
		(SELECT count(*) FROM t_knowledge_image_task WHERE doc_id = ? AND revision_id = ? AND operation <> 'summary' AND status IN ('pending','running')) AS pending,
		(SELECT count(*) FROM t_knowledge_image_occurrence i
		 LEFT JOIN t_knowledge_image_evidence e ON i.active_evidence_id = e.id
		 WHERE i.doc_id = ? AND i.revision_id = ? AND
		   (i.source_error IS NOT NULL OR e.ocr_status = 'error' OR e.caption_status = 'error'
		    OR EXISTS (SELECT 1 FROM t_knowledge_image_task t
		      WHERE t.occurrence_id = i.id AND t.status = 'error'))) AS failed,
		(SELECT count(*) FROM t_knowledge_image_occurrence WHERE doc_id = ? AND revision_id = ? AND active_evidence_id IS NOT NULL) AS completed,
		(SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ?) AS available,
		(SELECT image_inventory_confirmed FROM t_knowledge_document_revision WHERE id = ?) AS inventory_confirmed`,
		documentID, revisionID, documentID, revisionID, documentID, revisionID,
		documentID, revisionID, documentID, revisionID).Scan(&counts).Error; err != nil {
		return err
	}
	status := "partial"
	if counts.InventoryConfirmed && counts.Pending > 0 {
		status = "running"
	} else if counts.Available == 0 {
		status = "failed"
	} else if counts.Pending == 0 && counts.Failed == 0 && counts.InventoryConfirmed {
		status = "success"
	}
	return tx.Exec(`UPDATE t_knowledge_document SET status = ?, image_count = ?,
		image_completed_count = ?, image_failed_count = ?, update_time = now()
		WHERE id = ? AND active_revision_id = ? AND deleted = 0 AND status <> 'deleting'
		AND NOT EXISTS(SELECT 1 FROM t_document_chunk_job j WHERE j.id=t_knowledge_document.current_chunk_job_id AND j.state IN ('pending','running'))`,
		status, counts.Total, counts.Completed, counts.Failed, documentID, revisionID).Error
}
