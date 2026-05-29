package postgres

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// RunMigrations 按文件名顺序执行所有 SQL 迁移文件。
// 使用 t_migration_version 表追踪已执行的迁移，支持幂等执行。
func RunMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required for migrations")
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// 确保版本追踪表存在
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS t_migration_version (
			filename VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`).Error; err != nil {
		return fmt.Errorf("create migration version table: %w", err)
	}

	// 按文件名排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if err := applyMigration(db, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

// EnsureTablesExist 检查必需的表是否存在，用于开发/测试环境的快速校验。
// 生产环境请使用 RunMigrations。
func EnsureTablesExist(db *gorm.DB, tables []string) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			return fmt.Errorf("required table %q does not exist, please run migrations first", table)
		}
	}
	return nil
}

func applyMigration(db *gorm.DB, filename string) error {
	// 检查是否已执行
	var count int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM t_migration_version WHERE filename = ?", filename,
	).Scan(&count).Error; err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if count > 0 {
		return nil
	}

	content, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("read migration file %s: %w", filename, err)
	}

	sql := strings.TrimSpace(string(content))
	if sql == "" {
		return nil
	}

	// 在一个事务中执行迁移并记录版本
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitSQLStatements(sql) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("execute migration %s: %w", filename, err)
			}
		}
		return tx.Exec(
			"INSERT INTO t_migration_version (filename) VALUES (?)", filename,
		).Error
	})
}

// splitSQLStatements splits SQL text into individual statements by semicolons.
// PostgreSQL dollar-quoted blocks ($$...$$) are kept intact — semicolons inside
// them do not cause a split.
func splitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inDollarQuote := false
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
		// Track dollar-quote boundaries.  A line that ends with $$ or contains
		// a standalone $$ toggles the flag.  This is deliberately simple: it
		// covers the common DO $$ … END $$ pattern without a full PL/pgSQL
		// lexer.
		if strings.Contains(trimmed, "$$") {
			inDollarQuote = !inDollarQuote
		}
		if strings.HasSuffix(trimmed, ";") && !inDollarQuote {
			statements = append(statements, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}
	if current.Len() > 0 {
		remaining := strings.TrimSpace(current.String())
		if remaining != "" {
			statements = append(statements, remaining)
		}
	}
	return statements
}
