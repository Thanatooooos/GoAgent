package imageevidence

import (
	"context"
	"fmt"
	"io"

	"gorm.io/gorm"
)

type Detail struct {
	ID                  string `json:"id"`
	DocumentID          string `json:"docId" gorm:"column:doc_id"`
	KnowledgeBaseID     string `json:"kbId" gorm:"column:kb_id"`
	Ordinal             int    `json:"ordinal"`
	Page                int    `json:"page" gorm:"column:page_number"`
	OCRStatus           string `json:"ocrStatus"`
	OCRText             string `json:"ocrText"`
	CaptionStatus       string `json:"captionStatus"`
	CaptionText         string `json:"captionText"`
	CaptionModel        string `json:"captionModel"`
	CaptionInputTokens  int    `json:"captionInputTokens"`
	CaptionOutputTokens int    `json:"captionOutputTokens"`
	CaptionLatencyMs    int    `json:"captionLatencyMs"`
	AdjacentText        string `json:"adjacentText"`
	OriginalMIME        string `json:"originalMime" gorm:"column:original_mime_type"`
	ObjectKey           string `json:"-" gorm:"column:original_object_key"`
}

type Item struct {
	OccurrenceID  string `json:"occurrenceId"`
	EvidenceID    string `json:"evidenceId"`
	Ordinal       int    `json:"ordinal"`
	SourceError   string `json:"sourceError"`
	OCRStatus     string `json:"ocrStatus"`
	CaptionStatus string `json:"captionStatus"`
	OCRError      string `json:"ocrError"`
	CaptionError  string `json:"captionError"`
}

func (s *Service) List(ctx context.Context, documentID string) ([]Item, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("image evidence repository is unavailable")
	}
	var items []Item
	err := s.DB.WithContext(ctx).Raw(`SELECT i.id AS occurrence_id,
		COALESCE(e.id, '') AS evidence_id, i.ordinal,
		COALESCE(i.source_error, '') AS source_error,
		CASE WHEN EXISTS (SELECT 1 FROM t_knowledge_image_task t
		  WHERE t.occurrence_id = i.id AND t.operation = 'ocr' AND t.status = 'error')
		  THEN 'error' ELSE COALESCE(e.ocr_status, 'pending') END AS ocr_status,
		CASE WHEN EXISTS (SELECT 1 FROM t_knowledge_image_task t
		  WHERE t.occurrence_id = i.id AND t.operation = 'caption' AND t.status = 'error')
		  THEN 'error' ELSE COALESCE(e.caption_status, 'pending') END AS caption_status,
		COALESCE(e.ocr_error, (SELECT t.error_code FROM t_knowledge_image_task t
		  WHERE t.occurrence_id = i.id AND t.operation = 'ocr' AND t.status = 'error'
		  ORDER BY t.updated_at DESC LIMIT 1), '') AS ocr_error,
		COALESCE(e.caption_error, (SELECT t.error_code FROM t_knowledge_image_task t
		  WHERE t.occurrence_id = i.id AND t.operation = 'caption' AND t.status = 'error'
		  ORDER BY t.updated_at DESC LIMIT 1), '') AS caption_error
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		LEFT JOIN t_knowledge_image_evidence e ON e.id = i.active_evidence_id
		WHERE i.doc_id = ? AND i.revision_id = d.active_revision_id AND d.deleted = 0
		ORDER BY i.ordinal`, documentID).Scan(&items).Error
	return items, err
}

func (s *Service) Get(ctx context.Context, evidenceID string) (Detail, error) {
	if s == nil || s.DB == nil {
		return Detail{}, fmt.Errorf("image evidence repository is unavailable")
	}
	var detail Detail
	err := s.DB.WithContext(ctx).Raw(`SELECT e.id, i.doc_id, d.kb_id, i.ordinal,
		COALESCE(i.page_number, 0) AS page_number, e.ocr_status,
		COALESCE(e.ocr_text, '') AS ocr_text, e.caption_status,
		COALESCE(e.caption_text, '') AS caption_text,
		COALESCE(e.caption_model, '') AS caption_model,
		e.caption_input_tokens, e.caption_output_tokens, e.caption_latency_ms,
		COALESCE(e.adjacent_text, '') AS adjacent_text,
		COALESCE(i.original_mime_type, '') AS original_mime_type,
		COALESCE(i.original_object_key, '') AS original_object_key
		FROM t_knowledge_image_evidence e
		JOIN t_knowledge_image_occurrence i ON i.id = e.occurrence_id
		JOIN t_knowledge_document d ON d.id = i.doc_id
		JOIN t_knowledge_base b ON b.id = d.kb_id
		WHERE e.id = ? AND i.deleted_at IS NULL AND d.deleted = 0 AND b.deleted = 0`,
		evidenceID).Scan(&detail).Error
	if err != nil {
		return Detail{}, err
	}
	if detail.ID == "" {
		return Detail{}, gorm.ErrRecordNotFound
	}
	return detail, nil
}

func (s *Service) OpenOriginal(ctx context.Context, evidenceID string) ([]byte, string, error) {
	detail, err := s.Get(ctx, evidenceID)
	if err != nil {
		return nil, "", err
	}
	if detail.ObjectKey == "" {
		return nil, "", gorm.ErrRecordNotFound
	}
	return s.openObject(ctx, detail.ObjectKey, detail.OriginalMIME)
}

// OpenOccurrenceOriginal is for the admin review list. It can show an
// extracted original even before OCR or captioning has published evidence.
func (s *Service) OpenOccurrenceOriginal(ctx context.Context, occurrenceID string) ([]byte, string, error) {
	var item struct {
		ObjectKey string `gorm:"column:original_object_key"`
		MIMEType  string `gorm:"column:original_mime_type"`
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT COALESCE(i.original_object_key, '') AS original_object_key,
		COALESCE(i.original_mime_type, '') AS original_mime_type
		FROM t_knowledge_image_occurrence i
		JOIN t_knowledge_document d ON d.id = i.doc_id
		JOIN t_knowledge_base b ON b.id = d.kb_id
		WHERE i.id = ? AND i.revision_id = d.active_revision_id
		  AND i.deleted_at IS NULL AND d.deleted = 0 AND b.deleted = 0`,
		occurrenceID).Scan(&item).Error; err != nil {
		return nil, "", err
	}
	if item.ObjectKey == "" {
		return nil, "", gorm.ErrRecordNotFound
	}
	return s.openObject(ctx, item.ObjectKey, item.MIMEType)
}

func (s *Service) openObject(ctx context.Context, objectKey, mimeType string) ([]byte, string, error) {
	reader, err := s.Storage.Open(ctx, objectKey)
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, 20<<20+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > 20<<20 {
		return nil, "", fmt.Errorf("image exceeds response size limit")
	}
	return data, mimeType, nil
}
