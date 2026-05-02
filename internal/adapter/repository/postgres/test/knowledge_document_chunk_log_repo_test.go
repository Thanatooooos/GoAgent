package postgres_test

import (
	"testing"
	"time"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/port"
)

func TestKnowledgeDocumentChunkLogRepositoryImplementsPort(t *testing.T) {
	t.Parallel()

	var _ port.KnowledgeDocumentChunkLogRepository = postgresknowledge.NewKnowledgeDocumentChunkLogRepository(nil)
}

func TestKnowledgeDocumentChunkLogModelTableName(t *testing.T) {
	t.Parallel()

	if got := (models.KnowledgeDocumentChunkLogModel{}).TableName(); got != "t_knowledge_document_chunk_log" {
		t.Fatalf("unexpected table name: %q", got)
	}
}

func TestKnowledgeDocumentChunkLogModelCarriesTimingFields(t *testing.T) {
	t.Parallel()

	start := time.Now()
	end := start.Add(time.Second)
	model := models.KnowledgeDocumentChunkLogModel{
		ID:              "log-1",
		DocumentID:      "doc-1",
		Status:          "success",
		ExtractDuration: 1,
		ChunkDuration:   2,
		EmbedDuration:   3,
		PersistDuration: 4,
		TotalDuration:   10,
		ChunkCount:      5,
		StartTime:       &start,
		EndTime:         &end,
	}

	if model.DocumentID != "doc-1" || model.TotalDuration != 10 || model.ChunkCount != 5 {
		t.Fatal("chunk log model did not preserve timing fields")
	}
}
