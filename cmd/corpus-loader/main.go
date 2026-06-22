package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	corechunk "local/rag-project/internal/app/core/chunk"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/distributedid"
	infraai "local/rag-project/internal/infra-ai"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

const loaderOperatorID = "corpus-loader"

type passagesFile struct {
	Description string            `json:"description"`
	Passages    map[string]string `json:"passages"`
}

type mappingEntry struct {
	PassageID  string `json:"passageId"`
	DocumentID string `json:"documentId"`
	ChunkID    string `json:"chunkId"`
}

type markdownCorpusFile struct {
	AbsolutePath string
	RelativePath string
	DocumentName string
	Content      []byte
}

type markdownManifest struct {
	GeneratedAt   time.Time                `json:"generatedAt"`
	KnowledgeBase markdownKnowledgeBaseRef `json:"knowledgeBase"`
	SourceDir     string                   `json:"sourceDir"`
	ChunkStrategy string                   `json:"chunkStrategy"`
	ChunkConfig   json.RawMessage          `json:"chunkConfig,omitempty"`
	DocumentCount int                      `json:"documentCount"`
	ChunkCount    int                      `json:"chunkCount"`
	Documents     []markdownManifestDoc    `json:"documents"`
}

type markdownKnowledgeBaseRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type markdownManifestDoc struct {
	DocumentID   string                  `json:"documentId"`
	DocumentName string                  `json:"documentName"`
	RelativePath string                  `json:"relativePath"`
	AbsolutePath string                  `json:"absolutePath"`
	ChunkCount   int                     `json:"chunkCount"`
	Chunks       []markdownManifestChunk `json:"chunks"`
}

type markdownManifestChunk struct {
	ChunkID   string         `json:"chunkId"`
	Index     int            `json:"index"`
	Content   string         `json:"content"`
	CharCount int            `json:"charCount"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type vectorChunkRow struct {
	ChunkID  string
	Index    int
	Content  string
	Metadata map[string]any
}

func main() {
	inputPath := flag.String("input", "", "path to passages JSON file")
	dirPath := flag.String("dir", "", "path to a markdown corpus directory")
	kbName := flag.String("kb", "corpus-bench", "knowledge base name")
	batchSize := flag.Int("batch", 48, "embedding batch size")
	chunkStrategy := flag.String("chunk-strategy", "markdown", "chunk strategy for -dir imports: markdown or fixed_size")
	chunkConfig := flag.String("chunk-config", "", "optional chunk config JSON for -dir imports")
	manifestPath := flag.String("manifest", "", "optional markdown manifest output path for -dir imports")
	dryRun := flag.Bool("dry-run", false, "only print what would be done")
	ensureIndexes := flag.Bool("ensure-indexes", false, "create GIN trigram indexes on chunk vector content and metadata columns")
	cleanKB := flag.Bool("clean-kb", false, "delete all documents, chunks and vectors for the specified knowledge base before import")
	flag.Parse()

	mode, err := detectLoaderMode(strings.TrimSpace(*inputPath), strings.TrimSpace(*dirPath), *ensureIndexes, *cleanKB)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	var passages map[string]string
	var corpusFiles []markdownCorpusFile

	switch mode {
	case "passages":
		passages, err = loadPassages(*inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load passages: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[loader] loaded %d passages\n", len(passages))
	case "markdown":
		strategy, err := normalizeLoaderChunkStrategy(*chunkStrategy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "normalize chunk strategy: %v\n", err)
			os.Exit(1)
		}
		*chunkStrategy = strategy
		corpusFiles, err = loadMarkdownCorpus(*dirPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load markdown corpus: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[loader] loaded %d markdown files from %s\n", len(corpusFiles), *dirPath)
	}

	if *dryRun {
		switch mode {
		case "passages":
			for pid, text := range passages {
				preview := previewText(text, 80)
				fmt.Printf("  %s: %s\n", pid, preview)
			}
		case "markdown":
			for _, file := range corpusFiles {
				fmt.Printf("  %s -> %s (%d bytes)\n", file.RelativePath, file.DocumentName, len(file.Content))
			}
		}
		return
	}

	if err := config.LoadConfig("configs"); err != nil {
		fmt.Fprintf(os.Stderr, "[loader] warning: load config: %v (continuing with defaults)\n", err)
	}

	ctx := context.Background()

	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create db: %v\n", err)
		os.Exit(1)
	}
	defer closeDB(db)

	if err := ensureKnowledgeTables(db); err != nil {
		fmt.Fprintf(os.Stderr, "ensure tables: %v\n", err)
		os.Exit(1)
	}

	if *cleanKB {
		if err := cleanKnowledgeBase(ctx, db, *kbName); err != nil {
			fmt.Fprintf(os.Stderr, "clean kb: %v\n", err)
			os.Exit(1)
		}
		if mode == "none" {
			return
		}
	}

	if *ensureIndexes {
		if err := createTrgmIndexes(db); err != nil {
			fmt.Fprintf(os.Stderr, "create indexes: %v\n", err)
			os.Exit(1)
		}
		if mode == "none" {
			return
		}
	}

	switch mode {
	case "passages":
		runPassageImport(ctx, db, *inputPath, *kbName, *batchSize, passages)
	case "markdown":
		runMarkdownImport(ctx, *dirPath, *kbName, *chunkStrategy, strings.TrimSpace(*chunkConfig), *manifestPath, corpusFiles)
	default:
		fmt.Fprintln(os.Stderr, "nothing to do")
	}
}

func runPassageImport(ctx context.Context, db *gorm.DB, inputPath string, kbName string, batchSize int, passages map[string]string) {
	aiRuntime := infraai.NewRuntime()
	embedding := aiRuntime.Embedding
	vectorStore := pgvectorstore.NewVectorStore(db)

	kbID, err := ensureKnowledgeBase(ctx, db, kbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ensure kb: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[loader] knowledge base: id=%s name=%q\n", kbID, kbName)

	mappingPath := strings.TrimSuffix(inputPath, ".json") + "_mapping.json"
	mappings, err := ingestPassages(ctx, db, vectorStore, embedding, kbID, passages, batchSize, mappingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest passages: %v\n", err)
		os.Exit(1)
	}

	outputPath := strings.TrimSuffix(inputPath, ".json") + "_mapping.json"
	if err := writeJSONFile(outputPath, mappings); err != nil {
		fmt.Fprintf(os.Stderr, "write mappings: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[loader] done: %d documents ingested, mapping written to %s\n", len(mappings), outputPath)
}

func runMarkdownImport(
	ctx context.Context,
	dirPath string,
	kbName string,
	chunkStrategy string,
	chunkConfig string,
	manifestPath string,
	corpusFiles []markdownCorpusFile,
) {
	runtime, err := knowledgebootstrap.NewRuntime(ctx, knowledgebootstrap.RuntimeOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create knowledge runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "[loader] warning: close runtime: %v\n", closeErr)
		}
	}()

	if runtime.DocumentService == nil || runtime.DocumentProcessService == nil || runtime.DB == nil {
		fmt.Fprintln(os.Stderr, "knowledge runtime is missing document processing dependencies")
		os.Exit(1)
	}

	kbID, err := ensureKnowledgeBase(ctx, runtime.DB, kbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ensure kb: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[loader] knowledge base: id=%s name=%q\n", kbID, kbName)

	if strings.TrimSpace(manifestPath) == "" {
		manifestPath = defaultManifestPath(dirPath, chunkStrategy)
	}

	chunkRepo := postgresknowledge.NewKnowledgeChunkRepository(runtime.DB)
	docRepo := postgresknowledge.NewKnowledgeDocumentRepository(runtime.DB, nil)

	manifest := markdownManifest{
		GeneratedAt: time.Now(),
		KnowledgeBase: markdownKnowledgeBaseRef{
			ID:   kbID,
			Name: kbName,
		},
		SourceDir:     absPathOrOriginal(dirPath),
		ChunkStrategy: chunkStrategy,
		DocumentCount: len(corpusFiles),
	}
	if strings.TrimSpace(chunkConfig) != "" {
		manifest.ChunkConfig = json.RawMessage(strings.TrimSpace(chunkConfig))
	}

	totalChunks := 0
	for idx, file := range corpusFiles {
		doc, err := runtime.DocumentService.Upload(ctx, knowledgeservice.UploadKnowledgeDocumentInput{
			KnowledgeBaseID: kbID,
			SourceType:      knowledgedomain.KnowledgeDocumentSourceFile,
			FileName:        file.DocumentName,
			ContentType:     "text/markdown",
			Size:            int64(len(file.Content)),
			Body:            bytes.NewReader(file.Content),
			ProcessMode:     knowledgedomain.KnowledgeDocumentProcessModeChunk,
			ChunkStrategy:   chunkStrategy,
			ChunkConfig:     strings.TrimSpace(chunkConfig),
			OperatorID:      loaderOperatorID,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "upload markdown document %s: %v\n", file.RelativePath, err)
			os.Exit(1)
		}

		if err := runtime.DocumentProcessService.ExecuteChunk(ctx, knowledgeservice.ExecuteChunkInput{
			DocumentID:  doc.ID,
			TriggeredBy: loaderOperatorID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "process markdown document %s: %v\n", file.RelativePath, err)
			os.Exit(1)
		}

		persistedDoc, err := docRepo.GetByID(ctx, doc.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reload document %s: %v\n", doc.ID, err)
			os.Exit(1)
		}
		chunks, err := chunkRepo.List(ctx, knowledgeport.KnowledgeChunkListFilter{DocumentID: doc.ID})
		if err != nil {
			fmt.Fprintf(os.Stderr, "list chunks for %s: %v\n", doc.ID, err)
			os.Exit(1)
		}
		vectorRows, err := listDocumentVectors(ctx, runtime.DB, doc.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "list vector metadata for %s: %v\n", doc.ID, err)
			os.Exit(1)
		}

		docManifest := buildMarkdownManifestDoc(file, persistedDoc, chunks, vectorRows)
		manifest.Documents = append(manifest.Documents, docManifest)
		totalChunks += len(docManifest.Chunks)

		fmt.Fprintf(os.Stderr, "[loader] document %d/%d: %s -> %d chunks\n", idx+1, len(corpusFiles), file.RelativePath, len(docManifest.Chunks))
	}

	manifest.ChunkCount = totalChunks
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[loader] done: %d markdown documents processed, manifest written to %s\n", len(manifest.Documents), manifestPath)
}

func detectLoaderMode(inputPath string, dirPath string, ensureIndexes bool, cleanKB bool) (string, error) {
	hasInput := inputPath != ""
	hasDir := dirPath != ""

	if hasInput && hasDir {
		return "", errors.New("corpus-loader accepts either -input or -dir, not both")
	}
	if hasInput {
		return "passages", nil
	}
	if hasDir {
		return "markdown", nil
	}
	if ensureIndexes || cleanKB {
		return "none", nil
	}

	return "", errors.New("usage: corpus-loader (-input <passages.json> | -dir <markdown-dir>) [-kb <name>] [-batch <n>] [-chunk-strategy markdown|fixed_size] [-chunk-config <json>] [-manifest <path>] [-ensure-indexes] [-clean-kb] [-dry-run]")
}

func cleanKnowledgeBase(ctx context.Context, db *gorm.DB, kbName string) error {
	repo := postgresknowledge.NewKnowledgeBaseRepository(db)
	kbs, err := repo.List(ctx, knowledgeport.KnowledgeBaseListFilter{
		Query:       kbName,
		ListOptions: knowledgeport.ListOptions{Limit: 1},
	})
	if err != nil {
		return fmt.Errorf("find kb: %w", err)
	}
	if len(kbs) == 0 || kbs[0].ID == "" {
		fmt.Fprintf(os.Stderr, "[loader] kb %q not found, nothing to clean\n", kbName)
		return nil
	}
	kbID := kbs[0].ID

	tables := []struct {
		name  string
		query string
	}{
		{"vectors", "DELETE FROM t_knowledge_chunk_vector WHERE kb_id = ?"},
		{"chunks", "DELETE FROM t_knowledge_chunk WHERE kb_id = ?"},
		{"chunk_logs", "DELETE FROM t_knowledge_document_chunk_log WHERE doc_id IN (SELECT id FROM t_knowledge_document WHERE kb_id = ?)"},
		{"documents", "DELETE FROM t_knowledge_document WHERE kb_id = ?"},
		{"base", "DELETE FROM t_knowledge_base WHERE id = ?"},
	}
	for _, t := range tables {
		result := db.WithContext(ctx).Exec(t.query, kbID)
		if result.Error != nil {
			return fmt.Errorf("clean %s: %w", t.name, result.Error)
		}
		fmt.Fprintf(os.Stderr, "[loader] cleaned %s: %d rows\n", t.name, result.RowsAffected)
	}
	return nil
}

func createTrgmIndexes(db *gorm.DB) error {
	indexes := []struct {
		name string
		sql  string
	}{
		{"content keyword", `CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_trgm ON t_knowledge_chunk_vector USING GIN (content gin_trgm_ops)`},
		{"metadata document_name", `CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_docname_trgm ON t_knowledge_chunk_vector USING GIN ((metadata->>'document_name') gin_trgm_ops)`},
		{"metadata source_file_name", `CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_filename_trgm ON t_knowledge_chunk_vector USING GIN ((metadata->>'source_file_name') gin_trgm_ops)`},
		{"metadata section", `CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_section_trgm ON t_knowledge_chunk_vector USING GIN ((metadata->>'section') gin_trgm_ops)`},
	}
	for _, idx := range indexes {
		if err := db.Exec(idx.sql).Error; err != nil {
			return fmt.Errorf("create index %s: %w", idx.name, err)
		}
		fmt.Fprintf(os.Stderr, "[loader] index ok: %s\n", idx.name)
	}
	return nil
}

func loadPassages(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pf passagesFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse passages file: %w", err)
	}
	if len(pf.Passages) == 0 {
		return nil, fmt.Errorf("no passages found")
	}
	return pf.Passages, nil
}

func loadMarkdownCorpus(dir string) ([]markdownCorpusFile, error) {
	absRoot, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return nil, fmt.Errorf("resolve corpus dir: %w", err)
	}

	files := make([]markdownCorpusFile, 0)
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read markdown file %s: %w", path, err)
		}
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)
		files = append(files, markdownCorpusFile{
			AbsolutePath: path,
			RelativePath: relPath,
			DocumentName: markdownDocumentName(relPath),
			Content:      content,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no markdown files found under %s", absRoot)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, nil
}

func markdownDocumentName(relPath string) string {
	name := strings.TrimSpace(relPath)
	name = strings.ReplaceAll(name, "/", "__")
	name = strings.ReplaceAll(name, "\\", "__")
	return name
}

func normalizeLoaderChunkStrategy(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "markdown", "structure_aware":
		return string(corechunk.StrategyMarkdown), nil
	case "fixed_size":
		return string(corechunk.StrategyFixedSize), nil
	default:
		return "", fmt.Errorf("chunk strategy must be markdown or fixed_size")
	}
}

func ensureKnowledgeTables(db *gorm.DB) error {
	return postgresrepo.EnsureTablesExist(db, []string{
		"t_knowledge_base",
		"t_knowledge_document",
		"t_knowledge_chunk",
		"t_knowledge_chunk_vector",
		"t_knowledge_document_chunk_log",
	})
}

func ensureKnowledgeBase(ctx context.Context, db *gorm.DB, name string) (string, error) {
	repo := postgresknowledge.NewKnowledgeBaseRepository(db)
	existing, err := repo.List(ctx, knowledgeport.KnowledgeBaseListFilter{
		Query:       name,
		ListOptions: knowledgeport.ListOptions{Limit: 1},
	})
	if err != nil {
		return "", fmt.Errorf("list kb: %w", err)
	}
	if len(existing) > 0 && existing[0].ID != "" {
		return existing[0].ID, nil
	}

	id, err := distributedid.NextID()
	if err != nil {
		return "", fmt.Errorf("generate kb id: %w", err)
	}
	kb := knowledgedomain.NewKnowledgeBase(
		fmt.Sprintf("%d", id),
		name,
		"",
		name,
		loaderOperatorID,
	)
	if _, err := repo.Create(ctx, kb); err != nil {
		return "", fmt.Errorf("create kb: %w", err)
	}
	return kb.ID, nil
}

func ingestPassages(
	ctx context.Context,
	db *gorm.DB,
	vectorStore *pgvectorstore.VectorStore,
	embedding aiembedding.EmbeddingService,
	kbID string,
	passages map[string]string,
	batchSize int,
	mappingPath string,
) ([]mappingEntry, error) {
	docRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	chunkRepo := postgresknowledge.NewKnowledgeChunkRepository(db)

	pids := make([]string, 0, len(passages))
	for pid := range passages {
		pids = append(pids, pid)
	}
	sort.Strings(pids)

	mappings := make([]mappingEntry, 0, len(pids))
	startedAt := time.Now()

	for i := 0; i < len(pids); i += batchSize {
		end := i + batchSize
		if end > len(pids) {
			end = len(pids)
		}
		batch := pids[i:end]

		batchMappings := make([]mappingEntry, len(batch))
		texts := make([]string, len(batch))
		for j, pid := range batch {
			text := strings.TrimSpace(passages[pid])

			docID, err := distributedid.NextID()
			if err != nil {
				return nil, fmt.Errorf("generate doc id: %w", err)
			}
			chunkID, err := distributedid.NextID()
			if err != nil {
				return nil, fmt.Errorf("generate chunk id: %w", err)
			}
			docIDStr := fmt.Sprintf("%d", docID)
			chunkIDStr := fmt.Sprintf("%d", chunkID)

			doc := knowledgedomain.KnowledgeDocument{
				ID:              docIDStr,
				KnowledgeBaseID: kbID,
				Name:            fmt.Sprintf("passage-%s", pid),
				Enabled:         true,
				ChunkCount:      1,
				FileType:        "text/plain",
				FileSize:        int64(len([]byte(text))),
				ProcessMode:     knowledgedomain.KnowledgeDocumentProcessModeChunk,
				Status:          knowledgedomain.KnowledgeDocumentStatusSuccess,
				SourceType:      knowledgedomain.KnowledgeDocumentSourceFile,
				ChunkStrategy:   string(corechunk.StrategyFixedSize),
				CreatedBy:       loaderOperatorID,
				UpdatedBy:       loaderOperatorID,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if _, err := docRepo.Create(ctx, doc); err != nil {
				return nil, fmt.Errorf("create doc for %s: %w", pid, err)
			}

			chunk := knowledgedomain.KnowledgeChunk{
				ID:              chunkIDStr,
				KnowledgeBaseID: kbID,
				DocumentID:      docIDStr,
				ChunkIndex:      0,
				Content:         text,
				CharCount:       len([]rune(text)),
				Enabled:         true,
				CreatedBy:       loaderOperatorID,
				UpdatedBy:       loaderOperatorID,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if _, err := chunkRepo.Create(ctx, chunk); err != nil {
				return nil, fmt.Errorf("create chunk for %s: %w", pid, err)
			}

			batchMappings[j] = mappingEntry{PassageID: pid, DocumentID: docIDStr, ChunkID: chunkIDStr}
			texts[j] = text
		}
		mappings = append(mappings, batchMappings...)

		vectors, err := embedding.EmbedBatch(texts)
		if err != nil {
			return nil, fmt.Errorf("embed batch %d: %w", i/batchSize, err)
		}
		if len(vectors) != len(texts) {
			return nil, fmt.Errorf("embed batch %d: got %d vectors for %d texts", i/batchSize, len(vectors), len(texts))
		}

		cvs := make([]knowledgeport.ChunkVector, len(batch))
		for j, m := range batchMappings {
			cvs[j] = knowledgeport.ChunkVector{
				ChunkID:         m.ChunkID,
				DocumentID:      m.DocumentID,
				KnowledgeBaseID: kbID,
				Index:           0,
				Text:            texts[j],
				Embedding:       vectors[j],
				Metadata: map[string]any{
					"document_name":    fmt.Sprintf("passage-%s", m.PassageID),
					"source_type":      "file",
					"chunk_index":      0,
					"source_file_name": fmt.Sprintf("passage-%s", m.PassageID),
				},
			}
		}
		if err := vectorStore.UpsertDocumentChunks(ctx, cvs); err != nil {
			return nil, fmt.Errorf("upsert vectors batch %d: %w", i/batchSize, err)
		}

		fmt.Fprintf(os.Stderr, "[loader] batch %d/%d: %d passages embedded and indexed\n",
			i/batchSize+1, (len(pids)+batchSize-1)/batchSize, len(batch))

		if err := writeJSONFile(mappingPath, mappings); err != nil {
			return nil, fmt.Errorf("write incremental mapping: %w", err)
		}

		time.Sleep(300 * time.Millisecond)
	}

	fmt.Fprintf(os.Stderr, "[loader] ingested %d passages in %v\n", len(pids), time.Since(startedAt).Round(time.Millisecond))
	return mappings, nil
}

func buildMarkdownManifestDoc(
	file markdownCorpusFile,
	document knowledgedomain.KnowledgeDocument,
	chunks []knowledgedomain.KnowledgeChunk,
	vectorRows []vectorChunkRow,
) markdownManifestDoc {
	vectorByChunkID := make(map[string]vectorChunkRow, len(vectorRows))
	for _, row := range vectorRows {
		vectorByChunkID[row.ChunkID] = row
	}

	manifestChunks := make([]markdownManifestChunk, 0, len(chunks))
	for _, chunk := range chunks {
		row := vectorByChunkID[chunk.ID]
		content := row.Content
		if strings.TrimSpace(content) == "" {
			content = chunk.Content
		}
		manifestChunks = append(manifestChunks, markdownManifestChunk{
			ChunkID:   chunk.ID,
			Index:     chunk.ChunkIndex,
			Content:   content,
			CharCount: chunk.CharCount,
			Metadata:  row.Metadata,
		})
	}
	sort.Slice(manifestChunks, func(i, j int) bool {
		if manifestChunks[i].Index == manifestChunks[j].Index {
			return manifestChunks[i].ChunkID < manifestChunks[j].ChunkID
		}
		return manifestChunks[i].Index < manifestChunks[j].Index
	})

	return markdownManifestDoc{
		DocumentID:   document.ID,
		DocumentName: document.Name,
		RelativePath: file.RelativePath,
		AbsolutePath: file.AbsolutePath,
		ChunkCount:   len(manifestChunks),
		Chunks:       manifestChunks,
	}
}

func listDocumentVectors(ctx context.Context, db *gorm.DB, documentID string) ([]vectorChunkRow, error) {
	rows, err := db.WithContext(ctx).Raw(`
SELECT chunk_id, chunk_index, content, metadata
FROM t_knowledge_chunk_vector
WHERE doc_id = ?
ORDER BY chunk_index ASC, chunk_id ASC
`, documentID).Rows()
	if err != nil {
		return nil, fmt.Errorf("query document vectors: %w", err)
	}
	defer rows.Close()

	result := make([]vectorChunkRow, 0)
	for rows.Next() {
		var (
			chunkID  string
			index    int
			content  string
			metadata []byte
		)
		if err := rows.Scan(&chunkID, &index, &content, &metadata); err != nil {
			return nil, fmt.Errorf("scan document vector row: %w", err)
		}
		result = append(result, vectorChunkRow{
			ChunkID:  chunkID,
			Index:    index,
			Content:  content,
			Metadata: unmarshalJSONMap(metadata),
		})
	}
	return result, nil
}

func defaultManifestPath(dirPath string, chunkStrategy string) string {
	base := filepath.Base(strings.TrimRight(filepath.Clean(dirPath), string(filepath.Separator)))
	base = sanitizeFileToken(base)
	if base == "" {
		base = "markdown_corpus"
	}
	return filepath.Join("testdata", fmt.Sprintf("%s_%s_manifest.json", base, chunkStrategy))
}

func sanitizeFileToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	value = replacer.Replace(value)
	return strings.Trim(value, "_")
}

func writeJSONFile(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func unmarshalJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]any{}
	}
	if result == nil {
		return map[string]any{}
	}
	return result
}

func previewText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func absPathOrOriginal(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}
