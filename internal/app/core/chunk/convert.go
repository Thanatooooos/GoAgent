package chunk

import "local/rag-project/internal/framework/convention"

func ToRetrievedChunks(chunks []Chunk) []convention.RetrievedChunk {
	if len(chunks) == 0 {
		return []convention.RetrievedChunk{}
	}
	result := make([]convention.RetrievedChunk, 0, len(chunks))
	for _, each := range chunks {
		result = append(result, convention.RetrievedChunk{
			ID:   each.ID,
			Text: each.Text,
		})
	}
	return result
}
