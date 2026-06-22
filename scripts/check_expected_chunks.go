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

	expected := []string{
		"23848386822271233",
		"23848389070745857",
		"23848389070680321",
		"23848386822467841",
		"23848386822402305",
		"23848386822795521",
	}
	repo := postgresknowledge.NewKnowledgeChunkRepository(runtime.DB)
	for _, id := range expected {
		chunk, err := repo.GetByID(ctx, id)
		if err != nil {
			fmt.Printf("%s: ERROR %v\n", id, err)
			continue
		}
		if strings.TrimSpace(chunk.ID) == "" {
			fmt.Printf("%s: NOT FOUND\n", id)
			continue
		}
		preview := strings.TrimSpace(chunk.Content)
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		fmt.Printf("%s: FOUND doc=%s preview=%q\n", id, chunk.DocumentID, preview)
	}
}
