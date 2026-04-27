package chunk

// Chunk is the normalized chunk domain model used by upper layers.
type Chunk struct {
	ID        string
	Index     int
	Text      string
	Metadata  map[string]any
	Embedding []float32
}

// VectorChunk is kept as an alias for compatibility with future callers.
type VectorChunk = Chunk

func NewChunk(id string, index int, text string) Chunk {
	return Chunk{
		ID:       id,
		Index:    index,
		Text:     text,
		Metadata: map[string]any{},
	}
}
