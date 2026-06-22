//go:build ignore

package main

import (
	"context"
	"fmt"

	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

type row struct {
	ChunkID string
	DocID   string
	KbID    string
	Preview string
}

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

	ids := []string{
		"23848386822271233", "23848389070745857", "23848389070680321",
		"23848386822467841", "23848386822402305", "23848386822795521",
		"19753007797432577", "19753054774620417", "19753073582338305",
		"19753045077520641", "19753054773702913",
	}
	var rows []row
	err = runtime.DB.WithContext(ctx).Raw(`
SELECT chunk_id, doc_id, kb_id, left(content, 60) AS preview
FROM t_knowledge_chunk_vector
WHERE chunk_id IN ?
`, ids).Scan(&rows).Error
	if err != nil {
		panic(err)
	}
	found := map[string]row{}
	for _, r := range rows {
		found[r.ChunkID] = r
	}
	for _, id := range ids {
		if r, ok := found[id]; ok {
			fmt.Printf("VECTOR %s kb=%s doc=%s preview=%q\n", id, r.KbID, r.DocID, r.Preview)
		} else {
			fmt.Printf("VECTOR %s MISSING\n", id)
		}
	}
}
