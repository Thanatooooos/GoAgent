package core

import ragretrieve "local/rag-project/internal/app/rag/core/retrieve"

// KnowledgeBaseInsufficient returns true when retrieval evidence is too weak to answer directly.
func KnowledgeBaseInsufficient(retrieveResult ragretrieve.Result) bool {
	if len(retrieveResult.Chunks) == 0 {
		return true
	}
	maxScore := float32(0)
	for _, chunk := range retrieveResult.Chunks {
		if chunk.Score > maxScore {
			maxScore = chunk.Score
		}
	}
	if maxScore < 0.4 {
		return true
	}
	allErrored := len(retrieveResult.ChannelStats) > 0
	if allErrored {
		for _, stat := range retrieveResult.ChannelStats {
			if stat.Error == "" && stat.ChunkCount > 0 {
				allErrored = false
				break
			}
		}
	}
	return allErrored
}
