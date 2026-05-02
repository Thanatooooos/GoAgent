package convention

type RetrievedChunk struct {
	ID              string         `json:"id"`
	Text            string         `json:"text"`
	Score           float32        `json:"score"`
	DocumentID      string         `json:"documentId,omitempty"`
	KnowledgeBaseID string         `json:"knowledgeBaseId,omitempty"`
	ChunkIndex      int            `json:"chunkIndex,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}
