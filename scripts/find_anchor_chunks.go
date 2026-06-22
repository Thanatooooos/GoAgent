//go:build ignore

package main

import (
	"context"
	"fmt"

	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

const evalKB = "23848386738319617"

func main() {
	config.LoadConfig("configs")
	ctx := context.Background()
	runtime, _ := ragbootstrap.NewRuntime(ctx, ragbootstrap.RuntimeOptions{})
	defer runtime.Close()

	for _, q := range []struct{ label, like string }{
		{"csp", "%CSP%"},
		{"go_slice", "%slice%扩容%"},
		{"redis_persist", "%AOF%持久化%"},
	} {
		var rows []struct{ ChunkID, Preview string }
		runtime.DB.WithContext(ctx).Raw(`
SELECT chunk_id, left(content, 100) AS preview FROM t_knowledge_chunk_vector
WHERE kb_id=? AND content ILIKE ? LIMIT 3`, evalKB, q.like).Scan(&rows)
		fmt.Printf("=== %s ===\n", q.label)
		for _, r := range rows {
			fmt.Printf("  %s %q\n", r.ChunkID, r.Preview)
		}
	}
}
