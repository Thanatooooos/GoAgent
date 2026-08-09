package wiki_write

import (
	"context"
	"sort"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

// ChunkLister 读取文档的 chunk 列表。
type ChunkLister interface {
	List(ctx context.Context, filter port.KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error)
}

// DocumentGetter 取文档元数据。
type DocumentGetter interface {
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error)
}

const maxContentRunes = 20000

type contentReader struct {
	docGetter DocumentGetter
	chunkList ChunkLister
}

func NewContentReader(docGetter DocumentGetter, chunkList ChunkLister) DocumentContentReader {
	return &contentReader{docGetter: docGetter, chunkList: chunkList}
}

// ReadContent 返回文档标题 + 按 index 排序拼接的 chunk 内容（截断到 maxContentRunes）。
func (r *contentReader) ReadContent(ctx context.Context, documentID string) (string, string, error) {
	doc, err := r.docGetter.GetByID(ctx, documentID)
	if err != nil {
		return "", "", err
	}
	chunks, err := r.chunkList.List(ctx, port.KnowledgeChunkListFilter{
		DocumentID: documentID,
		ListOptions: port.ListOptions{
			Limit: 1000,
		},
	})
	if err != nil {
		return "", "", err
	}
	sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].ChunkIndex < chunks[j].ChunkIndex })
	var parts []string
	total := 0
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Content)
		if text == "" {
			continue
		}
		parts = append(parts, text)
		total += len([]rune(text))
		if total >= maxContentRunes {
			break
		}
	}
	content := strings.Join(parts, "\n\n")
	if len([]rune(content)) > maxContentRunes {
		content = string([]rune(content)[:maxContentRunes])
	}
	return doc.Name, content, nil
}
