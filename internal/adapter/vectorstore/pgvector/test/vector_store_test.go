package pgvector_test

import (
	"context"
	"testing"

	vectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/app/knowledge/port"
)

func TestVectorStoreImplementsPort(t *testing.T) {
	t.Parallel()

	var _ port.VectorStore = vectorstore.NewVectorStore(nil)
}

func TestVectorStoreUpsertEmptyChunksSkipsNilDB(t *testing.T) {
	t.Parallel()

	store := vectorstore.NewVectorStore(nil)
	if err := store.UpsertDocumentChunks(context.Background(), nil); err != nil {
		t.Fatalf("UpsertDocumentChunks(empty) error = %v", err)
	}
}

func TestVectorStoreDeleteChunksEmptySkipsNilDB(t *testing.T) {
	t.Parallel()

	store := vectorstore.NewVectorStore(nil)
	if err := store.DeleteChunks(context.Background(), nil); err != nil {
		t.Fatalf("DeleteChunks(empty) error = %v", err)
	}
}
