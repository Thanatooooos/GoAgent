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
	Search(ctx context.Context, request SearchRequest) ([]SearchHit, error)
}

type StoreAdmin interface {
	EnsureVectorSpace(ctx context.Context, spec VectorSpaceSpec) error
	VectorSpaceExists(ctx context.Context, spaceID VectorSpaceID) (bool, error)
}
