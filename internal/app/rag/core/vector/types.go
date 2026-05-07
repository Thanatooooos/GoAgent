package vector

import "context"

type VectorSpaceID struct {
	LogicalName string
	Namespace   string
}

type VectorSpaceSpec struct {
	SpaceID VectorSpaceID
	Remark  string
}

type SearchRequest struct {
	Vector           []float32
	KnowledgeBaseIDs []string
	TopK             int
	ScoreThreshold   *float32
	// SearchMode 检索模式：semantic（纯向量）、keyword（纯关键词）、hybrid（混合）。
	SearchMode string
	// Query 原始查询文本，供 keyword 通道使用。
	Query string
}

type SearchHit struct {
	ChunkID         string
	DocumentID      string
	KnowledgeBaseID string
	Index           int
	Text            string
	Score           float32
	Metadata        map[string]any
}

type Searcher interface {
	// Search 执行向量语义检索。
	Search(ctx context.Context, request SearchRequest) ([]SearchHit, error)
	// SearchByKeyword 执行关键词/全文检索。
	SearchByKeyword(ctx context.Context, query string, knowledgeBaseIDs []string, topK int) ([]SearchHit, error)
}

type StoreAdmin interface {
	EnsureVectorSpace(ctx context.Context, spec VectorSpaceSpec) error
	VectorSpaceExists(ctx context.Context, spaceID VectorSpaceID) (bool, error)
}
