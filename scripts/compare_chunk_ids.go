//go:build ignore

package main

import (
	"context"
	"fmt"
	"strings"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

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

	repo := postgresknowledge.NewKnowledgeChunkRepository(runtime.DB)
	ids := []string{
		"23848386822271233", // expected go_slice
		"19753007797432577", // retrieved rank1 baseline go_slice
		"19753054774620417", // retrieved rank1 redis persistence
		"23848389070745857", // expected redis persistence
	}
	for _, id := range ids {
		chunk, err := repo.GetByID(ctx, id)
		if err != nil {
			fmt.Printf("%s err=%v\n", id, err)
			continue
		}
		preview := strings.TrimSpace(chunk.Content)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		fmt.Printf("id=%s doc=%s kb-related doc=%s\n  %q\n\n", id, chunk.DocumentID, chunk.DocumentID, preview)
	}
}
