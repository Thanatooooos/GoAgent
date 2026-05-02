package service_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	storage "local/rag-project/internal/adapter/storage/s3"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/config"
)

func TestKnowledgeDocumentUploadIntegration(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_UPLOAD") != "1" {
		t.Skip("set RAG_INTEGRATION_UPLOAD=1 to run upload integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := postgresrepo.NewGormDB(config.DataSourceConfig{
		Url:      getenvDefault("POSTGRES_URL", "jdbc:postgresql://postgres:5432/ragent"),
		Username: getenvDefault("POSTGRES_USER", "postgres"),
		Password: getenvDefault("POSTGRES_PASSWORD", "postgres"),
	})
	if err != nil {
		t.Fatalf("new postgres db: %v", err)
	}

	fileStorage, err := storage.NewFileStorage(config.RustFSConfig{
		Url:             getenvDefault("RUSTFS_URL", "http://127.0.0.1:9000"),
		AccessKeyId:     getenvDefault("RUSTFS_ACCESS_KEY_ID", "rustfsadmin"),
		SecretAccessKey: getenvDefault("RUSTFS_SECRET_ACCESS_KEY", "rustfsadmin"),
		Bucket:          getenvDefault("RUSTFS_BUCKET", "knowledge"),
	})
	if err != nil {
		t.Fatalf("new file storage: %v", err)
	}

	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	documentRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	documentService := service.NewKnowledgeDocumentService(baseRepo, documentRepo, nil, nil, nil, fileStorage, nil, nil, nil)

	suffix := time.Now().UnixNano()
	base := domain.NewKnowledgeBase(
		fmt.Sprintf("%d", suffix%1000000000000000000),
		fmt.Sprintf("upload-it-%d", suffix),
		"integration-embedding",
		fmt.Sprintf("upload_it_%d", suffix%1000000),
		"integration",
	)
	createdBase, err := baseRepo.Create(ctx, base)
	if err != nil {
		t.Fatalf("create knowledge base: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = db.WithContext(cleanupCtx).Exec("delete from t_knowledge_base where id = ?", createdBase.ID).Error
	})

	content := []byte("document upload integration content")
	createdDocument, err := documentService.Upload(ctx, service.UploadKnowledgeDocumentInput{
		KnowledgeBaseID: createdBase.ID,
		FileName:        "../upload-integration.txt",
		ContentType:     "text/plain; charset=utf-8",
		Size:            int64(len(content)),
		Body:            bytes.NewReader(content),
		OperatorID:      "integration",
	})
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = fileStorage.Delete(cleanupCtx, createdDocument.FileURL)
		_ = db.WithContext(cleanupCtx).Exec("delete from t_knowledge_document where id = ?", createdDocument.ID).Error
	})

	if createdDocument.ID == "" {
		t.Fatal("created document id is empty")
	}
	if createdDocument.KnowledgeBaseID != createdBase.ID {
		t.Fatalf("document kb id mismatch: got %q want %q", createdDocument.KnowledgeBaseID, createdBase.ID)
	}
	if createdDocument.Name != "upload-integration.txt" {
		t.Fatalf("document name should be sanitized, got %q", createdDocument.Name)
	}
	if createdDocument.FileURL == "" {
		t.Fatal("document file url is empty")
	}

	persisted, err := documentRepo.GetByID(ctx, createdDocument.ID)
	if err != nil {
		t.Fatalf("get uploaded document: %v", err)
	}
	if persisted.ID != createdDocument.ID {
		t.Fatalf("persisted document id mismatch: got %q want %q", persisted.ID, createdDocument.ID)
	}

	reader, err := fileStorage.Open(ctx, createdDocument.FileURL)
	if err != nil {
		t.Fatalf("open uploaded object: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read uploaded object: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("uploaded content mismatch: got %q want %q", string(got), string(content))
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
