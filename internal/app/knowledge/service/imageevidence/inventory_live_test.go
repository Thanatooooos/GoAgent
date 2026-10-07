package imageevidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	postgres "local/rag-project/internal/adapter/repository/postgres"
	"local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/framework/config"
)

func TestImageInventoryRetryLive(t *testing.T) {
	if os.Getenv("IMAGE_EVIDENCE_LIVE") != "1" || os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set IMAGE_EVIDENCE_LIVE=1 and DOCREADER_LIVE=1")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	db, err := postgres.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	if err := postgres.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader", "text.png"))
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := parser.NewDefaultSelector(nil).Select(parser.ParserTypeDocReader)
	if !ok {
		t.Fatal("DocReader parser is unavailable")
	}
	ctx := context.Background()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		kbID := fmt.Sprintf("invkb%d", time.Now().UnixNano()%1000000000)
		docID := fmt.Sprintf("invdoc%d", time.Now().UnixNano()%1000000000)
		if err := tx.Exec(`INSERT INTO t_knowledge_base
			(id, name, embedding_model, collection_name, created_by)
			VALUES (?, 'Inventory retry live', 'test', ?, 'test')`, kbID, kbID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO t_knowledge_document
			(id, kb_id, doc_name, file_url, file_type, created_by, status)
			VALUES (?, ?, 'text.png', 'source.png', 'png', 'test', 'partial')`, docID, kbID).Error; err != nil {
			return err
		}
		storage := &liveStorage{files: map[string][]byte{"source.png": content}}
		svc := &Service{DB: tx, Storage: storage, InventoryParser: selected.(*parser.DocReaderDocumentParser)}
		old, err := svc.Register(ctx, RegisterInput{DocumentID: docID, KnowledgeBaseID: kbID,
			Source: content, Parsed: parser.ParseResult{Text: "fallback text", Metadata: map[string]any{"parser_type": "tika"}}})
		if err != nil {
			return err
		}
		claimed, err := svc.claim(ctx, "inventory-test")
		if err != nil || claimed.Operation != "inventory" {
			return fmt.Errorf("expected inventory task, got %+v: %w", claimed, err)
		}
		if err := svc.process(ctx, "inventory-test", claimed); err != nil {
			return err
		}
		var newRevision string
		if err := tx.Raw(`SELECT active_revision_id FROM t_knowledge_document WHERE id = ?`, docID).Scan(&newRevision).Error; err != nil {
			return err
		}
		if newRevision == old.RevisionID || newRevision == "" {
			return fmt.Errorf("inventory did not publish a new revision")
		}
		var imageCount, pendingTasks int
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_image_occurrence WHERE revision_id = ?`, newRevision).Scan(&imageCount).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT count(*) FROM t_knowledge_image_task
			WHERE revision_id = ? AND operation IN ('ocr','caption') AND status = 'pending'`, newRevision).Scan(&pendingTasks).Error; err != nil {
			return err
		}
		if imageCount != 1 || pendingTasks != 2 {
			return fmt.Errorf("expected one image and two processing tasks, got %d/%d", imageCount, pendingTasks)
		}
		return fmt.Errorf("ROLLBACK_TEST_FIXTURE")
	})
	if err != nil && err.Error() != "ROLLBACK_TEST_FIXTURE" {
		t.Fatal(err)
	}
}
