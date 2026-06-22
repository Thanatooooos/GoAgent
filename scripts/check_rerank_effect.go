package main

import (
	"context"
	"fmt"
	"os"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
)

func main() {
	query := "defer LIFO execution order in Go"
	kb := []string{"23848386738319617"}
	expected := "23848386822205697"

	for _, model := range []string{"qwen3-rerank", "rerank-noop"} {
		_ = os.Setenv("AI_RERANK_DEFAULT_MODEL", model)
		rt, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{})
		if err != nil {
			fmt.Printf("%s build: %v\n", model, err)
			continue
		}
		res, err := rt.Retrieve.Retrieve(context.Background(), ragretrieve.Request{
			Query:            query,
			KnowledgeBaseIDs: kb,
			SearchMode:       ragretrieve.SearchModeHybrid,
			TopK:             5,
		})
		_ = rt.Close()
		if err != nil {
			fmt.Printf("%s retrieve: %v\n", model, err)
			continue
		}
		fmt.Printf("\n=== rerank model: %s ===\n", model)
		hit := false
		for i, c := range res.Chunks {
			if c.ID == expected {
				hit = true
			}
			fmt.Printf("  %d id=%s score=%.6f\n", i+1, c.ID, c.Score)
		}
		fmt.Printf("  expected in top5: %v\n", hit)
	}
}
