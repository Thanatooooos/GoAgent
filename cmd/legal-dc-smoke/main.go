package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/distributedid"
)

const operatorID = "legal-dc-smoke"

type chunkRow struct {
	ID         string `json:"id"`
	ChunkIndex int    `json:"chunkIndex"`
	CharCount  int    `json:"charCount"`
	Content    string `json:"content"`
}

type report struct {
	SourceFile      string     `json:"sourceFile"`
	KBID            string     `json:"knowledgeBaseId"`
	KBName          string     `json:"knowledgeBaseName"`
	DocumentID      string     `json:"documentId"`
	DocumentName    string     `json:"documentName"`
	DocumentStatus  string     `json:"documentStatus"`
	FileType        string     `json:"fileType"`
	ChunkConfig     string     `json:"chunkConfig"`
	ChunkRows       int        `json:"persistedChunkRows"`
	ParentRows      int        `json:"parentChunkRows"`
	ChildRows       int        `json:"childChunkRows"`
	VectorRows      int        `json:"vectorRows"`
	ChildVectorRows int        `json:"childVectorRows"`
	Samples         []chunkRow `json:"chunkSamples"`
	GeneratedAt     time.Time  `json:"generatedAt"`
}

func main() {
	source := flag.String("source", "tmp/legal-dc/Legal-DC/data/documents_539/documents_539/《对外劳务合作经营资格管理办法》补充规定.docx", "DOCX source to ingest")
	kbName := flag.String("kb", "legal-dc-smoke", "isolated smoke-test knowledge base name")
	reuseKB := flag.Bool("reuse-kb", false, "allow adding this document to an existing knowledge base")
	output := flag.String("output", "tmp/legal-dc/smoke-ingestion-report.json", "report output path")
	flag.Parse()

	info, err := os.Stat(*source)
	if err != nil {
		fatalf("stat source: %v", err)
	}
	file, err := os.Open(*source)
	if err != nil {
		fatalf("open source: %v", err)
	}
	defer file.Close()

	if err := config.LoadConfig("configs"); err != nil {
		fatalf("load config: %v", err)
	}
	cfg := config.Get()
	if cfg == nil {
		fatalf("knowledge config is unavailable")
	}
	// This smoke test validates parsing/chunking/embedding only; never invoke chat enrichment.
	cfg.Rag.Knowledge.Enrichment.Enabled = false

	ctx := context.Background()
	runtime, err := knowledgebootstrap.NewRuntime(ctx, knowledgebootstrap.RuntimeOptions{Config: cfg})
	if err != nil {
		fatalf("create knowledge runtime: %v", err)
	}
	defer runtime.Close()

	kbID, err := ensureKnowledgeBase(ctx, runtime.DB, *kbName, *reuseKB)
	if err != nil {
		fatalf("create smoke knowledge base: %v", err)
	}
	chunkConfig := `{"enableParentChild":true,"parentChunkSize":1200,"parentOverlapSize":0,"childChunkSize":300,"childOverlapSize":60}`
	docRepo := postgresknowledge.NewKnowledgeDocumentRepository(runtime.DB, nil)
	documentID, err := existingDocumentID(ctx, runtime.DB, kbID, filepath.Base(*source))
	if err != nil {
		fatalf("find existing document: %v", err)
	}
	var document knowledgedomain.KnowledgeDocument
	if documentID != "" && *reuseKB {
		document, err = docRepo.GetByID(ctx, documentID)
		if err != nil {
			fatalf("reload existing document: %v", err)
		}
	} else {
		document, err = runtime.DocumentService.Upload(ctx, knowledgeservice.UploadKnowledgeDocumentInput{
			KnowledgeBaseID: kbID,
			SourceType:      knowledgedomain.KnowledgeDocumentSourceFile,
			FileName:        filepath.Base(*source),
			ContentType:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			Size:            info.Size(),
			Body:            file,
			ProcessMode:     knowledgedomain.KnowledgeDocumentProcessModeChunk,
			ChunkStrategy:   "fixed_size",
			ChunkConfig:     chunkConfig,
			OperatorID:      operatorID,
		})
		if err != nil {
			fatalf("upload docx: %v", err)
		}
	}
	if err := runtime.DocumentProcessService.ExecuteChunk(ctx, knowledgeservice.ExecuteChunkInput{
		DocumentID:  document.ID,
		TriggeredBy: operatorID,
	}); err != nil {
		fatalf("parse, chunk and embed docx: %v", err)
	}

	persisted, err := docRepo.GetByID(ctx, document.ID)
	if err != nil {
		fatalf("reload document: %v", err)
	}
	var chunks []chunkRow
	if err := runtime.DB.Raw(`SELECT id, chunk_index, char_count, content FROM t_knowledge_chunk WHERE doc_id = ? ORDER BY chunk_index, id`, document.ID).Scan(&chunks).Error; err != nil {
		fatalf("read chunks: %v", err)
	}
	var vectorRows, childVectorRows int
	if err := runtime.DB.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ?`, document.ID).Scan(&vectorRows).Error; err != nil {
		fatalf("count vectors: %v", err)
	}
	if err := runtime.DB.Raw(`SELECT count(*) FROM t_knowledge_chunk_vector WHERE doc_id = ? AND metadata->>'record_type' = 'child'`, document.ID).Scan(&childVectorRows).Error; err != nil {
		fatalf("count child vectors: %v", err)
	}

	parents, children := 0, 0
	for _, chunk := range chunks {
		if strings.Contains(chunk.ID, "-p-") {
			parents++
		} else {
			children++
		}
		if len(chunk.Content) > 360 {
			chunk.Content = chunk.Content[:360] + "…"
		}
	}
	if persisted.Status != knowledgedomain.KnowledgeDocumentStatusSuccess || parents == 0 || children == 0 || vectorRows != childVectorRows || childVectorRows != children {
		fatalf("smoke verification failed: status=%s parents=%d children=%d vectors=%d childVectors=%d", persisted.Status, parents, children, vectorRows, childVectorRows)
	}

	reportData := report{
		SourceFile: *source, KBID: kbID, KBName: *kbName, DocumentID: document.ID,
		DocumentName: persisted.Name, DocumentStatus: persisted.Status, FileType: persisted.FileType,
		ChunkConfig: chunkConfig, ChunkRows: len(chunks), ParentRows: parents, ChildRows: children,
		VectorRows: vectorRows, ChildVectorRows: childVectorRows, Samples: chunks, GeneratedAt: time.Now(),
	}
	encoded, err := json.MarshalIndent(reportData, "", "  ")
	if err != nil {
		fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("PASS doc=%s status=%s parent_chunks=%d child_chunks=%d vectors=%d report=%s\n", document.ID, persisted.Status, parents, children, vectorRows, *output)
}

func existingDocumentID(ctx context.Context, db *gorm.DB, knowledgeBaseID, documentName string) (string, error) {
	var row struct{ ID string }
	err := db.Raw(`SELECT id FROM t_knowledge_document WHERE kb_id = ? AND doc_name = ? AND deleted = 0 ORDER BY create_time DESC LIMIT 1`, knowledgeBaseID, documentName).Scan(&row).Error
	return row.ID, err
}

func ensureKnowledgeBase(ctx context.Context, db *gorm.DB, name string, reuse bool) (string, error) {
	repo := postgresknowledge.NewKnowledgeBaseRepository(db)
	existing, err := repo.List(ctx, knowledgeport.KnowledgeBaseListFilter{Query: name, ListOptions: knowledgeport.ListOptions{Limit: 1}})
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		if reuse {
			return existing[0].ID, nil
		}
		return "", fmt.Errorf("knowledge base %q already exists; refuse to reuse a smoke-test base", name)
	}
	id, err := distributedid.NextID()
	if err != nil {
		return "", err
	}
	kb := knowledgedomain.NewKnowledgeBase(fmt.Sprintf("%d", id), name, "", name, operatorID)
	if _, err := repo.Create(ctx, kb); err != nil {
		return "", err
	}
	return kb.ID, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
