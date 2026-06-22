//go:build ignore

package main

import (
	"context"
	"fmt"
	"strings"

	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

const evalKB = "23848386738319617"

func main() {
	if err := config.LoadConfig("configs"); err != nil {
		panic(err)
	}
	ctx := context.Background()
	runtime, err := ragbootstrap.NewRuntime(ctx, ragbootstrap.RuntimeOptions{})
	if err != nil {
		panic(err)
	}
	defer runtime.Close()

	queries := []struct {
		name  string
		query string
		exp   string
	}{
		{"baseline_short", "那扩容规则呢", "23848386822271233"},
		{"candidate_rewrite", "Go 1.18 之后 slice 扩容规则是什么", "23848386822271233"},
	}
	for _, q := range queries {
		sample := rageval.Sample{
			Name:             q.name,
			Query:            q.query,
			Target:           rageval.TargetChunk,
			ExpectedIDs:      []string{q.exp},
			KnowledgeBaseIDs: []string{evalKB},
			SearchMode:       "hybrid",
			TopK:             5,
		}
		if err := rageval.ExecuteSample(ctx, &sample, rageval.ExecuteConfig{Retrieve: runtime.Retrieve}); err != nil {
			panic(err)
		}
		fmt.Printf("=== %s query=%q ===\n", q.name, q.query)
		for i, item := range sample.Retrieved {
			mark := ""
			if item.ChunkID == q.exp {
				mark = " EXPECTED"
			}
			fmt.Printf("  %d. %s score=%.4f%s\n", i+1, item.ChunkID, item.Score, mark)
		}
		fmt.Println()
	}

	// sanity: search chunks containing slice growth in eval KB
	var rows []struct {
		ChunkID string
		Preview string
	}
	_ = runtime.DB.WithContext(ctx).Raw(`
SELECT chunk_id, left(content, 80) AS preview
FROM t_knowledge_chunk_vector
WHERE kb_id = ? AND content ILIKE '%slice%' AND content ILIKE '%1.18%'
LIMIT 5`, evalKB).Scan(&rows)
	fmt.Println("=== chunks with slice+1.18 in eval KB ===")
	for _, r := range rows {
		fmt.Printf("  %s %q\n", r.ChunkID, strings.TrimSpace(r.Preview))
	}
}
