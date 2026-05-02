package s3_test

import (
	"testing"

	storage "local/rag-project/internal/adapter/storage/s3"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/config"
)

func TestFileStorageImplementsPort(t *testing.T) {
	t.Parallel()

	var _ port.FileStorage = storage.NewFileStorageWithClient(nil, "knowledge")
}

func TestNewFileStorageRequiresURL(t *testing.T) {
	t.Parallel()

	_, err := storage.NewFileStorage(config.RustFSConfig{})
	if err == nil {
		t.Fatal("NewFileStorage should require rustfs url")
	}
}
