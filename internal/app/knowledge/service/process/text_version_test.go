package process

import (
	"testing"

	corechunk "local/rag-project/internal/app/core/chunk"
	"local/rag-project/internal/app/knowledge/domain"
)

func TestTextChunkIDsFollowPublicationGeneration(t *testing.T) {
	document := domain.KnowledgeDocument{ID: "doc-a", KnowledgeBaseID: "kb-a", Name: "source.pdf"}
	chunks := []corechunk.Chunk{{Index: 0, Text: "first paragraph"}}
	firstGeneration := "11111111-1111-4111-8111-111111111111"
	secondGeneration := "22222222-2222-4222-8222-222222222222"
	first := buildKnowledgeChunks(document, chunks, "tester", firstGeneration)
	firstVectors := buildChunkVectors(document, chunks, firstGeneration)
	second := buildKnowledgeChunks(document, chunks, "tester", secondGeneration)
	if len(first) != 1 || len(firstVectors) != 1 || len(second) != 1 {
		t.Fatal("unexpected chunk counts")
	}
	if first[0].ID != firstVectors[0].ChunkID || first[0].ID == second[0].ID || len(first[0].ID) > 64 {
		t.Fatalf("text publication identities are inconsistent: %q %q %q",
			first[0].ID, firstVectors[0].ChunkID, second[0].ID)
	}
}
