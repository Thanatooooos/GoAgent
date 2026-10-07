package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/framework/config"
)

func TestDocumentProcessServiceLiveDocReader(t *testing.T) {
	if os.Getenv("DOCREADER_LIVE") != "1" {
		t.Skip("set DOCREADER_LIVE=1 to run against a live DocReader")
	}
	fixtureDir := os.Getenv("DOCREADER_FIXTURE_DIR")
	if fixtureDir == "" {
		fixtureDir = filepath.Join("..", "..", "..", "core", "parser", "test", "fixtures", "docreader")
	}
	if err := config.LoadConfig(filepath.Join("..", "..", "..", "..", "..", "configs")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file, fileType, want string
		wantFailure          bool
	}{
		{"report.pdf", "pdf", "Quarterly Report", false},
		{"scanned.pdf", "pdf", "SCANNED INVOICE 2026", false},
		{"text.png", "png", "IMAGE RECEIPT 42", false},
		{"blank.png", "png", "", true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(fixtureDir, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			document := processDocument()
			document.Name = tc.file
			document.FileType = tc.fileType
			document.FileURL = tc.file
			documentRepo := &processDocumentRepositoryStub{document: document}
			chunkRepo := &processChunkRepositoryStub{}
			vectorStore := &processVectorStoreStub{}
			chunkLogRepo := &processChunkLogRepositoryStub{}
			enrichmentEnabled := false
			svc := NewDocumentProcessService(DocumentProcessServiceOptions{
				BaseRepo:          processBaseRepositoryStub{base: domain.KnowledgeBase{ID: "kb-1", EmbeddingModel: "embed-model"}},
				DocumentRepo:      documentRepo,
				ChunkRepo:         chunkRepo,
				ChunkLogRepo:      chunkLogRepo,
				Storage:           processStorageStub{body: string(content)},
				VectorStore:       vectorStore,
				Embedding:         processEmbeddingStub{},
				EnrichmentEnabled: &enrichmentEnabled,
			})
			err = svc.ExecuteChunk(context.Background(), ExecuteChunkInput{DocumentID: document.ID, TriggeredBy: "smoke"})
			if tc.wantFailure {
				if err == nil || len(chunkLogRepo.updated) == 0 || !strings.Contains(chunkLogRepo.updated[len(chunkLogRepo.updated)-1].ErrorMessage, "no searchable text") {
					t.Fatalf("expected no searchable text failure, got %v", err)
				}
				if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusFailed) || len(vectorStore.upserted) != 0 {
					t.Fatal("blank image must fail without persisting vectors")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !documentStatusUpdated(documentRepo.updateFields, domain.KnowledgeDocumentStatusSuccess) || len(chunkRepo.created) == 0 || len(vectorStore.upserted) == 0 {
				t.Fatalf("incomplete pipeline: chunks=%d vectors=%d", len(chunkRepo.created), len(vectorStore.upserted))
			}
			found := false
			for _, vector := range vectorStore.upserted {
				if strings.Contains(vector.Text, tc.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("parsed text missing from vectors: %#v", vectorStore.upserted)
			}
		})
	}
}
