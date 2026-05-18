package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	pgvectorstore "local/rag-project/internal/adapter/vectorstore/pgvector"
	"local/rag-project/internal/framework/config"
)

type chunkRow struct {
	ChunkID  string
	Content  string
	Metadata []byte
}

func main() {
	kbID := flag.String("kb", "", "knowledge base ID filter (empty = all)")
	batchSize := flag.Int("batch", 200, "batch size for updates")
	dryRun := flag.Bool("dry-run", false, "only print statistics, do not update")
	flag.Parse()

	if err := config.LoadConfig("configs"); err != nil {
		fmt.Fprintf(os.Stderr, "[lexical-rebuild] warning: load config: %v (continuing with defaults)\n", err)
	}

	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create db: %v\n", err)
		os.Exit(1)
	}
	defer closeDB(db)

	ctx := context.Background()

	// Apply lexical column migration if not already present.
	if err := ensureLexicalColumns(db); err != nil {
		fmt.Fprintf(os.Stderr, "[lexical-rebuild] migration failed: %v\n", err)
		os.Exit(1)
	}

	if err := run(ctx, db, strings.TrimSpace(*kbID), *batchSize, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "rebuild failed: %v\n", err)
		os.Exit(1)
	}
}

func ensureLexicalColumns(db *gorm.DB) error {
	// Check if already applied.
	if _, err := db.Raw(
		"SELECT content_lexemes FROM t_knowledge_chunk_vector LIMIT 0",
	).Rows(); err == nil {
		fmt.Println("[lexical-rebuild] lexical columns already present")
		return nil
	}

	fmt.Println("[lexical-rebuild] adding lexical columns and indexes...")
	stmts := []string{
		`ALTER TABLE t_knowledge_chunk_vector
			ADD COLUMN IF NOT EXISTS content_lexemes TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS metadata_document_name_lexemes TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS metadata_source_file_name_lexemes TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS metadata_section_lexemes TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_chunk_vector_content_lexemes_tsv
			ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', content_lexemes))`,
		`CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_docname_lexemes_tsv
			ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_document_name_lexemes))`,
		`CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_filename_lexemes_tsv
			ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_source_file_name_lexemes))`,
		`CREATE INDEX IF NOT EXISTS idx_chunk_vector_meta_section_lexemes_tsv
			ON t_knowledge_chunk_vector USING GIN (to_tsvector('simple', metadata_section_lexemes))`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%w\nSQL: %s", err, stmt)
		}
	}
	fmt.Println("[lexical-rebuild] lexical columns and indexes created")
	return nil
}

func run(ctx context.Context, db *gorm.DB, kbID string, batchSize int, dryRun bool) error {
	// Verify lexical columns exist by executing the query.
	rows, err := db.WithContext(ctx).Raw(
		"SELECT content_lexemes FROM t_knowledge_chunk_vector LIMIT 0",
	).Rows()
	if err != nil {
		return fmt.Errorf("lexical columns not found — migration may not have been applied: %w", err)
	}
	_ = rows.Close()

	// Count total rows.
	var total int64
	countSQL := "SELECT COUNT(1) FROM t_knowledge_chunk_vector"
	args := []any{}
	if kbID != "" {
		countSQL += " WHERE kb_id = ?"
		args = append(args, kbID)
	}
	if err := db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return fmt.Errorf("count chunks: %w", err)
	}
	if total == 0 {
		if kbID != "" {
			fmt.Printf("[lexical-rebuild] no chunks found for kb_id=%s\n", kbID)
		} else {
			fmt.Println("[lexical-rebuild] no chunks found")
		}
		return nil
	}

	kbLabel := kbID
	if kbLabel == "" {
		kbLabel = "(all)"
	}
	fmt.Printf("[lexical-rebuild] kb=%s total=%d dry-run=%t\n", kbLabel, total, dryRun)

	if dryRun {
		return dryRunReport(ctx, db, kbID, total)
	}

	// Process in batches.
	offset := 0
	updated := int64(0)
	skipped := int64(0)
	startedAt := time.Now()

	for offset < int(total) {
		rows, err := fetchBatch(ctx, db, kbID, batchSize, offset)
		if err != nil {
			return fmt.Errorf("fetch batch at offset %d: %w", offset, err)
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			metadata := unmarshalMetadata(row.Metadata)
			payload := pgvectorstore.BuildLexicalPayload(row.Content, metadata)

			// Skip rows where all lexemes are empty (nothing to index).
			if payload.ContentLexemes == "" &&
				payload.DocumentNameLexemes == "" &&
				payload.SourceFileNameLexemes == "" &&
				payload.SectionLexemes == "" {
				skipped++
				continue
			}

			err := db.WithContext(ctx).Exec(`
				UPDATE t_knowledge_chunk_vector
				SET content_lexemes = ?,
				    metadata_document_name_lexemes = ?,
				    metadata_source_file_name_lexemes = ?,
				    metadata_section_lexemes = ?
				WHERE chunk_id = ?`,
				payload.ContentLexemes,
				payload.DocumentNameLexemes,
				payload.SourceFileNameLexemes,
				payload.SectionLexemes,
				row.ChunkID,
			).Error
			if err != nil {
				return fmt.Errorf("update chunk %s: %w", row.ChunkID, err)
			}
			updated++
		}

		offset += len(rows)
		pct := float64(offset) / float64(total) * 100
		fmt.Printf("[lexical-rebuild] progress: %d/%d (%.1f%%) updated=%d skipped=%d\n",
			offset, total, pct, updated, skipped)
	}

	elapsed := time.Since(startedAt).Round(time.Millisecond)
	fmt.Printf("[lexical-rebuild] done: updated=%d skipped=%d total=%d elapsed=%s\n",
		updated, skipped, total, elapsed)
	return nil
}

func dryRunReport(ctx context.Context, db *gorm.DB, kbID string, total int64) error {
	// Sample a few rows to show what would be rebuilt.
	rows, err := fetchBatch(ctx, db, kbID, 20, 0)
	if err != nil {
		return fmt.Errorf("dry-run fetch: %w", err)
	}

	needRebuild := int64(0)
	emptyContent := int64(0)
	for _, row := range rows {
		metadata := unmarshalMetadata(row.Metadata)
		payload := pgvectorstore.BuildLexicalPayload(row.Content, metadata)
		if payload.ContentLexemes != "" || payload.DocumentNameLexemes != "" ||
			payload.SourceFileNameLexemes != "" || payload.SectionLexemes != "" {
			needRebuild++
		}
		if strings.TrimSpace(row.Content) == "" {
			emptyContent++
		}
	}

	fmt.Printf("[lexical-rebuild] dry-run: sampled %d rows, %d would be updated, %d have empty content\n",
		len(rows), needRebuild, emptyContent)

	if len(rows) > 0 && needRebuild > 0 {
		docName := metadataString(unmarshalMetadata(rows[0].Metadata), "document_name")
		fmt.Printf("[lexical-rebuild] example: chunk=%s doc=%s content_preview=%s\n",
			rows[0].ChunkID, docName, previewText(rows[0].Content, 60))
	}

	fmt.Printf("[lexical-rebuild] dry-run: %d total rows would be scanned\n", total)
	return nil
}

func fetchBatch(ctx context.Context, db *gorm.DB, kbID string, limit int, offset int) ([]chunkRow, error) {
	sql := "SELECT chunk_id, content, metadata FROM t_knowledge_chunk_vector"
	args := []any{}

	if kbID != "" {
		sql += " WHERE kb_id = ?"
		args = append(args, kbID)
	}
	sql += " ORDER BY chunk_id ASC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var result []chunkRow
	if err := db.WithContext(ctx).Raw(sql, args...).Scan(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func unmarshalMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return map[string]any{}
	}
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func previewText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
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
