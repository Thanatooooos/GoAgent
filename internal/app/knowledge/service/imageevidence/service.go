package imageevidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/core/vision"
	"local/rag-project/internal/app/knowledge/port"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type Service struct {
	DB              *gorm.DB
	Storage         port.FileStorage
	OCR             parser.OCRClient
	Vision          *vision.Client
	Embedding       aiembedding.EmbeddingService
	SummaryChat     interface{ Chat(string) (string, error) }
	SummaryEnabled  bool
	InventoryParser *parser.DocReaderDocumentParser
	workers         sync.WaitGroup
}

func (s *Service) RefreshStatus(ctx context.Context, documentID, revisionID string) error {
	if s == nil || s.DB == nil || revisionID == "" {
		return nil
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := refreshDocumentStatus(tx, documentID, revisionID); err != nil {
			return err
		}
		return s.enqueueOCRSummary(tx, documentID, revisionID)
	})
}

type RegisterInput struct {
	DocumentID         string
	KnowledgeBaseID    string
	ExpectedRevisionID string
	RevisionID         string
	Stage              bool
	Source             []byte
	Parsed             parser.ParseResult
}

type RegisterResult struct {
	RevisionID string
	ImageCount int
	Failures   int
}

// Register stores immutable image assets before making a new document revision
// active. Failed uploads are recorded per occurrence, so other images proceed.
func (s *Service) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	if s == nil || s.DB == nil || s.Storage == nil {
		return RegisterResult{}, fmt.Errorf("image evidence dependencies are missing")
	}
	if input.DocumentID == "" || input.KnowledgeBaseID == "" {
		return RegisterResult{}, fmt.Errorf("document and knowledge base ids are required")
	}
	revisionID := input.RevisionID
	if revisionID == "" {
		revisionID = uuid.NewString()
	}
	sourceHash := sha256.Sum256(input.Source)
	parserType, _ := input.Parsed.Metadata["parser_type"].(string)
	if parserType == "" {
		parserType = "unknown"
	}
	type registered struct {
		id, key, mime, hash, sourceError string
		processingKey, processingMIME    string
		ordinal                          int
		originalRef                      string
		adjacentText                     string
		page                             int
	}
	registeredImages := make([]registered, 0, len(input.Parsed.Images))
	uploadedKeys := make([]string, 0, len(input.Parsed.Images))
	failures := 0
	for _, image := range input.Parsed.Images {
		entry := registered{id: uuid.NewString(), ordinal: image.Index, originalRef: image.OriginalRef,
			adjacentText: image.AdjacentText, page: image.Page}
		imageData := image.Data
		if len(imageData) == 0 && image.StorageKey != "" &&
			!strings.Contains(image.StorageKey, "://") {
			reader, err := s.Storage.Open(ctx, image.StorageKey)
			if err == nil {
				imageData, err = io.ReadAll(io.LimitReader(reader, 20<<20+1))
				_ = reader.Close()
			}
			if err != nil || len(imageData) > 20<<20 {
				imageData = nil
			}
		}
		if image.Error != "" {
			entry.sourceError = image.Error
		} else if len(imageData) == 0 {
			entry.sourceError = "image bytes unavailable"
		} else {
			entry.mime = strings.TrimSpace(image.MIMEType)
			sniffed := http.DetectContentType(imageData)
			if strings.HasPrefix(sniffed, "image/") {
				entry.mime = sniffed
			}
			if !allowedRasterMIME(entry.mime) || (sniffed != "application/octet-stream" && !strings.HasPrefix(sniffed, "image/")) {
				entry.sourceError = "unsupported image content type"
			} else {
				hash := sha256.Sum256(imageData)
				entry.hash = hex.EncodeToString(hash[:])
				entry.key = fmt.Sprintf("image-evidence/%s/%s/%s/original", input.DocumentID, revisionID, entry.id)
				_, err := s.Storage.Upload(ctx, port.FileUpload{
					Key: entry.key, FileName: image.Filename, ContentType: entry.mime,
					Size: int64(len(imageData)), Body: bytes.NewReader(imageData),
				})
				if err != nil {
					entry.sourceError = "image storage failed"
					entry.key = ""
				} else {
					uploadedKeys = append(uploadedKeys, entry.key)
					prepared, preparedMIME, prepareErr := prepareProcessingImage(imageData, entry.mime)
					if prepareErr != nil {
						entry.sourceError = "image processing copy failed"
					} else {
						entry.processingKey = fmt.Sprintf("image-evidence/%s/%s/%s/processing", input.DocumentID, revisionID, entry.id)
						entry.processingMIME = preparedMIME
						if _, err := s.Storage.Upload(ctx, port.FileUpload{
							Key: entry.processingKey, FileName: image.Filename,
							ContentType: preparedMIME, Size: int64(len(prepared)), Body: bytes.NewReader(prepared),
						}); err != nil {
							entry.processingKey = ""
							entry.sourceError = "image processing storage failed"
						} else {
							uploadedKeys = append(uploadedKeys, entry.processingKey)
						}
					}
				}
			}
		}
		if entry.sourceError != "" {
			failures++
		}
		registeredImages = append(registeredImages, entry)
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document struct {
			ID               string
			ActiveRevisionID sql.NullString
		}
		if input.Stage {
			if err := tx.Raw(`SELECT id, active_revision_id FROM t_knowledge_document
				WHERE id = ? AND kb_id = ? AND deleted = 0 AND status <> 'deleting'
				FOR UPDATE`, input.DocumentID, input.KnowledgeBaseID).Scan(&document).Error; err != nil {
				return err
			}
			if document.ID == "" {
				return fmt.Errorf("document is unavailable for staged registration")
			}
			if err := tx.Exec(`INSERT INTO t_knowledge_image_object_cleanup (object_key, doc_id)
				SELECT object_key, ? FROM (
					SELECT i.original_object_key AS object_key FROM t_knowledge_image_occurrence i
					JOIN t_knowledge_document_revision r ON r.id = i.revision_id
					WHERE r.doc_id = ? AND r.status = 'building'
					UNION SELECT i.processing_object_key FROM t_knowledge_image_occurrence i
					JOIN t_knowledge_document_revision r ON r.id = i.revision_id
					WHERE r.doc_id = ? AND r.status = 'building'
				) keys WHERE object_key IS NOT NULL
				ON CONFLICT (object_key) DO NOTHING`, input.DocumentID, input.DocumentID, input.DocumentID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE t_knowledge_document_revision SET status = 'abandoned'
				WHERE doc_id = ? AND status = 'building'`, input.DocumentID).Error; err != nil {
				return err
			}
		}
		status := "published"
		if input.Stage {
			status = "building"
		}
		if err := tx.Exec(`INSERT INTO t_knowledge_document_revision
			(id, doc_id, source_hash, parser_type, image_inventory_confirmed, status, base_revision_id, published_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'published' THEN now() END)`, revisionID, input.DocumentID,
			hex.EncodeToString(sourceHash[:]), parserType, input.Parsed.ImageInventoryConfirmed,
			status, document.ActiveRevisionID, status).Error; err != nil {
			return err
		}
		for _, image := range registeredImages {
			if err := tx.Exec(`INSERT INTO t_knowledge_image_occurrence
				(id, doc_id, revision_id, ordinal, original_ref, page_number, original_object_key,
				 original_mime_type, original_sha256, processing_object_key,
				 processing_mime_type, source_error, adjacent_text)
				VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''),
				 NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
				image.id, input.DocumentID, revisionID, image.ordinal, image.originalRef, image.page,
				image.key, image.mime, image.hash, image.processingKey, image.processingMIME,
				image.sourceError, image.adjacentText).Error; err != nil {
				return err
			}
			if image.sourceError != "" {
				continue
			}
			for _, operation := range []string{"ocr", "caption"} {
				if err := tx.Exec(`INSERT INTO t_knowledge_image_task
					(id, doc_id, revision_id, occurrence_id, operation) VALUES (?, ?, ?, ?, ?)`,
					uuid.NewString(), input.DocumentID, revisionID, image.id, operation).Error; err != nil {
					return err
				}
			}
		}
		if !input.Parsed.ImageInventoryConfirmed {
			if err := tx.Exec(`INSERT INTO t_knowledge_image_task
				(id, doc_id, revision_id, operation) VALUES (?, ?, ?, 'inventory')`,
				uuid.NewString(), input.DocumentID, revisionID).Error; err != nil {
				return err
			}
		}
		if input.Stage {
			return nil
		}
		updateSQL := `UPDATE t_knowledge_document
			SET active_revision_id = ?, image_count = ?, image_completed_count = 0,
			    image_failed_count = ?, update_time = now()
			WHERE id = ? AND kb_id = ? AND deleted = 0 AND status <> 'deleting'`
		updateArgs := []any{revisionID, len(registeredImages), failures, input.DocumentID, input.KnowledgeBaseID}
		if input.ExpectedRevisionID != "" {
			updateSQL += ` AND active_revision_id = ?`
			updateArgs = append(updateArgs, input.ExpectedRevisionID)
		}
		updated := tx.Exec(updateSQL, updateArgs...)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("document is unavailable for image registration")
		}
		if err := tx.Exec(`DELETE FROM t_knowledge_chunk_vector
			WHERE doc_id = ? AND metadata->>'record_type' = 'image'`, input.DocumentID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// Object storage and PostgreSQL cannot share a transaction. Best effort
		// compensation keeps a failed registration from leaving live assets.
		for _, key := range uploadedKeys {
			if deleteErr := s.Storage.Delete(context.Background(), key); deleteErr != nil {
				_ = s.DB.Exec(`INSERT INTO t_knowledge_image_object_cleanup
					(object_key, doc_id) VALUES (?, ?) ON CONFLICT (object_key) DO NOTHING`,
					key, input.DocumentID).Error
			}
		}
		return RegisterResult{}, fmt.Errorf("register image evidence: %w", err)
	}
	return RegisterResult{RevisionID: revisionID, ImageCount: len(registeredImages), Failures: failures}, nil
}

func (s *Service) AbortStage(ctx context.Context, documentID, revisionID string) error {
	if s == nil || s.DB == nil || revisionID == "" {
		return nil
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Exec(`UPDATE t_knowledge_document_revision SET status = 'abandoned'
			WHERE id = ? AND doc_id = ? AND status = 'building'`, revisionID, documentID)
		if updated.Error != nil || updated.RowsAffected == 0 {
			return updated.Error
		}
		return tx.Exec(`INSERT INTO t_knowledge_image_object_cleanup (object_key, doc_id)
			SELECT object_key, ? FROM (
				SELECT original_object_key AS object_key FROM t_knowledge_image_occurrence WHERE revision_id = ?
				UNION SELECT processing_object_key FROM t_knowledge_image_occurrence WHERE revision_id = ?
			) keys WHERE object_key IS NOT NULL
			ON CONFLICT (object_key) DO NOTHING`, documentID, revisionID, revisionID).Error
	})
}

func allowedRasterMIME(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/tiff":
		return true
	default:
		return false
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(1<<uint(attempt-1)) * 15 * time.Second
}
