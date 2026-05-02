package s3_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	storage "local/rag-project/internal/adapter/storage/s3"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/config"
)

func TestFileStorageUploadOpenDeleteIntegration(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_RUSTFS") != "1" {
		t.Skip("set RAG_INTEGRATION_RUSTFS=1 to run RustFS integration test")
	}

	endpoint := getenvDefault("RUSTFS_URL", "http://localhost:9000")
	bucket := getenvDefault("RUSTFS_BUCKET", "knowledge")
	accessKey := getenvDefault("RUSTFS_ACCESS_KEY_ID", "rustfsadmin")
	secretKey := getenvDefault("RUSTFS_SECRET_ACCESS_KEY", "rustfsadmin")

	fileStorage, err := storage.NewFileStorage(config.RustFSConfig{
		Url:             endpoint,
		AccessKeyId:     accessKey,
		SecretAccessKey: secretKey,
		Bucket:          bucket,
	})
	if err != nil {
		t.Fatalf("new file storage: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	content := []byte("rustfs integration upload from FileStorage")
	key := fmt.Sprintf("integration/file-storage-%d.txt", time.Now().UnixNano())

	stored, err := fileStorage.Upload(ctx, port.FileUpload{
		Key:         key,
		FileName:    "file-storage.txt",
		ContentType: "text/plain; charset=utf-8",
		Size:        int64(len(content)),
		Body:        bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}
	t.Cleanup(func() {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer deleteCancel()
		_ = fileStorage.Delete(deleteCtx, key)
	})

	if stored.Key != key {
		t.Fatalf("stored key mismatch: got %q want %q", stored.Key, key)
	}
	if stored.Size != int64(len(content)) {
		t.Fatalf("stored size mismatch: got %d want %d", stored.Size, len(content))
	}

	reader, err := fileStorage.Open(ctx, key)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q want %q", string(got), string(content))
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
